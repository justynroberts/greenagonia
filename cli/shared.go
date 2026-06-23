package main

// greenagonia shared <subcommand>
//
// Manages the shared/multi-user Greenagonia environment: multiple admins
// in one PagerDuty account, provisioned from terraform-shared/.
// Config and secrets live in terraform-shared/ alongside the .tf files.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/term"
)

// ---------------------------------------------------------------------------
// Config types — mirrors config.auto.tfvars.json and secrets.auto.tfvars.json
// ---------------------------------------------------------------------------

type sharedConfig struct {
	ScheduleTimeZone        string                 `json:"schedule_time_zone"`
	SiteURL                 string                 `json:"site_url"`
	EnableIncidentWorkflows bool                   `json:"enable_incident_workflows"`
	EnableSlack             bool                   `json:"enable_slack"`
	SlackWorkspaceID        string                 `json:"slack_workspace_id"`
	Admins                  map[string]sharedAdmin `json:"admins"`
}

type sharedAdmin struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	SlackEmail string `json:"slack_email,omitempty"`
	Exists     bool   `json:"exists,omitempty"`
}

type sharedSecrets struct {
	PagerDutyToken     string `json:"pagerduty_token,omitempty"`
	PagerDutyRegion    string `json:"pagerduty_region,omitempty"`
	PagerDutyUserToken string `json:"pagerduty_user_token,omitempty"`
	SlackBotToken      string `json:"slack_bot_token,omitempty"`
}

// ---------------------------------------------------------------------------
// Directory resolution
// ---------------------------------------------------------------------------

func defaultSharedTfDir() string {
	if _, err := os.Stat("terraform-shared/main.tf"); err == nil {
		return "terraform-shared"
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "..", "terraform-shared")
		if _, err := os.Stat(filepath.Join(cand, "main.tf")); err == nil {
			return cand
		}
	}
	return "terraform-shared"
}

// ---------------------------------------------------------------------------
// Config and secrets IO
// ---------------------------------------------------------------------------

const defaultSharedConfig = `{
  "schedule_time_zone": "Europe/London",
  "site_url": "http://localhost:8080",
  "enable_incident_workflows": false,
  "enable_slack": false,
  "slack_workspace_id": "",
  "admins": {}
}`

func sharedConfigPath(dir string) string { return filepath.Join(dir, "config.auto.tfvars.json") }
func sharedSecretsPath(dir string) string { return filepath.Join(dir, "secrets.auto.tfvars.json") }

func loadSharedConfig(dir string) (sharedConfig, error) {
	path := sharedConfigPath(dir)
	var cfg sharedConfig
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg.ScheduleTimeZone = "Europe/London"
		cfg.SiteURL = "http://localhost:8080"
		cfg.Admins = map[string]sharedAdmin{}
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Admins == nil {
		cfg.Admins = map[string]sharedAdmin{}
	}
	return cfg, nil
}

func saveSharedConfig(dir string, cfg sharedConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sharedConfigPath(dir), data, 0644)
}

func loadSharedSecrets(dir string) (sharedSecrets, error) {
	var s sharedSecrets
	data, err := os.ReadFile(sharedSecretsPath(dir))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func saveSharedSecrets(dir string, s sharedSecrets) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := sharedSecretsPath(dir)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	fmt.Printf("  %s saved to %s (chmod 600, gitignored)\n", green("✓"), path)
	return nil
}

// ---------------------------------------------------------------------------
// Input helpers
// ---------------------------------------------------------------------------

var stdinReader = bufio.NewReader(os.Stdin)

func prompt(p string) string {
	fmt.Print(p)
	line, _ := stdinReader.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptDefault(p, def string) string {
	fmt.Printf("%s (default %s): ", p, def)
	line, _ := stdinReader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func promptPassword(p string) (string, error) {
	fmt.Print(p)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return strings.TrimSpace(string(b)), err
}

func promptYN(p string, def bool) bool {
	if def {
		fmt.Printf("%s [Y/n]: ", p)
	} else {
		fmt.Printf("%s [y/N]: ", p)
	}
	line, _ := stdinReader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return def
	}
	return line == "y" || line == "yes"
}

// ---------------------------------------------------------------------------
// Command: shared token
// ---------------------------------------------------------------------------

func cmdSharedToken(dir string) {
	tok, err := promptPassword("PagerDuty REST API token (admin/write scope, hidden): ")
	if err != nil || tok == "" {
		die("no token entered")
	}
	region := strings.ToLower(prompt("Region [us/eu] (default us): "))
	if region == "" {
		region = "us"
	}
	if region != "us" && region != "eu" {
		die("region must be us or eu")
	}
	s, err := loadSharedSecrets(dir)
	if err != nil {
		die("reading secrets: " + err.Error())
	}
	s.PagerDutyToken = tok
	s.PagerDutyRegion = region
	if err := saveSharedSecrets(dir, s); err != nil {
		die("saving secrets: " + err.Error())
	}
}

// ---------------------------------------------------------------------------
// Command: shared user-token
// ---------------------------------------------------------------------------

func cmdSharedUserToken(dir string) {
	tok, err := promptPassword("PagerDuty user-level API token (My Profile → User Settings → Create API User Token, hidden): ")
	if err != nil || tok == "" {
		die("no token entered")
	}
	s, err := loadSharedSecrets(dir)
	if err != nil {
		die("reading secrets: " + err.Error())
	}
	s.PagerDutyUserToken = tok
	if err := saveSharedSecrets(dir, s); err != nil {
		die("saving secrets: " + err.Error())
	}
}

// ---------------------------------------------------------------------------
// Command: shared slack-token
// ---------------------------------------------------------------------------

func cmdSharedSlackToken(dir string) {
	tok, err := promptPassword("Slack bot token (xoxb-…, hidden): ")
	if err != nil || tok == "" {
		die("no token entered")
	}
	if !strings.HasPrefix(tok, "xoxb-") {
		die("token must start with xoxb-")
	}
	s, err := loadSharedSecrets(dir)
	if err != nil {
		die("reading secrets: " + err.Error())
	}
	s.SlackBotToken = tok
	if err := saveSharedSecrets(dir, s); err != nil {
		die("saving secrets: " + err.Error())
	}
	fmt.Printf("  %s run 'greenagonia shared deploy' to create the Slack channels\n", green("✓"))
}

// ---------------------------------------------------------------------------
// Command: shared admin
// ---------------------------------------------------------------------------

var reInitials = regexp.MustCompile(`^[A-Z]{2,4}$`)
var reAlpha = regexp.MustCompile(`[^A-Za-z]`)

func cmdSharedAdmin(dir string, args []string) {
	if len(args) < 1 {
		cmdSharedAdminList(dir)
		return
	}
	switch args[0] {
	case "add":
		if len(args) < 4 {
			die("usage: greenagonia shared admin add <INITIALS> \"Full Name\" <email> [slack-email]")
		}
		ini := strings.ToUpper(args[1])
		name := args[2]
		email := args[3]
		slack := ""
		if len(args) >= 5 {
			slack = args[4]
		}
		cmdSharedAdminAdd(dir, ini, name, email, slack)
	case "remove":
		if len(args) < 2 {
			die("usage: greenagonia shared admin remove <INITIALS>")
		}
		cmdSharedAdminRemove(dir, strings.ToUpper(args[1]))
	case "list", "":
		cmdSharedAdminList(dir)
	default:
		die("usage: greenagonia shared admin [add|remove|list]")
	}
}

func cmdSharedAdminAdd(dir, ini, name, email, slackEmail string) {
	if !reInitials.MatchString(ini) {
		die("initials must be 2–4 uppercase letters (e.g. JR)")
	}
	if !strings.Contains(email, "@") {
		die(fmt.Sprintf("%q doesn't look like an email", email))
	}
	if slackEmail != "" && !strings.Contains(slackEmail, "@") {
		die(fmt.Sprintf("%q doesn't look like an email", slackEmail))
	}

	cfg, err := loadSharedConfig(dir)
	if err != nil {
		die("reading config: " + err.Error())
	}

	// Same email under same initials → update in place.
	if ex, ok := cfg.Admins[ini]; ok && strings.EqualFold(ex.Email, email) {
		entry := sharedAdmin{Name: name, Email: email, Exists: ex.Exists}
		if slackEmail != "" {
			entry.SlackEmail = slackEmail
		}
		cfg.Admins[ini] = entry
		if err := saveSharedConfig(dir, cfg); err != nil {
			die("saving config: " + err.Error())
		}
		fmt.Printf("  %s updated %s = %s <%s>\n", green("✓"), ini, name, email)
		fmt.Printf("  run 'greenagonia shared deploy' to apply\n")
		return
	}

	// Initials taken by someone else → derive an alternative.
	if _, taken := cfg.Admins[ini]; taken {
		parts := strings.Fields(name)
		var alpha []string
		for _, p := range parts {
			c := reAlpha.ReplaceAllString(p, "")
			if c != "" {
				alpha = append(alpha, c)
			}
		}
		var first, last string
		if len(alpha) > 1 {
			first, last = alpha[0], alpha[len(alpha)-1]
		} else if len(alpha) == 1 {
			first, last = alpha[0], alpha[0]
		} else {
			die(fmt.Sprintf("'%s' is already taken and can't derive initials from %q", ini, name))
		}

		var candidates []string
		for n := 1; n <= min(4, len(last)); n++ {
			c := strings.ToUpper(string(first[0])) + strings.ToUpper(last[:n])
			if reInitials.MatchString(c) {
				candidates = append(candidates, c)
			}
		}
		for n := 2; n <= min(3, len(first)); n++ {
			c := strings.ToUpper(first[:n]) + strings.ToUpper(string(last[0]))
			if reInitials.MatchString(c) {
				candidates = append(candidates, c)
			}
		}

		var pick string
		for _, c := range candidates {
			if _, exists := cfg.Admins[c]; !exists {
				pick = c
				break
			}
		}
		if pick == "" {
			die(fmt.Sprintf("'%s' is taken and no free alternative could be derived from %q — pass explicit initials", ini, name))
		}
		fmt.Printf("  %s '%s' is taken by %s — using %s instead\n", dim("→"), ini, cfg.Admins[ini].Name, pick)
		ini = pick
	}

	entry := sharedAdmin{Name: name, Email: email}
	if slackEmail != "" {
		entry.SlackEmail = slackEmail
	}
	cfg.Admins[ini] = entry
	if err := saveSharedConfig(dir, cfg); err != nil {
		die("saving config: " + err.Error())
	}
	suffix := ""
	if slackEmail != "" {
		suffix = fmt.Sprintf(" (slack: %s)", slackEmail)
	}
	fmt.Printf("  %s added %s = %s <%s>%s\n", green("✓"), ini, name, email, suffix)
	fmt.Printf("  run 'greenagonia shared deploy' to apply\n")
}

func cmdSharedAdminRemove(dir, ini string) {
	cfg, err := loadSharedConfig(dir)
	if err != nil {
		die("reading config: " + err.Error())
	}
	if _, ok := cfg.Admins[ini]; !ok {
		die(fmt.Sprintf("no admin %q in config", ini))
	}
	delete(cfg.Admins, ini)
	if err := saveSharedConfig(dir, cfg); err != nil {
		die("saving config: " + err.Error())
	}
	fmt.Printf("  %s removed %s — their PagerDuty stack will be destroyed on next deploy\n", green("✓"), ini)
	fmt.Printf("  run 'greenagonia shared deploy' to apply\n")
}

func cmdSharedAdminList(dir string) {
	cfg, err := loadSharedConfig(dir)
	if err != nil {
		die("reading config: " + err.Error())
	}
	if len(cfg.Admins) == 0 {
		fmt.Println("  no admins configured — add one:")
		fmt.Println("  greenagonia shared admin add JR \"Justyn Roberts\" justyn@example.com")
		return
	}
	keys := make([]string, 0, len(cfg.Admins))
	for k := range cfg.Admins {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, ini := range keys {
		a := cfg.Admins[ini]
		slack := ""
		if a.SlackEmail != "" {
			slack = fmt.Sprintf("  (slack: %s)", a.SlackEmail)
		}
		fmt.Printf("  %-4s %s <%s>%s\n", ini, a.Name, a.Email, slack)
	}
}

// ---------------------------------------------------------------------------
// Command: shared site-url
// ---------------------------------------------------------------------------

func cmdSharedSiteURL(dir string, args []string) {
	var u string
	if len(args) > 0 {
		u = args[0]
	} else {
		u = prompt("Storefront base URL (e.g. http://3.85.144.140): ")
	}
	if u == "" {
		die("no URL entered")
	}
	cfg, err := loadSharedConfig(dir)
	if err != nil {
		die("reading config: " + err.Error())
	}
	cfg.SiteURL = strings.TrimRight(u, "/")
	if err := saveSharedConfig(dir, cfg); err != nil {
		die("saving config: " + err.Error())
	}
	fmt.Printf("  %s site_url = %s\n", green("✓"), cfg.SiteURL)
	fmt.Println("  run 'greenagonia shared deploy' to apply, then 'greenagonia shared urls' to see updated links")
}

// ---------------------------------------------------------------------------
// Command: shared setup (first-run wizard)
// ---------------------------------------------------------------------------

func cmdSharedSetup(dir string) {
	banner()
	fmt.Printf("  %s  %s\n\n", bold("greenagonia"), dim("shared environment — setup wizard"))

	s, err := loadSharedSecrets(dir)
	if err != nil {
		die("reading secrets: " + err.Error())
	}
	if s.PagerDutyToken != "" {
		fmt.Printf("  %s token already configured (run 'greenagonia shared token' to replace)\n\n", green("✓"))
	} else {
		cmdSharedToken(dir)
		fmt.Println()
	}

	cfg, err := loadSharedConfig(dir)
	if err != nil {
		die("reading config: " + err.Error())
	}

	cfg.ScheduleTimeZone = promptDefault("Schedule time zone", "Europe/London")
	cfg.SiteURL = strings.TrimRight(promptDefault("Storefront base URL", "http://localhost:8080"), "/")
	cfg.EnableIncidentWorkflows = promptYN("Enable Incident Workflows? (paid feature, 404s without it)", false)
	cfg.EnableSlack = promptYN("Enable per-admin Slack incident channels? (needs Slack V2 integration + workflows)", false)
	if cfg.EnableSlack {
		wsid := prompt("Slack workspace ID (starts with T, from the PD Slack integration page): ")
		if !regexp.MustCompile(`^T[A-Z0-9]+$`).MatchString(wsid) {
			die("that doesn't look like a Slack workspace ID (e.g. T01ABCDEF23)")
		}
		cfg.SlackWorkspaceID = wsid
	}

	if err := saveSharedConfig(dir, cfg); err != nil {
		die("saving config: " + err.Error())
	}
	fmt.Printf("\n  %s settings saved\n", green("✓"))

	if len(cfg.Admins) == 0 {
		fmt.Printf("\n  %s\n", bold("Add the first admin"))
		ini := strings.ToUpper(prompt("  Initials (e.g. JR): "))
		name := prompt("  Full name: ")
		email := prompt("  Email (their real PagerDuty notification address): ")
		cmdSharedAdminAdd(dir, ini, name, email, "")
	}

	fmt.Printf("\n  %s setup complete\n", green("✓"))
	fmt.Printf("  next: greenagonia shared deploy\n\n")
}

// ---------------------------------------------------------------------------
// Command: shared deploy
// ---------------------------------------------------------------------------

func cmdSharedDeploy(dir string) {
	if _, err := os.Stat(sharedSecretsPath(dir)); os.IsNotExist(err) {
		die("no token configured — run 'greenagonia shared setup' first")
	}

	// Ensure config exists.
	if _, err := os.Stat(sharedConfigPath(dir)); os.IsNotExist(err) {
		if err := os.WriteFile(sharedConfigPath(dir), []byte(defaultSharedConfig), 0644); err != nil {
			die("creating config: " + err.Error())
		}
	}

	fmt.Printf("\n  %s  terraform init\n\n", dim("▸"))
	if err := tfRun(dir, "init", "-upgrade"); err != nil {
		die("terraform init: " + err.Error())
	}

	planFile := filepath.Join(dir, "tfplan")
	fmt.Printf("\n  %s  terraform plan\n\n", dim("▸"))
	if err := tfRun(dir, "plan", "-out="+planFile); err != nil {
		_ = os.Remove(planFile)
		die("terraform plan: " + err.Error())
	}

	if !promptYN("\nApply this plan?", false) {
		_ = os.Remove(planFile)
		die("aborted — nothing applied")
	}

	fmt.Printf("\n  %s  terraform apply\n\n", dim("▸"))
	if err := tfRun(dir, "apply", planFile); err != nil {
		_ = os.Remove(planFile)
		die("terraform apply: " + err.Error())
	}
	_ = os.Remove(planFile)

	fmt.Println()
	cmdSharedURLs(dir, nil)
}

// ---------------------------------------------------------------------------
// Command: shared urls
// ---------------------------------------------------------------------------

func cmdSharedURLs(dir string, args []string) {
	filter := ""
	if len(args) > 0 {
		filter = strings.ToUpper(args[0])
	}

	outs, err := terraformOutputs(dir)
	if err != nil {
		die("terraform outputs: " + err.Error())
	}

	get := func(name string) map[string]string {
		raw, ok := outs[name]
		if !ok {
			return nil
		}
		var m map[string]string
		_ = json.Unmarshal(raw, &m)
		return m
	}
	getString := func(name string) string {
		raw, ok := outs[name]
		if !ok {
			return ""
		}
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}

	routing := get("admin_routing_keys")
	siteURLs := get("admin_site_urls")
	fullURLs := get("admin_full_urls")
	changeKeys := get("change_event_keys")
	ldKeys := get("change_event_keys_ld")
	platformKey := getString("platform_routing_key")

	cfg, _ := loadSharedConfig(dir)

	if len(routing) == 0 {
		die("no outputs yet — run 'greenagonia shared deploy'")
	}

	if filter != "" && filter != "PLATFORM" {
		if _, ok := routing[filter]; !ok {
			keys := make([]string, 0, len(routing))
			for k := range routing {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			die(fmt.Sprintf("unknown admin %q — known: %s, PLATFORM", filter, strings.Join(keys, ", ")))
		}
	}

	if filter == "" || filter == "PLATFORM" {
		if platformKey != "" {
			fmt.Printf("\n%s  %s\n", bold("PLATFORM"), dim("(Greenagonia shared)"))
			fmt.Printf("  %-14s %s\n", dim("routing key"), yellow(platformKey))
			fmt.Printf("  %-14s %s\n", dim("services"), "api-gateway · data-platform · identity-service · infrastructure · platform-engineering")
		}
	}

	keys := make([]string, 0, len(routing))
	for k := range routing {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, ini := range keys {
		if filter != "" && filter != "PLATFORM" && ini != filter {
			continue
		}
		rk := routing[ini]
		a := cfg.Admins[ini]
		label := ""
		if a.Name != "" {
			label = fmt.Sprintf("  %s", dim(fmt.Sprintf("(%s · %s)", a.Name, a.Email)))
		}
		fmt.Printf("\n%s%s\n", bold(ini), label)
		fmt.Printf("  %-14s %s\n", dim("routing key"), green(rk))
		if u := siteURLs[ini]; u != "" {
			fmt.Printf("  %-14s %s\n", dim("storefront"), cyan(u))
		}
		if u := fullURLs[ini]; u != "" {
			fmt.Printf("  %-14s %s\n", dim("full URL"), cyan(u))
		}

		// Change keys for this admin.
		var svcKeys []string
		for k := range changeKeys {
			if strings.HasPrefix(k, ini+"/") {
				svcKeys = append(svcKeys, k)
			}
		}
		sort.Strings(svcKeys)
		if len(svcKeys) > 0 {
			fmt.Printf("\n  %s\n", bold("Change event keys"))
			for _, k := range svcKeys {
				svc := strings.TrimPrefix(k, ini+"/")
				fmt.Printf("  %-32s %s  %s\n", dim(svc), changeKeys[k], dim("(github)"))
				if svc == "payment-gateway" {
					ldKey := ""
					for ldk, lv := range ldKeys {
						if strings.HasPrefix(ldk, ini+"/") {
							ldKey = lv
							break
						}
					}
					if ldKey != "" {
						fmt.Printf("  %-32s %s  %s\n", dim(""), ldKey, dim("(launchdarkly)"))
					}
				}
			}
		}
	}
	fmt.Println()
}

// ---------------------------------------------------------------------------
// Command: shared slack-channels
// ---------------------------------------------------------------------------

func slackAPI(token, method, path string, body map[string]any) (map[string]any, error) {
	var reqBody io.Reader
	var reqURL = "https://slack.com/api/" + path

	if method == "GET" {
		if len(body) > 0 {
			q := make([]string, 0, len(body))
			for k, v := range body {
				q = append(q, fmt.Sprintf("%s=%v", k, v))
			}
			reqURL += "?" + strings.Join(q, "&")
		}
	} else {
		b, _ := json.Marshal(body)
		reqBody = strings.NewReader(string(b))
	}

	req, err := http.NewRequest(method, reqURL, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func cmdSharedSlackChannels(dir string) {
	s, err := loadSharedSecrets(dir)
	if err != nil || s.SlackBotToken == "" {
		die("no Slack credentials — run 'greenagonia shared slack-token' first")
	}
	cfg, err := loadSharedConfig(dir)
	if err != nil {
		die("reading config: " + err.Error())
	}
	if len(cfg.Admins) == 0 {
		die("no admins configured — add one first")
	}

	tok := s.SlackBotToken
	var errCount int

	inis := make([]string, 0, len(cfg.Admins))
	for k := range cfg.Admins {
		inis = append(inis, k)
	}
	sort.Strings(inis)

	for _, ini := range inis {
		admin := cfg.Admins[ini]
		channelName := strings.ToLower(ini) + "-incidents"

		// Create channel.
		r, err := slackAPI(tok, "POST", "conversations.create", map[string]any{
			"name": channelName, "is_private": false,
		})
		var channelID string
		if err != nil {
			fmt.Printf("  %s %s: %v\n", red("✗"), channelName, err)
			errCount++
			continue
		}
		if r["ok"] == true {
			if ch, ok := r["channel"].(map[string]any); ok {
				channelID, _ = ch["id"].(string)
			}
			fmt.Printf("  %s %s created (%s)\n", green("✓"), bold(channelName), channelID)
		} else if r["error"] == "name_taken" {
			// Look it up.
			r2, _ := slackAPI(tok, "GET", "conversations.list", map[string]any{
				"types": "public_channel", "limit": "200",
			})
			if chs, ok := r2["channels"].([]any); ok {
				for _, c := range chs {
					if ch, ok := c.(map[string]any); ok && ch["name"] == channelName {
						channelID, _ = ch["id"].(string)
						break
					}
				}
			}
			if channelID == "" {
				fmt.Printf("  %s %s: created but couldn't look up ID\n", red("✗"), channelName)
				errCount++
				continue
			}
			fmt.Printf("  %s %s already exists (%s)\n", dim("–"), bold(channelName), channelID)
		} else {
			fmt.Printf("  %s %s: %v\n", red("✗"), channelName, r["error"])
			errCount++
			continue
		}

		// Look up user by email (prefer slack_email if set).
		lookupEmail := admin.Email
		if admin.SlackEmail != "" {
			lookupEmail = admin.SlackEmail
		}
		r, err = slackAPI(tok, "GET", "users.lookupByEmail", map[string]any{"email": lookupEmail})
		if err != nil || r["ok"] != true {
			fmt.Printf("    %s user %s: %v — skipping invite\n", red("✗"), lookupEmail, r["error"])
			errCount++
			continue
		}
		var userID string
		if u, ok := r["user"].(map[string]any); ok {
			userID, _ = u["id"].(string)
		}

		// Invite user.
		r, err = slackAPI(tok, "POST", "conversations.invite", map[string]any{
			"channel": channelID, "users": userID,
		})
		if err != nil {
			fmt.Printf("    %s invite %s: %v\n", red("✗"), lookupEmail, err)
			errCount++
		} else if r["ok"] == true {
			fmt.Printf("    %s invited %s (%s)\n", green("✓"), lookupEmail, userID)
		} else if e, _ := r["error"].(string); e == "already_in_channel" || e == "cant_invite_self" {
			fmt.Printf("    %s %s already in channel\n", dim("–"), lookupEmail)
		} else {
			fmt.Printf("    %s invite %s: %v\n", red("✗"), lookupEmail, r["error"])
			errCount++
		}
	}

	fmt.Println()
	if errCount > 0 {
		fmt.Printf("  %s completed with %d error(s)\n", red("✗"), errCount)
		os.Exit(1)
	}
	fmt.Printf("  %s all channels ready\n", green("✓"))
}

// ---------------------------------------------------------------------------
// Command: shared destroy
// ---------------------------------------------------------------------------

func cmdSharedDestroy(dir string) {
	fmt.Printf("\n  %s This DESTROYS the entire shared environment in PagerDuty:\n", red("✗"))
	fmt.Println("  every admin's team, services, schedules, orchestration — and the personas.")
	confirm := prompt("  Type \"destroy\" to confirm: ")
	if confirm != "destroy" {
		die("aborted")
	}
	if err := tfRun(dir, "destroy", "-auto-approve"); err != nil {
		die("terraform destroy: " + err.Error())
	}
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

func cmdShared(args []string) {
	dir := defaultSharedTfDir()

	if len(args) < 1 {
		sharedUsage()
		return
	}

	switch args[0] {
	case "setup":
		cmdSharedSetup(dir)
	case "token":
		cmdSharedToken(dir)
	case "user-token":
		cmdSharedUserToken(dir)
	case "admin":
		cmdSharedAdmin(dir, args[1:])
	case "deploy":
		cmdSharedDeploy(dir)
	case "urls":
		cmdSharedURLs(dir, args[1:])
	case "site-url":
		cmdSharedSiteURL(dir, args[1:])
	case "slack-token":
		cmdSharedSlackToken(dir)
	case "slack-channels":
		cmdSharedSlackChannels(dir)
	case "destroy":
		cmdSharedDestroy(dir)
	default:
		fmt.Fprintf(os.Stderr, "unknown shared subcommand %q\n\n", args[0])
		sharedUsage()
		os.Exit(2)
	}
}

func sharedUsage() {
	fmt.Print(`usage:
  greenagonia shared setup                                          first-run wizard
  greenagonia shared token                                          set/replace PagerDuty REST API token
  greenagonia shared user-token                                     set PagerDuty user-level token (Slack connections)
  greenagonia shared admin add <INI> "Name" <email> [slack-email]  add/update an admin
  greenagonia shared admin remove <INI>                             remove an admin (destroyed on next deploy)
  greenagonia shared admin list                                     list admins
  greenagonia shared deploy                                         terraform init + plan + confirm + apply
  greenagonia shared urls [INITIALS]                                per-admin storefront links and keys
  greenagonia shared site-url [URL]                                 set storefront base URL
  greenagonia shared slack-token                                    save Slack bot token
  greenagonia shared slack-channels                                 create {ini}-incidents channels and invite admins
  greenagonia shared destroy                                        tear down the entire shared environment

config and state live in terraform-shared/ (gitignored, kept locally).
run from the repo root or use --tf-dir to override.
`)
}

