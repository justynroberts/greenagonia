// greenagonia — deploy & demo the Greenagonia PagerDuty environment.
//
// Three subcommands:
//
//	greenagonia deploy    --api-token <t> --email <e> [--env <name>] [--user-name <n>]
//	greenagonia undeploy  --api-token <t> [--env <name>]
//	greenagonia scenarios list
//	greenagonia scenarios run [--env <name>] [--scenario <name>] [--repeat N] [--no-delay] [--routing-key <k>]
//
// deploy/undeploy shell out to terraform (which must be on PATH) and use a
// per-env workspace so several environments can coexist in the same PagerDuty
// account. After deploy, the orchestration routing key AND the per-service
// change-event integration keys are persisted to ~/.greenagonia/<env>.json
// so `scenarios run --env <name>` works without further flags.
//
// scenarios posts two kinds of payloads:
//
//   - Change events to https://events.pagerduty.com/v2/change/enqueue, using
//     the per-service integration key. Each scenario opens with a simulated
//     bad GitHub deploy that PagerDuty correlates with the incidents that
//     follow on the same service.
//   - Alert events to https://events.pagerduty.com/v2/enqueue, using the
//     orchestration routing key. Every payload carries custom_details.service
//     so the router in main.tf fans events out to the right service.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed web.html
var webHTML string

// ===========================================================================
// Style — TTY-aware colours, severity chips, hyperlinks, boxed cards
// ===========================================================================

var noColor = os.Getenv("NO_COLOR") != "" || !isTTY(os.Stdout)

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func ansi(code, s string) string {
	if noColor || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func red(s string) string        { return ansi("31", s) }
func green(s string) string      { return ansi("32", s) }
func yellow(s string) string     { return ansi("33", s) }
func cyan(s string) string       { return ansi("36", s) }
func gray(s string) string       { return ansi("90", s) }
func bold(s string) string       { return ansi("1", s) }
func dim(s string) string        { return ansi("2", s) }
func pdGreen(s string) string    { return ansi("38;5;35;1", s) }
func pdGreenSoft(s string) string { return ansi("38;5;35", s) }

// sevChip is a coloured pill label for severities and the "change" marker.
// All chips render at the same visible width (10 chars) so adjacent
// columns stay aligned in scenario timelines. NO_COLOR mode falls back to
// "[CRITICAL]" form, also padded to 10 chars.
func sevChip(sev string) string {
	const w = 8 // chip body width (inside the colour blocks / brackets)
	upper := strings.ToUpper(sev)
	label := upper + strings.Repeat(" ", w-len(upper))
	if len(upper) > w {
		label = upper[:w]
	}
	if noColor {
		return "[" + label + "]"
	}
	pad := " " + label + " "
	switch sev {
	case "critical":
		return "\x1b[48;5;160;97;1m" + pad + "\x1b[0m"
	case "error":
		return "\x1b[48;5;208;97;1m" + pad + "\x1b[0m"
	case "warning":
		return "\x1b[48;5;220;30;1m" + pad + "\x1b[0m"
	case "info":
		return "\x1b[48;5;33;97;1m" + pad + "\x1b[0m"
	case "change":
		return "\x1b[48;5;35;97;1m" + pad + "\x1b[0m"
	default:
		return "\x1b[48;5;240;97;1m" + pad + "\x1b[0m"
	}
}

// hyperlink emits an OSC 8 escape — clickable in modern terminals
// (iTerm2, kitty, WezTerm, Windows Terminal), plain text elsewhere.
func hyperlink(url, text string) string {
	if noColor {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// visualWidth measures rune count excluding ANSI escapes / OSC 8 link wrappers,
// so card padding calculations are correct even with colour codes baked in.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m|\x1b\]8;;[^\x07\x1b]*(\x07|\x1b\\)`)

func visualWidth(s string) int {
	return utf8.RuneCountInString(ansiRE.ReplaceAllString(s, ""))
}

var bannerPrinted bool

func banner() {
	if bannerPrinted {
		return
	}
	bannerPrinted = true
	dot := pdGreen("●")
	fmt.Println()
	fmt.Println("  " + dot + "  " + bold("greenagonia") + "  " + dim("·") + "  " + dim("PagerDuty demo environment"))
	fmt.Println("  " + dim(strings.Repeat("─", 56)))
	fmt.Println()
}

const (
	bTL = "╭"
	bTR = "╮"
	bBL = "╰"
	bBR = "╯"
	bLT = "├"
	bRT = "┤"
	bH  = "─"
	bV  = "│"
)

// card renders a Unicode-boxed card. `title` is bolded into the top border;
// `badges` (pre-styled, e.g. sevChip) appear right-aligned on the same line;
// `body` is one pre-styled string per row. Use "" for a blank row and "@DIV"
// for a horizontal divider inside the box.
func card(title string, badges []string, body []string) {
	const w = 96
	inner := w - 2    // chars between the two vertical bars
	contentW := w - 4 // chars usable by one body row (with 1-space side pad)

	// Top border: ╭ title ──── badge1 badge2 ─╮
	titleStr := " " + bold(title) + " "
	badgesStr := ""
	if len(badges) > 0 {
		badgesStr = " " + strings.Join(badges, " ") + " "
	}
	dashes := inner - visualWidth(titleStr) - visualWidth(badgesStr)
	if dashes < 1 {
		dashes = 1
	}
	fmt.Println(pdGreenSoft(bTL) + titleStr + pdGreenSoft(strings.Repeat(bH, dashes)) + badgesStr + pdGreenSoft(bTR))

	for _, ln := range body {
		switch {
		case ln == "@DIV":
			fmt.Println(pdGreenSoft(bLT) + pdGreenSoft(strings.Repeat(bH, inner)) + pdGreenSoft(bRT))
		case ln == "":
			fmt.Println(pdGreenSoft(bV) + strings.Repeat(" ", inner) + pdGreenSoft(bV))
		default:
			pad := contentW - visualWidth(ln)
			if pad < 0 {
				pad = 0
			}
			fmt.Println(pdGreenSoft(bV) + " " + ln + strings.Repeat(" ", pad) + " " + pdGreenSoft(bV))
		}
	}

	fmt.Println(pdGreenSoft(bBL) + pdGreenSoft(strings.Repeat(bH, inner)) + pdGreenSoft(bBR))
}

// homeShort replaces $HOME with ~ for nicer printing.
func homeShort(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" && strings.HasPrefix(p, h) {
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}

// eventsBase returns the Events API base URL for the requested PD service
// region. "us" (default) → events.pagerduty.com; "eu" → events.eu.pagerduty.com.
func eventsBase(region string) string {
	if strings.EqualFold(region, "eu") {
		return "https://events.eu.pagerduty.com"
	}
	return "https://events.pagerduty.com"
}

func alertsURLFor(region string) string  { return eventsBase(region) + "/v2/enqueue" }
func changesURLFor(region string) string { return eventsBase(region) + "/v2/change/enqueue" }

// validRegion returns "us" or "eu", lowercased. Falls back to "us" for empty.
func validRegion(r string) (string, error) {
	switch strings.ToLower(r) {
	case "", "us":
		return "us", nil
	case "eu":
		return "eu", nil
	default:
		return "", fmt.Errorf("--region must be 'us' or 'eu' (got %q)", r)
	}
}

// ---------------------------------------------------------------------------
// Events API types — alerts (v2/enqueue) and changes (v2/change/enqueue)
// ---------------------------------------------------------------------------

type pdEvent struct {
	RoutingKey  string  `json:"routing_key"`
	EventAction string  `json:"event_action"`
	DedupKey    string  `json:"dedup_key,omitempty"`
	Payload     payload `json:"payload"`
}

type payload struct {
	Summary       string         `json:"summary"`
	Source        string         `json:"source"`
	Severity      string         `json:"severity"`
	Component     string         `json:"component,omitempty"`
	Group         string         `json:"group,omitempty"`
	Class         string         `json:"class,omitempty"`
	CustomDetails map[string]any `json:"custom_details,omitempty"`
}

type pdChange struct {
	RoutingKey string        `json:"routing_key"`
	Payload    changePayload `json:"payload"`
	Links      []changeLink  `json:"links,omitempty"`
}

type changePayload struct {
	Summary       string         `json:"summary"`
	Timestamp     string         `json:"timestamp,omitempty"`
	Source        string         `json:"source,omitempty"`
	CustomDetails map[string]any `json:"custom_details,omitempty"`
}

type changeLink struct {
	Href string `json:"href"`
	Text string `json:"text,omitempty"`
}

// ---------------------------------------------------------------------------
// Scenarios
// ---------------------------------------------------------------------------

type step struct {
	delay    time.Duration // wall-clock offset from the first alert
	service  string        // MUST match a technical_services key in main.tf
	summary  string        // short notification title — NO service name (service shows via the PD service itself)
	desc     string        // one-sentence elaboration, lands in custom_details.description
	severity string        // critical | error | warning | info
	source   string
	extra    map[string]any // merged into custom_details
}

// changeEvent is the simulated GitHub deploy posted at scenario start to
// the change-events endpoint, attached to a specific service.
type changeEvent struct {
	service    string // MUST match a technical_services key (drives lookup of integration key)
	summary    string
	sourceTool string // e.g. "GitHub Actions"
	custom     map[string]any
	links      []changeLink
}

type scenario struct {
	name   string
	desc   string
	// friendly fields surfaced through the web UI for non-technical users.
	// title falls back to name; impact/affects to desc/services when empty.
	title   string
	impact  string   // customer-facing consequence in plain language
	affects []string // business-area names (not technical services)
	// shared lands in custom_details of EVERY alert in this scenario —
	// identical values across the whole cascade (deploy_id, release,
	// root_cause_service). This is deliberate: PagerDuty's intelligent
	// grouper keys on matching field values, so shared fields let it
	// correlate alerts across services, not just within one.
	shared map[string]any
	change *changeEvent
	steps  []step
}

// Technical-service list — keep in sync with locals.technical_services in
// terraform/main.tf.
var techServices = []string{
	"payment-gateway", "checkout-api", "user-auth", "product-catalog",
	"recommendation-engine", "search-service", "order-service", "notification-service",
}

// commonFields land in custom_details on EVERY alert across every scenario.
// They give PagerDuty's intelligent grouper a stable substrate of identical
// fields to recognise — alerts sharing region/cluster/environment look more
// related than alerts that don't.
var commonFields = map[string]any{
	"environment":       "production",
	"region":            "us-east-1",
	"cluster":           "use1-prod-1",
	"kubernetes":        true,
	"monitoring_system": "datadog",
	"datacenter":        "aws-use1",
}

// serviceMeta is per-service constant context — namespace, owning team,
// runbook. All alerts on the same service share these, which is exactly
// the signal AIOps grouping uses to roll them up.
var serviceMeta = map[string]map[string]any{
	"payment-gateway": {
		"namespace":  "payments",
		"deployment": "payment-gateway",
		"team":       "platform-payments",
		"runbook":    "https://wiki.greenagonia.io/runbooks/payment-gateway",
	},
	"checkout-api": {
		"namespace":  "commerce",
		"deployment": "checkout-api",
		"team":       "checkout",
		"runbook":    "https://wiki.greenagonia.io/runbooks/checkout-api",
	},
	"user-auth": {
		"namespace":  "identity",
		"deployment": "user-auth",
		"team":       "identity",
		"runbook":    "https://wiki.greenagonia.io/runbooks/user-auth",
	},
	"product-catalog": {
		"namespace":  "catalog",
		"deployment": "product-catalog",
		"team":       "catalog",
		"runbook":    "https://wiki.greenagonia.io/runbooks/product-catalog",
	},
	"recommendation-engine": {
		"namespace":  "recommendations",
		"deployment": "recommendation-engine",
		"team":       "data-platform",
		"runbook":    "https://wiki.greenagonia.io/runbooks/recommendation-engine",
	},
	"search-service": {
		"namespace":  "search",
		"deployment": "search-service",
		"team":       "search",
		"runbook":    "https://wiki.greenagonia.io/runbooks/search-service",
	},
	"order-service": {
		"namespace":  "commerce",
		"deployment": "order-service",
		"team":       "orders",
		"runbook":    "https://wiki.greenagonia.io/runbooks/order-service",
	},
	"notification-service": {
		"namespace":  "platform",
		"deployment": "notification-service",
		"team":       "platform-notifications",
		"runbook":    "https://wiki.greenagonia.io/runbooks/notification-service",
	},
}

// ghChange constructs the simulated bad GitHub push that opens each scenario.
func ghChange(service, repo, version, author string, prNumber int, prTitle, sha string, extra map[string]any) *changeEvent {
	short := sha
	if len(short) > 7 {
		short = sha[:7]
	}
	custom := map[string]any{
		"repository":     repo,
		"branch":         "main",
		"commit_sha":     sha,
		"commit_short":   short,
		"author":         author,
		"pr_number":      prNumber,
		"pr_title":       prTitle,
		"deployed_to":    "production",
		"release_tag":    version,
		"deploy_status":  "success",
		"service":        service,
		"workflow":       "deploy.yml",
		"trigger":        "merge_to_main",
	}
	for k, v := range extra {
		custom[k] = v
	}
	return &changeEvent{
		service:    service,
		summary:    fmt.Sprintf("Deployed %s %s → production (PR #%d: %s)", service, version, prNumber, prTitle),
		sourceTool: "GitHub Actions",
		custom:     custom,
		links: []changeLink{
			{Href: fmt.Sprintf("https://github.com/%s/pull/%d", repo, prNumber), Text: fmt.Sprintf("PR #%d", prNumber)},
			{Href: fmt.Sprintf("https://github.com/%s/commit/%s", repo, short), Text: "Commit " + short},
			{Href: fmt.Sprintf("https://github.com/%s/actions/workflows/deploy.yml", repo), Text: "CI deploy workflow"},
		},
	}
}

// alertStep is a tiny constructor that keeps the scenario tables readable.
// Numerical detail belongs in `extra` (custom_details), never the summary.
func alertStep(delaySec int, svc, summary, desc, severity, source string, extra map[string]any) step {
	return step{
		delay:    time.Duration(delaySec) * time.Second,
		service:  svc,
		summary:  summary,
		desc:     desc,
		severity: severity,
		source:   source,
		extra:    extra,
	}
}

// Each scenario fans 3-4 alerts at every affected service within a ~15s
// burst. Service-level time-based grouping (2-minute window, set in
// services.tf) collapses each fan-out into a single incident — so a
// 12-alert scenario produces 3-4 incidents (one per service) rather than
// 12. Summaries describe symptoms only; numerical detail lives in
// custom_details.
var scenarios = []scenario{
	{
		name:    "bad-payment-deploy",
		title:   "Payment Outage",
		impact:  "Customers can't complete payment at checkout. Orders are failing and confirmation emails are backing up.",
		affects: []string{"Customer Checkout", "Order Processing"},
		desc:    "payment-gateway v2.4.1 cascade — payment, checkout, orders, notifications.",
		shared: map[string]any{
			"release":            "v2.4.1",
			"deploy_id":          "github-deploy-412",
			"root_cause_service": "payment-gateway",
		},
		change: ghChange(
			"payment-gateway", "greenagonia/payment-gateway",
			"v2.4.1", "alice@greenagonia.io", 412,
			"Refactor Amex card auth flow",
			"a3f9c12ee45b8a2c1d9f8b3c7e5a9d1f2b6c4e8a",
			map[string]any{"reviewers": []string{"bob", "carol"}, "files_changed": 14, "lines_added": 287, "lines_removed": 41},
		),
		steps: []step{
			// payment-gateway — burst of 4
			alertStep(0, "payment-gateway", "Authentication failures elevated",
				"Card authorization requests failing for the majority of Amex transactions since the v2.4.1 deploy.",
				"critical", "payment-gw-prod-3",
				map[string]any{"release": "v2.4.1", "error_class": "java.lang.NullPointerException", "card_brand": "amex"}),
			alertStep(5, "payment-gateway", "Card processor timeouts",
				"Calls to the Adyen processor exceeding the 10s timeout; retries are compounding the load.",
				"critical", "payment-gw-prod-7",
				map[string]any{"processor": "adyen", "timeout_count": 184}),
			alertStep(10, "payment-gateway", "Service health check failing",
				"The /healthz endpoint has failed 12 consecutive probes; instances are being pulled from the load balancer.",
				"critical", "payment-gw-prod-3",
				map[string]any{"endpoint": "/healthz", "consecutive_failures": 12}),
			alertStep(14, "payment-gateway", "Error budget burn-rate exceeded",
				"Availability SLO burning 14x faster than sustainable; budget exhausted in under 2 hours at this rate.",
				"error", "slo-monitor",
				map[string]any{"slo": "payment-gateway-availability", "burn_rate_x": 14}),

			// checkout-api — burst of 3
			alertStep(20, "checkout-api", "Upstream payment timeouts",
				"Requests to payment-gateway timing out at p99 of 9.8s; checkout requests queuing behind them.",
				"error", "checkout-prod-7",
				map[string]any{"upstream": "payment-gateway", "timeout_p99_ms": 9800}),
			alertStep(25, "checkout-api", "Checkout completion rate dropped",
				"Completed checkouts down from ~280/min to 12/min; customers abandoning after payment errors.",
				"error", "checkout-prod-7",
				map[string]any{"complete_per_min": 12, "baseline_per_min": 280}),
			alertStep(30, "checkout-api", "Cart abandonment elevated",
				"Real-user monitoring shows 64% of carts abandoned at the payment step.",
				"warning", "rum-monitor",
				map[string]any{"abandonment_pct": 64}),

			// order-service — burst of 3
			alertStep(36, "order-service", "Failed-order rate above baseline",
				"Orders failing at 412/min against a 34/min baseline; failures correlate with payment declines.",
				"error", "order-prod-2",
				map[string]any{"failed_per_min": 412, "baseline_per_min": 34}),
			alertStep(41, "order-service", "Order processing degraded",
				"Order pipeline p99 at 8.2s; payment-confirmation steps dominating processing time.",
				"error", "order-prod-2",
				map[string]any{"p99_ms": 8200}),
			alertStep(46, "order-service", "Downstream payment errors propagating",
				"79% of order failures trace to upstream payment-gateway errors.",
				"error", "order-prod-5",
				map[string]any{"upstream_error_pct": 79}),

			// notification-service — burst of 2
			alertStep(52, "notification-service", "Confirmation email queue backlog growing",
				"order_confirmations queue depth at 18,450 and climbing; emails delayed until orders clear.",
				"warning", "notif-prod-1",
				map[string]any{"queue": "order_confirmations", "queue_depth": 18450}),
			alertStep(58, "notification-service", "Notification delivery delayed",
				"Delivery delayed by more than 60s; current lag is 4 minutes behind real time.",
				"warning", "notif-prod-3",
				map[string]any{"delivery_lag_sec": 240}),
		},
	},
	{
		name:    "db-migration-fail",
		title:   "Login Failure",
		impact:  "Customers can't log in. Product browsing and search are degraded across the site.",
		affects: []string{"Customer Login", "Browse & Discover"},
		desc:    "Long-running schema migration locks user_session — auth, catalog, search, recs degrade.",
		shared: map[string]any{
			"migration_id":       "0427_session_uuid",
			"deploy_id":          "github-deploy-1207",
			"root_cause_service": "user-auth",
		},
		change: ghChange(
			"user-auth", "greenagonia/user-auth",
			"v4.11.0", "bob@greenagonia.io", 1207,
			"Migrate user_session.id from BIGINT to UUID",
			"7b2e9d34c8a1f6b5e3d2c7a9b8f1e6d4c3b2a1f0",
			map[string]any{"migration_id": "0427_session_uuid", "estimated_runtime_sec": 45, "actual_runtime_sec": ">300 (still running)"},
		),
		steps: []step{
			// user-auth — burst of 4
			alertStep(0, "user-auth", "Login latency elevated",
				"Login p99 at 14s against a 320ms baseline; the user_session table is locked by a running migration.",
				"critical", "auth-prod-4",
				map[string]any{"p99_ms": 14000, "baseline_p99_ms": 320}),
			alertStep(5, "user-auth", "Connection pool saturated",
				"All 200 database connections in use; new login attempts queuing until connections free up.",
				"critical", "auth-prod-4",
				map[string]any{"pool_in_use": 200, "pool_size": 200}),
			alertStep(10, "user-auth", "Session creation failing",
				"INSERTs into user_session blocked by migration 0427_session_uuid holding a table lock.",
				"critical", "auth-prod-1",
				map[string]any{"db_lock_table": "user_session", "migration_id": "0427_session_uuid"}),
			alertStep(14, "user-auth", "Auth health check timing out",
				"The /healthz probe exceeding its 30s timeout because the readiness query is stuck behind the lock.",
				"error", "auth-prod-1",
				map[string]any{"endpoint": "/healthz", "timeout_ms": 30000}),

			// product-catalog — burst of 3
			alertStep(20, "product-catalog", "Database read timeouts elevated",
				"38% of catalog reads timing out; shared database under contention from the auth migration.",
				"error", "catalog-prod-9",
				map[string]any{"read_timeout_pct": 38}),
			alertStep(26, "product-catalog", "Cache miss rate elevated",
				"Cache misses at 71%; refresh queries failing so stale entries are being evicted without replacement.",
				"error", "catalog-prod-5",
				map[string]any{"miss_pct": 71}),
			alertStep(32, "product-catalog", "Catalog query failures",
				"28% of product queries returning errors after exhausting database retries.",
				"error", "catalog-prod-9",
				map[string]any{"query_error_pct": 28}),

			// search-service — burst of 3
			alertStep(38, "search-service", "Empty result rate elevated",
				"22% of searches returning zero results because the catalog backend is unavailable.",
				"error", "search-prod-2",
				map[string]any{"empty_result_pct": 22, "cause": "catalog backend unavailable"}),
			alertStep(44, "search-service", "Index lookups failing",
				"1,840 index lookups per minute failing on catalog enrichment calls.",
				"error", "search-prod-1",
				map[string]any{"failed_lookups_per_min": 1840}),
			alertStep(50, "search-service", "Search latency degraded",
				"Search p99 at 5.2s while enrichment calls wait on catalog timeouts.",
				"warning", "search-prod-2",
				map[string]any{"p99_ms": 5200}),

			// recommendation-engine — burst of 2
			alertStep(56, "recommendation-engine", "Falling back to default recommendations",
				"Personalised recommendations unavailable; 100% of traffic now served generic defaults.",
				"warning", "recs-prod-5",
				map[string]any{"fallback_active": true, "fallback_pct": 100}),
			alertStep(62, "recommendation-engine", "Personalization pipeline degraded",
				"4 pipeline stages stalled waiting on user-profile reads that depend on the locked session store.",
				"warning", "recs-prod-3",
				map[string]any{"pipeline_stalled_steps": 4}),
		},
	},
	{
		name:    "memory-leak-recs",
		title:   "Recommendations Crashing",
		impact:  "Product recommendations are unavailable. Search is slower and the catalog is overloaded.",
		affects: []string{"Browse & Discover"},
		desc:    "recommendation-engine v3.2 leaks heap — OOM-kill loop drives load into search & catalog.",
		shared: map[string]any{
			"release":            "v3.2.0",
			"deploy_id":          "github-deploy-89",
			"root_cause_service": "recommendation-engine",
		},
		change: ghChange(
			"recommendation-engine", "greenagonia/recommendation-engine",
			"v3.2.0", "carol@greenagonia.io", 89,
			"Add cohort-based ranking experiment",
			"e1d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0",
			map[string]any{"feature_flag": "cohort_ranking_v3", "rollout_pct": 100, "experiment_owner": "data-platform"},
		),
		steps: []step{
			// recommendation-engine — burst of 4
			alertStep(0, "recommendation-engine", "OOMKilled containers",
				"Containers killed at the 8GB memory limit 3 times in 5 minutes following the v3.2 deploy.",
				"critical", "recs-prod-1",
				map[string]any{"release": "v3.2", "restart_count": 3, "window_min": 5}),
			alertStep(5, "recommendation-engine", "Restart loop detected",
				"Pods restarting 8 times per minute; each instance leaks heap and dies within seconds of serving traffic.",
				"critical", "recs-prod-1",
				map[string]any{"restarts_per_min": 8}),
			alertStep(10, "recommendation-engine", "Memory pressure critical",
				"Heap at 7.8GB of an 8GB ceiling within 90 seconds of startup; allocation profile points at the new ranking cache.",
				"critical", "recs-prod-2",
				map[string]any{"heap_used_mb": 7820, "heap_max_mb": 8192}),
			alertStep(14, "recommendation-engine", "Health endpoint not responding",
				"The /healthz endpoint unreachable; instances dying before the readiness window completes.",
				"error", "recs-prod-2",
				map[string]any{"endpoint": "/healthz"}),

			// search-service — burst of 3
			alertStep(20, "search-service", "Search latency degraded",
				"Search p99 at 4.2s (baseline 850ms); recommendation sidecar calls timing out and retrying.",
				"error", "search-prod-1",
				map[string]any{"p99_ms": 4200, "p50_ms": 920}),
			alertStep(26, "search-service", "Tail latency elevated",
				"p999 at 12s; worst-case requests waiting through multiple recommendation retries.",
				"error", "search-prod-3",
				map[string]any{"p999_ms": 12000}),
			alertStep(32, "search-service", "Queue depth growing",
				"8,400 requests queued; capacity consumed by slow downstream calls.",
				"warning", "search-prod-1",
				map[string]any{"queue_depth": 8400}),

			// product-catalog — burst of 3
			alertStep(38, "product-catalog", "Request rate above baseline",
				"Traffic at 3x baseline as search falls back to direct catalog queries.",
				"warning", "catalog-prod-2",
				map[string]any{"rps": 4200, "baseline_rps": 1400}),
			alertStep(44, "product-catalog", "Cache hit ratio dropped",
				"Hit ratio down to 41%; fallback queries have a different access pattern than the warmed cache.",
				"warning", "catalog-prod-4",
				map[string]any{"hit_ratio_pct": 41}),
			alertStep(50, "product-catalog", "Database load elevated",
				"Database CPU at 87% absorbing the cache misses.",
				"warning", "catalog-prod-2",
				map[string]any{"db_cpu_pct": 87}),
		},
	},
	{
		name:    "config-push-gateway",
		title:   "Site-Wide Authentication Failure",
		impact:  "Site-wide impact. Customers can't log in, pay, or browse — everything is returning 401.",
		affects: []string{"Customer Login", "Customer Checkout", "Browse & Discover"},
		desc:    "Bad API-gateway config drops the Authorization header — every downstream 401s.",
		shared: map[string]any{
			"config_version":     "gateway-cfg-2026.06.09-3",
			"deploy_id":          "github-deploy-88",
			"root_cause_service": "api-gateway",
		},
		// Attached to user-auth because it's the first and most-impacted service;
		// metadata makes clear the change is in a gateway config repo, not the
		// service's own repo.
		change: ghChange(
			"user-auth", "greenagonia/api-gateway-config",
			"cfg-2026.06.09-3", "dave@greenagonia.io", 88,
			"Tighten upstream header allowlist",
			"f5e4d3c2b1a0f9e8d7c6b5a4f3e2d1c0b9a8f7e6",
			map[string]any{"config_file": "routes/auth.yaml", "change_type": "header_allowlist", "header_removed": "Authorization", "rollout_minutes": 0},
		),
		steps: []step{
			// user-auth — burst of 4
			alertStep(0, "user-auth", "Authorization failures elevated",
				"71% of requests rejected with 401 since gateway config push; the Authorization header is being stripped upstream.",
				"critical", "auth-prod-1",
				map[string]any{"config_version": "gateway-cfg-2026.06.09-3", "rate_401_pct": 71}),
			alertStep(4, "user-auth", "Session validation failing",
				"Session validation calls arriving with no Authorization header; every validation attempt rejected.",
				"critical", "auth-prod-2",
				map[string]any{"cause": "missing Authorization header"}),
			alertStep(8, "user-auth", "Token verification errors",
				"Token verifier raising MissingAuthHeader for all inbound traffic routed through the gateway.",
				"critical", "auth-prod-3",
				map[string]any{"error_class": "MissingAuthHeader"}),
			alertStep(13, "user-auth", "Auth health check failing",
				"Authenticated health probe failing — the probe's own credentials are stripped by the gateway too.",
				"critical", "auth-prod-1",
				map[string]any{"endpoint": "/healthz"}),

			// checkout-api — burst of 3
			alertStep(18, "checkout-api", "Authentication errors elevated",
				"98% of checkout calls receiving 401 from downstream auth; no customer can reach payment.",
				"critical", "checkout-prod-1",
				map[string]any{"rate_401_pct": 98}),
			alertStep(23, "checkout-api", "Checkout failures across the board",
				"99% of checkout attempts failing; every authenticated step in the flow is rejected.",
				"critical", "checkout-prod-1",
				map[string]any{"failure_pct": 99}),
			alertStep(28, "checkout-api", "Service unavailable to clients",
				"All client traffic effectively blocked; circuit breakers opening across checkout instances.",
				"critical", "checkout-prod-3",
				map[string]any{"clients_affected": "all"}),

			// order-service — burst of 3
			alertStep(34, "order-service", "Cannot process orders",
				"Order processing halted; user-context lookups required for every order are failing.",
				"critical", "order-prod-1",
				map[string]any{"orders_blocked": true}),
			alertStep(39, "order-service", "Session lookup failing",
				"Session lookups against user-auth returning 401; orders cannot be attributed to customers.",
				"critical", "order-prod-2",
				map[string]any{"cause": "user-auth 401"}),
			alertStep(44, "order-service", "Order intake blocked",
				"New order intake suspended to prevent unattributed orders from queueing.",
				"critical", "order-prod-1",
				map[string]any{}),

			// product-catalog — burst of 2
			alertStep(50, "product-catalog", "Authentication required errors",
				"Personalised-pricing requests rejected with 401; anonymous browsing still functional.",
				"error", "catalog-prod-1",
				map[string]any{}),
			alertStep(56, "product-catalog", "Personalised endpoints failing",
				"All endpoints requiring a user context are failing; serving non-personalised fallbacks.",
				"error", "catalog-prod-3",
				map[string]any{}),
		},
	},
	{
		name:    "cache-stampede-search",
		title:   "Search Slowdown",
		impact:  "Search is slow and product browsing is degraded. Recommendations are serving stale results.",
		affects: []string{"Browse & Discover"},
		desc:    "Bad TTL change in search-service — cache stampede overloads catalog.",
		shared: map[string]any{
			"release":            "v1.8.0",
			"deploy_id":          "github-deploy-304",
			"root_cause_service": "search-service",
		},
		change: ghChange(
			"search-service", "greenagonia/search-service",
			"v1.8.0", "eve@greenagonia.io", 304,
			"Tighten cache TTL to improve freshness",
			"c4b3a2f1e0d9c8b7a6f5e4d3c2b1a0f9e8d7c6b5",
			map[string]any{"old_ttl_sec": 60, "new_ttl_sec": 5, "intent": "improve search result freshness"},
		),
		steps: []step{
			// search-service — burst of 4
			alertStep(0, "search-service", "Cache hit ratio dropped",
				"Hit ratio collapsed from 94% to 12% after v1.8.0 cut cache TTL from 60s to 5s.",
				"critical", "search-prod-3",
				map[string]any{"release": "v1.8.0", "hit_ratio_pct": 12, "change": "ttl 60s → 5s"}),
			alertStep(5, "search-service", "Backend QPS elevated",
				"7,100 queries/sec hitting the catalog backend — formerly absorbed by cache.",
				"critical", "search-prod-3",
				map[string]any{"backend_qps": 7100}),
			alertStep(10, "search-service", "Search latency degraded",
				"Search p99 at 1.8s; nearly every query now pays the full backend round-trip.",
				"critical", "search-prod-1",
				map[string]any{"p99_ms": 1800}),
			alertStep(14, "search-service", "Connection pool saturated",
				"All 64 backend connections in use; queries queueing for a free connection.",
				"error", "search-prod-2",
				map[string]any{"pool_in_use": 64, "pool_size": 64}),

			// product-catalog — burst of 3
			alertStep(20, "product-catalog", "Read load critical",
				"Read traffic at 5x baseline from the search cache stampede.",
				"critical", "catalog-prod-4",
				map[string]any{"qps": 7100, "baseline_qps": 1400}),
			alertStep(26, "product-catalog", "Database connections elevated",
				"412 database connections in use; approaching the cluster ceiling.",
				"error", "catalog-prod-4",
				map[string]any{"db_connections_in_use": 412}),
			alertStep(32, "product-catalog", "Query latency degraded",
				"Catalog query p99 at 1.8s under the sustained read load.",
				"error", "catalog-prod-2",
				map[string]any{"p99_ms": 1800}),

			// recommendation-engine — burst of 3
			alertStep(38, "recommendation-engine", "Upstream catalog timeouts",
				"31% of catalog enrichment calls timing out under load.",
				"warning", "recs-prod-2",
				map[string]any{"upstream": "product-catalog", "timeout_pct": 31}),
			alertStep(44, "recommendation-engine", "Stale recommendations served",
				"41% of recommendation responses served from stale snapshots while fresh reads time out.",
				"warning", "recs-prod-1",
				map[string]any{"stale_pct": 41}),
			alertStep(50, "recommendation-engine", "Personalization degraded",
				"Live personalisation disabled; serving cached cohort defaults until catalog recovers.",
				"warning", "recs-prod-3",
				map[string]any{"fallback_active": true}),
		},
	},
	{
		name:    "k8s-bad-image-rollout",
		title:   "Bad Deploy Crashed Pods",
		impact:  "Recommendation pods are crash-looping. Browse experience is degraded.",
		affects: []string{"Browse & Discover"},
		desc:    "Bad container image rollout — recommendation-engine pods CrashLoopBackOff, downstream degrades.",
		shared: map[string]any{
			"release":            "v3.3.0",
			"deploy_id":          "argocd-sync-2026.06.09-04",
			"replicaset":         "recommendation-engine-7d4f9c",
			"root_cause_service": "recommendation-engine",
		},
		// GitHub Actions builds the image; ArgoCD syncs it to the cluster;
		// the new pods immediately CrashLoop. Sources are realistic k8s
		// pod / control-plane component names so the AIOps grouper sees
		// host correlation as well as service correlation.
		change: ghChange(
			"recommendation-engine", "greenagonia/recommendation-engine",
			"v3.3.0", "frank@greenagonia.io", 94,
			"Switch to slimmer alpine base image",
			"b9a8f7e6d5c4b3a2f1e0d9c8b7a6f5e4d3c2b1a0",
			map[string]any{
				"image":          "ghcr.io/greenagonia/recommendation-engine:v3.3.0",
				"image_size_mb":  187,
				"previous_image": "ghcr.io/greenagonia/recommendation-engine:v3.2.0",
				"deployer":       "argocd",
				"k8s_cluster":    "use1-prod-1",
				"k8s_namespace":  "recommendations",
				"sync_revision":  "argocd-sync-2026.06.09-04",
			},
		),
		steps: []step{
			// recommendation-engine — pod-level alerts on the bad ReplicaSet.
			// Three pods of the new ReplicaSet all fail the same way; the
			// pod name varies but the replicaset/deployment fields match.
			alertStep(0, "recommendation-engine", "Pod CrashLoopBackOff",
				"Pod killed with exit code 137 (OOM) 6 times; the v3.3.0 alpine image is missing the jemalloc the service tunes for.",
				"critical", "recommendation-engine-7d4f9c-x2k8m",
				map[string]any{"pod_phase": "CrashLoopBackOff", "restart_count": 6, "exit_code": 137, "replicaset": "recommendation-engine-7d4f9c", "image": "ghcr.io/greenagonia/recommendation-engine:v3.3.0"}),
			alertStep(4, "recommendation-engine", "Pod CrashLoopBackOff",
				"Second pod of the new ReplicaSet in CrashLoopBackOff with the same exit code 137 signature.",
				"critical", "recommendation-engine-7d4f9c-p9m4n",
				map[string]any{"pod_phase": "CrashLoopBackOff", "restart_count": 5, "exit_code": 137, "replicaset": "recommendation-engine-7d4f9c"}),
			alertStep(8, "recommendation-engine", "Pod CrashLoopBackOff",
				"Third pod crash-looping — the entire new ReplicaSet is failing identically; zero healthy replicas.",
				"critical", "recommendation-engine-7d4f9c-q3w8j",
				map[string]any{"pod_phase": "CrashLoopBackOff", "restart_count": 4, "exit_code": 137, "replicaset": "recommendation-engine-7d4f9c"}),
			alertStep(12, "recommendation-engine", "Liveness probe failing",
				"Liveness probe failed 3 consecutive times; kubelet restarting the container on a backoff timer.",
				"error", "recommendation-engine-7d4f9c-x2k8m",
				map[string]any{"probe": "liveness", "endpoint": "/healthz", "consecutive_failures": 3, "replicaset": "recommendation-engine-7d4f9c"}),
			alertStep(16, "recommendation-engine", "ReplicaSet rollout stalled",
				"ReplicaSet at 0 of 3 ready replicas; rollout cannot make progress.",
				"error", "kube-controller-manager",
				map[string]any{"replicaset": "recommendation-engine-7d4f9c", "ready_replicas": 0, "desired_replicas": 3}),
			alertStep(20, "recommendation-engine", "Deployment rollout timed out",
				"Progress deadline exceeded; ArgoCD reports the sync OutOfSync and is awaiting manual intervention.",
				"error", "argocd",
				map[string]any{"progress_deadline_exceeded": true, "sync_status": "OutOfSync"}),

			// search-service — circuit-breaker opens to upstream recs.
			alertStep(28, "search-service", "Upstream recommendations unavailable",
				"Circuit breaker to recommendation-engine open; no healthy upstream endpoints to route to.",
				"error", "search-prod-2",
				map[string]any{"upstream": "recommendation-engine", "circuit_breaker": "open"}),
			alertStep(34, "search-service", "Fallback responses elevated",
				"100% of search responses using non-personalised fallback ranking.",
				"error", "search-prod-2",
				map[string]any{"fallback_pct": 100}),
			alertStep(40, "search-service", "Latency degraded",
				"Search p99 at 1.8s; fallback path is slower than the cached recommendation flow.",
				"warning", "search-prod-3",
				map[string]any{"p99_ms": 1800}),

			// product-catalog — absorbs the lost personalisation traffic.
			alertStep(46, "product-catalog", "Request rate above baseline",
				"Traffic at 2.3x baseline as fallback ranking queries the catalog directly.",
				"warning", "catalog-prod-2",
				map[string]any{"rps": 3200, "baseline_rps": 1400}),
			alertStep(52, "product-catalog", "Cache pressure elevated",
				"Cache evicting 28 entries/min under the shifted access pattern.",
				"warning", "catalog-prod-4",
				map[string]any{"eviction_rate_per_min": 28}),
			alertStep(58, "product-catalog", "Database load elevated",
				"Database CPU at 71% absorbing fallback traffic.",
				"warning", "catalog-prod-2",
				map[string]any{"db_cpu_pct": 71}),
		},
	},
	{
		name:    "noisy-neighbour",
		title:   "Background System Noise",
		impact:  "Low-priority alerts from across the platform — used to test that the router and grouping handle volume cleanly.",
		affects: []string{"Platform-Wide"},
		desc:    "Low-priority noise spread across every service — stress-tests the router.",
		// no change event — pure noise. steps populated by init().
	},
}

func init() {
	r := rand.New(rand.NewSource(20260609))
	for i := range scenarios {
		if scenarios[i].name != "noisy-neighbour" {
			continue
		}
		noise := []struct{ summary, desc string }{
			{"Transient CPU spike", "CPU briefly exceeded 80% for under a minute before recovering on its own."},
			{"Brief network latency blip", "Inter-zone round-trip time elevated for ~30 seconds; back to baseline."},
			{"Disk usage climbing", "Volume crossed the 70% soft watermark; cleanup job has been scheduled."},
			{"Sporadic 5xx responses", "A handful of 5xx responses in the last window; below the paging threshold."},
		}
		for j := 0; j < 16; j++ {
			svc := techServices[r.Intn(len(techServices))]
			sev := []string{"info", "warning"}[r.Intn(2)]
			n := noise[r.Intn(len(noise))]
			scenarios[i].steps = append(scenarios[i].steps, step{
				delay:    time.Duration(j*3) * time.Second,
				service:  svc,
				summary:  n.summary,
				desc:     n.desc,
				severity: sev,
				source:   fmt.Sprintf("%s-prod-%d", svc, r.Intn(8)+1),
				extra:    map[string]any{"sample": true, "step": j},
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Persisted env state (~/.greenagonia/<env>.json)
// ---------------------------------------------------------------------------

type envState struct {
	Env        string            `json:"env"`
	Region     string            `json:"region,omitempty"` // "us" | "eu"; default "us" if empty
	RoutingKey string            `json:"routing_key"`
	ChangeKeys map[string]string `json:"change_keys"`
	UpdatedAt  string            `json:"updated_at"`
}

// regionOf returns the state's region, defaulting to "us" for older state
// files written before --region existed.
func (s *envState) regionOf() string {
	if s == nil || s.Region == "" {
		return "us"
	}
	return s.Region
}

func stateFilePath(env string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".greenagonia", env+".json")
}

func saveState(s envState) (string, error) {
	p := stateFilePath(s.Env)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	return p, nil
}

func loadState(env string) (*envState, error) {
	b, err := os.ReadFile(stateFilePath(env))
	if err != nil {
		return nil, err
	}
	var s envState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return &s, nil
}

func removeState(env string) {
	_ = os.Remove(stateFilePath(env))
}

// ---------------------------------------------------------------------------
// CLI entry
// ---------------------------------------------------------------------------

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "setup":
		fmt.Print(setupGuide)
	case "deploy":
		cmdDeploy(os.Args[2:])
	case "undeploy":
		cmdUndeploy(os.Args[2:])
	case "scenarios":
		cmdScenarios(os.Args[2:])
	case "web":
		cmdWeb(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	banner()
	fmt.Println(`usage:
  greenagonia setup                                                # start here
  greenagonia deploy    --api-token <t> --email <e> [--env <name>] [--region us|eu]
                        [--user-name <n>] [--with-workflows]
  greenagonia undeploy  --api-token <t> [--env <name>] [--with-workflows]
  greenagonia scenarios list
  greenagonia scenarios run     [--env <name>] --scenario <name> [--repeat N] [--no-delay]
                                [--routing-key <k> [--region us|eu]]
  greenagonia scenarios resolve [--env <name>] --scenario <name|all>   # close incidents
  greenagonia web       [--env <name>] [--addr :8080]              # browser UI

environment variables (override flag defaults):
  PAGERDUTY_TOKEN     used by --api-token
  PD_ROUTING_KEY      used by --routing-key (overrides state file)
  NO_COLOR            disable colour output

region defaults to 'us' (api.pagerduty.com); use '--region eu' for
EU accounts (api.eu.pagerduty.com). The region is recorded in the state
file so scenarios fire against the right Events API host automatically.

after a successful deploy, the routing key and per-service change-event
keys are saved to ~/.greenagonia/<env>.json, so 'scenarios run --env <name>'
needs no further flags.

terraform must be on PATH for deploy / undeploy.
run 'greenagonia setup' for the full start-to-finish walkthrough.`)
}

// setupGuide is what `greenagonia setup` prints — a complete onboarding
// walkthrough so a brand-new user goes from zero to a running scenario.
const setupGuide = `
========================================================================
 GREENAGONIA — setup guide
========================================================================

A start-to-finish walkthrough. Skip any step you've already done.


------------------------------------------------------------------------
 STEP 1 ── Install Terraform (>= 1.5)
------------------------------------------------------------------------

  macOS      brew install terraform
  Linux      https://developer.hashicorp.com/terraform/install
  Windows    choco install terraform     (or scoop install terraform)

  Verify     terraform version


------------------------------------------------------------------------
 STEP 2 ── Install Go (>= 1.22) — only if you're building the CLI yourself
------------------------------------------------------------------------

  macOS      brew install go
  Linux      https://go.dev/dl/
  Windows    https://go.dev/dl/

  Verify     go version

  Skip this if you already have a prebuilt greenagonia binary.


------------------------------------------------------------------------
 STEP 3 ── PagerDuty account
------------------------------------------------------------------------

  1. Sign up or log in:  https://www.pagerduty.com/
  2. Note your subdomain (the address bar shows
     https://<your-subdomain>.pagerduty.com).
  3. Confirm you're an Admin / Account Owner. The deploy creates a user,
     team, services, etc. — these all need write permission.


------------------------------------------------------------------------
 STEP 4 ── Allow the on-call email's domain
------------------------------------------------------------------------

  The deploy creates a NEW PagerDuty user with the email you supply
  (--email). PagerDuty must accept that email's domain.

  In the PagerDuty UI:

    Settings  →  Account Settings  →  Single Sign-On (or Domains)

  Make sure the domain of your --email value is on the allow-list
  (e.g. add @greenagonia.io if that's what you'll use).

  Also check: the email MUST NOT already exist as a user in the account.
  If it does, either delete the existing user first, or use a different
  email. (PagerDuty rejects duplicate emails.)


------------------------------------------------------------------------
 STEP 5 ── Generate a REST API token (and note your region)
------------------------------------------------------------------------

  In the PagerDuty UI:

    Integrations  →  API Access Keys  →  Create New API Key

  Settings:
    Description       greenagonia terraform
    Read/Write Access REQUIRED  (terraform creates resources)

  Copy the token immediately — PagerDuty only shows it once.

  Stash it for this shell session:

    export PAGERDUTY_TOKEN=<paste-token-here>

  Persist it: add the same export to ~/.zshrc / ~/.bashrc / your secrets
  manager. The CLI reads PAGERDUTY_TOKEN as the default for --api-token.

  Region check:
    US account     subdomain at *.pagerduty.com         → default
    EU account     subdomain at *.eu.pagerduty.com      → pass --region eu

  An EU account's token does NOT work against the US endpoint and vice
  versa — passing the wrong --region produces a 401 at apply time.


------------------------------------------------------------------------
 STEP 6 ── Build the greenagonia CLI (skip if you have a binary)
------------------------------------------------------------------------

  Native binary for your platform:

    make build           # → ./bin/greenagonia

  Cross-compile for every desktop platform:

    make build-all       # → ./bin/greenagonia-{darwin,linux,windows}-*

  Sanity check:

    ./bin/greenagonia help


------------------------------------------------------------------------
 STEP 7 ── Deploy
------------------------------------------------------------------------

    ./bin/greenagonia deploy \
        --env prod \
        --email oncall@greenagonia.io

  Optional flags:

    --region eu                        deploy to EU PagerDuty
                                       (default: us)
    --user-name "Site Reliability"     display name for the on-call user
    --with-workflows                   add Incident Workflows
                                       (paid feature — Business / Digital
                                       Operations plans only)

  What this creates:

    - 1 user, 1 team (Site Reliability Engineering),
      1 escalation policy (SRE Primary On-Call)
    - 8 technical services + Events API integrations
        payment-gateway, checkout-api, user-auth, product-catalog,
        recommendation-engine, search-service, order-service,
        notification-service
    - 2 business services with dependency graph
        customer-checkout, product-discovery
    - 1 event orchestration ("Greenagonia Event Router") +
      dynamic routing rules
    - 3 automation actions on every service
        Run Diagnostics, Rollback Deployment, Clear Down Settings
    - 1 catch-all service ("unrouted-events")
    - (3 incident workflows, only when --with-workflows is set)

  Routing key + per-service change-event keys are persisted to:

    ~/.greenagonia/<env>.json     (mode 0600)

  So 'scenarios run --env <env>' needs no further flags.


------------------------------------------------------------------------
 STEP 8 ── Drive incidents through it
------------------------------------------------------------------------

    ./bin/greenagonia scenarios list
    ./bin/greenagonia scenarios run --env prod --scenario bad-payment-deploy

  Each scenario fires three things in order:

    1. A simulated GitHub deploy (change event on the affected service).
       PagerDuty correlates it with the incidents that follow.
    2. A short pause so the change lands first.
    3. An alert cascade matching the bad-code story — typically the
       affected service goes critical, then downstream services follow.

  Useful flags:

    --repeat N    run the scenario N times back-to-back
    --no-delay    skip the realistic inter-step pauses
    --scenario all   run every scenario in sequence


------------------------------------------------------------------------
 STEP 9 ── Tear down
------------------------------------------------------------------------

    ./bin/greenagonia undeploy --env prod

  Add --with-workflows if you deployed with it. Removes the PagerDuty
  resources AND deletes ~/.greenagonia/<env>.json.


------------------------------------------------------------------------
 Troubleshooting
------------------------------------------------------------------------

  - 404 from /incident_workflows
        Your account lacks the Incident Workflows entitlement.
        Re-run deploy WITHOUT --with-workflows.

  - "user already exists"
        The --email value already exists in PagerDuty. Delete the
        existing user or supply a different email.

  - "domain not allowed"
        Add the email's domain to the allow-list as in Step 4.

  - Routing key missing
        Re-run deploy. The CLI re-writes ~/.greenagonia/<env>.json on
        every successful deploy.

  - Stuck partial deploy
        Re-run the same deploy command. Terraform reconciles state.


------------------------------------------------------------------------
 Layout
------------------------------------------------------------------------

  terraform/    main.tf, users.tf, services.tf, orchestration.tf,
                automation.tf, workflows.tf, variables.tf, outputs.tf
  cli/          go.mod, main.go (single-file Go, stdlib only)
  Makefile      build / build-all / clean / fmt / vet
  README.md     longer-form reference

========================================================================
`

// ---------------------------------------------------------------------------
// deploy / undeploy
// ---------------------------------------------------------------------------

func cmdDeploy(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	token := fs.String("api-token", os.Getenv("PAGERDUTY_TOKEN"), "PagerDuty API token")
	env := fs.String("env", "demo", "environment name (also the Terraform workspace)")
	region := fs.String("region", "us", "PagerDuty region: 'us' (default) or 'eu'")
	email := fs.String("email", "", "primary on-call user's email (required)")
	secondary := fs.String("secondary-email", "", "secondary on-call email (default: derive as <primary>+oncall2)")
	tertiary := fs.String("tertiary-email", "", "tertiary on-call email (default: derive as <primary>+oncall3)")
	userName := fs.String("user-name", "Greenagonia Oncall", "primary on-call user's display name")
	tfDir := fs.String("tf-dir", defaultTfDir(), "path to the terraform/ directory")
	withWorkflows := fs.Bool("with-workflows", false, "create the 3 Incident Workflows (requires Business/Digital-Ops plan)")
	_ = fs.Parse(args)

	if *token == "" {
		die("--api-token (or $PAGERDUTY_TOKEN) is required")
	}
	if *email == "" {
		die("--email is required")
	}
	reg, err := validRegion(*region)
	if err != nil {
		die(err.Error())
	}
	secEmail := *secondary
	if secEmail == "" {
		secEmail = derivePlusEmail(*email, "oncall2")
	}
	terEmail := *tertiary
	if terEmail == "" {
		terEmail = derivePlusEmail(*email, "oncall3")
	}

	banner()

	if err := runTerraform(*tfDir, *env, "apply", []string{
		"-auto-approve",
		"-input=false",
		"-var=pagerduty_token=" + *token,
		"-var=pagerduty_region=" + reg,
		"-var=environment=" + *env,
		"-var=user_email=" + *email,
		"-var=secondary_user_email=" + secEmail,
		"-var=tertiary_user_email=" + terEmail,
		"-var=user_name=" + *userName,
		fmt.Sprintf("-var=enable_incident_workflows=%t", *withWorkflows),
	}); err != nil {
		die(err.Error())
	}

	outs, err := terraformOutputs(*tfDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nwarn: could not read terraform outputs: %v\n", err)
		fmt.Println("deployed.")
		return
	}

	var routingKey string
	if raw, ok := outs["routing_key"]; ok {
		_ = json.Unmarshal(raw, &routingKey)
	}
	changeKeys := map[string]string{}
	if raw, ok := outs["change_event_keys"]; ok {
		_ = json.Unmarshal(raw, &changeKeys)
	}

	state := envState{
		Env:        *env,
		Region:     reg,
		RoutingKey: routingKey,
		ChangeKeys: changeKeys,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	path, err := saveState(state)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nwarn: could not save state file: %v\n", err)
	}

	// Pretty summary card
	maskedKey := maskKey(routingKey)
	pdHome := "https://app.pagerduty.com/"
	if reg == "eu" {
		pdHome = "https://app.eu.pagerduty.com/"
	}
	body := []string{
		fmt.Sprintf("%s   %s",
			green("✓"),
			bold("deployed")),
		"",
		fmt.Sprintf("    %s  %s", dim("env         "), pdGreen(*env)),
		fmt.Sprintf("    %s  %s", dim("region      "), bold(strings.ToUpper(reg))),
		fmt.Sprintf("    %s  %s", dim("routing key "), cyan(maskedKey)),
		fmt.Sprintf("    %s  %d %s", dim("change keys "), len(changeKeys), dim("services")),
		fmt.Sprintf("    %s  %s", dim("on-call     "), cyan("3 users on weekly rotation")),
		fmt.Sprintf("    %s    %s", dim("            "), dim("primary:   ")+*email),
		fmt.Sprintf("    %s    %s", dim("            "), dim("secondary: ")+secEmail),
		fmt.Sprintf("    %s    %s", dim("            "), dim("tertiary:  ")+terEmail),
	}
	if path != "" {
		body = append(body, fmt.Sprintf("    %s  %s", dim("state file  "), hyperlink("file://"+path, cyan(homeShort(path)))))
	}
	body = append(body, fmt.Sprintf("    %s  %s", dim("pagerduty   "), hyperlink(pdHome, cyan(pdHome))))
	body = append(body, "")
	body = append(body, "@DIV")
	body = append(body, "")
	body = append(body, dim("    try:"))
	body = append(body, "    "+pdGreen("›")+" greenagonia scenarios list")
	body = append(body, "    "+pdGreen("›")+" greenagonia scenarios run --env "+*env+" --scenario bad-payment-deploy")
	body = append(body, "    "+pdGreen("›")+" greenagonia web --env "+*env)
	body = append(body, "")

	fmt.Println()
	card("deploy complete", nil, body)
}

func cmdUndeploy(args []string) {
	fs := flag.NewFlagSet("undeploy", flag.ExitOnError)
	token := fs.String("api-token", os.Getenv("PAGERDUTY_TOKEN"), "PagerDuty API token")
	env := fs.String("env", "demo", "environment name")
	region := fs.String("region", "us", "PagerDuty region: 'us' (default) or 'eu'")
	email := fs.String("email", "noreply@example.com", "placeholder; ignored during destroy")
	userName := fs.String("user-name", "Greenagonia Oncall", "placeholder; ignored during destroy")
	tfDir := fs.String("tf-dir", defaultTfDir(), "path to the terraform/ directory")
	withWorkflows := fs.Bool("with-workflows", false, "match the value used at deploy time (terraform destroy ignores it for resources already in state, but the var must parse)")
	_ = fs.Parse(args)
	if *token == "" {
		die("--api-token (or $PAGERDUTY_TOKEN) is required")
	}
	reg, err := validRegion(*region)
	if err != nil {
		die(err.Error())
	}
	// If we have a state file, prefer its region (it knows what was deployed).
	if s, err := loadState(*env); err == nil && s.Region != "" {
		reg = s.Region
	}

	banner()

	if err := runTerraform(*tfDir, *env, "destroy", []string{
		"-auto-approve",
		"-input=false",
		"-var=pagerduty_token=" + *token,
		"-var=pagerduty_region=" + reg,
		"-var=environment=" + *env,
		"-var=user_email=" + *email,
		"-var=secondary_user_email=" + derivePlusEmail(*email, "oncall2"),
		"-var=tertiary_user_email=" + derivePlusEmail(*email, "oncall3"),
		"-var=user_name=" + *userName,
		fmt.Sprintf("-var=enable_incident_workflows=%t", *withWorkflows),
	}); err != nil {
		die(err.Error())
	}
	removeState(*env)

	fmt.Println()
	card("undeploy complete", nil, []string{
		fmt.Sprintf("%s   %s", green("✓"), bold("removed PagerDuty resources")),
		"",
		fmt.Sprintf("    %s  %s", dim("env        "), pdGreen(*env)),
		fmt.Sprintf("    %s  %s", dim("region     "), bold(strings.ToUpper(reg))),
		fmt.Sprintf("    %s  %s", dim("state file "), dim("deleted ("+homeShort(stateFilePath(*env))+")")),
		"",
	})
}

func runTerraform(dir, env, action string, args []string) error {
	if _, err := os.Stat(filepath.Join(dir, "main.tf")); err != nil {
		return fmt.Errorf("terraform dir %q doesn't look right (no main.tf): %w", dir, err)
	}
	if err := tfRun(dir, "init", "-input=false"); err != nil {
		return err
	}
	if err := tfRun(dir, "workspace", "select", env); err != nil {
		if err := tfRun(dir, "workspace", "new", env); err != nil {
			return err
		}
	}
	return tfRun(dir, append([]string{action}, args...)...)
}

func tfRun(dir string, args ...string) error {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// terraformOutputs returns every output as a json.RawMessage so callers can
// unmarshal scalars and maps with the same code path.
func terraformOutputs(dir string) (map[string]json.RawMessage, error) {
	cmd := exec.Command("terraform", "output", "-json")
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(errb.String()))
	}
	var raw map[string]struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("decoding terraform outputs: %w", err)
	}
	res := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		res[k] = v.Value
	}
	return res, nil
}

func defaultTfDir() string {
	if _, err := os.Stat("terraform/main.tf"); err == nil {
		return "terraform"
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "..", "terraform")
		if _, err := os.Stat(filepath.Join(cand, "main.tf")); err == nil {
			return cand
		}
	}
	return "terraform"
}

// ---------------------------------------------------------------------------
// scenarios
// ---------------------------------------------------------------------------

func cmdScenarios(args []string) {
	if len(args) < 1 {
		die("scenarios needs a subcommand: list | run")
	}
	switch args[0] {
	case "list":
		banner()
		fmt.Println("  " + bold("scenarios") + dim(fmt.Sprintf("  ·  %d available", len(scenarios))))
		fmt.Println()
		for _, s := range scenarios {
			renderScenarioCard(s)
			fmt.Println()
		}
	case "run":
		runScenariosCmd(args[1:])
	case "resolve":
		resolveScenariosCmd(args[1:])
	default:
		die("scenarios needs a subcommand: list | run | resolve")
	}
}

func resolveScenariosCmd(args []string) {
	fs := flag.NewFlagSet("scenarios resolve", flag.ExitOnError)
	env := fs.String("env", "demo", "environment name (reads ~/.greenagonia/<env>.json)")
	name := fs.String("scenario", "", "scenario name to resolve (or 'all')")
	_ = fs.Parse(args)

	if *name == "" {
		die("--scenario is required (use --scenario all to resolve every scenario)")
	}

	state, err := loadState(*env)
	if err != nil {
		die(fmt.Sprintf("no state for env %q (%v) — deploy first", *env, err))
	}
	if state.RoutingKey == "" {
		die("state file has no routing_key — re-deploy")
	}

	var targets []scenario
	if *name == "all" {
		targets = scenarios
	} else {
		sc := findScenario(*name)
		if sc == nil {
			die(fmt.Sprintf("unknown scenario %q (try `greenagonia scenarios list`)", *name))
		}
		targets = []scenario{*sc}
	}

	banner()
	fmt.Println("  " + bold("resolving") + dim(fmt.Sprintf("  ·  env=%s · region=%s", *env, strings.ToUpper(state.regionOf()))))
	fmt.Println()

	totalResolved, totalFailed := 0, 0
	start := time.Now()
	for _, sc := range targets {
		resolved, failed := 0, 0
		for i, st := range sc.steps {
			dk := dedupKey(state.Env, sc.name, st.service, i)
			if err := sendResolve(state.regionOf(), state.RoutingKey, dk); err != nil {
				failed++
				continue
			}
			resolved++
		}
		totalResolved += resolved
		totalFailed += failed

		mark := green("✓")
		status := fmt.Sprintf("%d resolved", resolved)
		if failed > 0 {
			mark = yellow("⚠")
			status = fmt.Sprintf("%d resolved, %d failed", resolved, failed)
		}
		fmt.Printf("  %s %s %s\n", mark, fmt.Sprintf("%-22s", sc.name), dim(status))
	}

	fmt.Println()
	body := []string{
		fmt.Sprintf("%s   %s resolves sent in %s", green("✓"), bold(strconv.Itoa(totalResolved)), bold(time.Since(start).Truncate(time.Second).String())),
	}
	if totalFailed > 0 {
		body = append(body, fmt.Sprintf("%s   %s failed", red("✗"), red(strconv.Itoa(totalFailed))))
	}
	body = append(body, "")
	body = append(body, dim("    PagerDuty deduplicates resolves: only incidents whose dedup_key"))
	body = append(body, dim("    matches a triggered alert will close. Already-resolved alerts"))
	body = append(body, dim("    return 202 too and are no-ops."))
	card("resolve complete", nil, body)
}

func renderScenarioCard(sc scenario) {
	badges := []string{}
	if sc.change != nil {
		badges = append(badges, sevChip("change"))
	}

	body := []string{}
	body = append(body, gray(sc.desc))
	body = append(body, "")

	if sc.change != nil {
		var tag, author, repo string
		var prNum any
		var prTitle string
		if v, ok := sc.change.custom["release_tag"].(string); ok {
			tag = v
		}
		if v, ok := sc.change.custom["author"].(string); ok {
			author = v
		}
		if v, ok := sc.change.custom["repository"].(string); ok {
			repo = v
		}
		if v, ok := sc.change.custom["pr_number"]; ok {
			prNum = v
		}
		if v, ok := sc.change.custom["pr_title"].(string); ok {
			prTitle = v
		}
		head := []string{}
		if tag != "" {
			head = append(head, bold(tag))
		}
		if author != "" {
			head = append(head, cyan(author))
		}
		if prNum != nil {
			head = append(head, fmt.Sprintf("PR #%v", prNum))
		}
		body = append(body, pdGreen("▸ change ")+" "+strings.Join(head, " · "))
		if prTitle != "" {
			body = append(body, "            "+dim(prTitle))
		}
		if repo != "" {
			body = append(body, "            "+dim(repo))
		}
		body = append(body, "")
	}

	seen := map[string]bool{}
	services := []string{}
	sevCount := map[string]int{}
	for _, st := range sc.steps {
		if !seen[st.service] {
			seen[st.service] = true
			services = append(services, st.service)
		}
		sevCount[st.severity]++
	}

	sevSummary := []string{}
	for _, s := range []string{"critical", "error", "warning", "info"} {
		if n := sevCount[s]; n > 0 {
			sevSummary = append(sevSummary, sevChip(s)+" "+fmt.Sprintf("×%d", n))
		}
	}

	body = append(body, bold("▸ alerts ")+"  "+fmt.Sprintf("%d across %d service%s  →  %s", len(sc.steps), len(services), plural(len(services)), green(fmt.Sprintf("~%d incident%s after grouping", len(services), plural(len(services))))))
	if len(sevSummary) > 0 {
		body = append(body, "            "+strings.Join(sevSummary, "  "))
	}
	body = append(body, "            "+dim(strings.Join(services, " · ")))

	card(sc.name, badges, body)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func runScenariosCmd(args []string) {
	fs := flag.NewFlagSet("scenarios run", flag.ExitOnError)
	env := fs.String("env", "demo", "environment name (reads ~/.greenagonia/<env>.json)")
	region := fs.String("region", "", "PagerDuty region 'us' or 'eu'; only used with --routing-key (state file otherwise wins)")
	keyOverride := fs.String("routing-key", os.Getenv("PD_ROUTING_KEY"), "routing key override; if set, change events are skipped")
	name := fs.String("scenario", "", "scenario name (or `all`)")
	repeat := fs.Int("repeat", 1, "run the scenario N times back-to-back")
	noDelay := fs.Bool("no-delay", false, "fire all steps immediately, ignoring per-step delays")
	_ = fs.Parse(args)

	if *name == "" {
		die("--scenario is required (try `greenagonia scenarios list`)")
	}

	// State strategy:
	//   1. Load state file (gives us routing_key + change_keys + region).
	//   2. --routing-key flag overrides routing_key; change events are skipped
	//      because we don't have per-service keys without state. --region
	//      picks the endpoint host when overriding.
	//   3. If neither is available, fail.
	state, err := loadState(*env)
	if err != nil && *keyOverride == "" {
		die(fmt.Sprintf("no state for env %q (%v) — deploy first, or pass --routing-key", *env, err))
	}
	if *keyOverride != "" {
		reg, rerr := validRegion(*region)
		if rerr != nil {
			die(rerr.Error())
		}
		if state == nil {
			state = &envState{Env: *env, ChangeKeys: map[string]string{}}
		}
		state.RoutingKey = *keyOverride
		state.ChangeKeys = nil // can't post change events without per-service keys
		state.Region = reg
	}
	if state.RoutingKey == "" {
		die("state file has no routing_key — re-deploy or pass --routing-key")
	}

	var targets []scenario
	if *name == "all" {
		targets = scenarios
	} else {
		sc := findScenario(*name)
		if sc == nil {
			die(fmt.Sprintf("unknown scenario %q (try `greenagonia scenarios list`)", *name))
		}
		targets = []scenario{*sc}
	}

	for i := 0; i < *repeat; i++ {
		for _, sc := range targets {
			if err := runScenario(state, sc, *noDelay); err != nil {
				die(err.Error())
			}
		}
	}
}

func findScenario(name string) *scenario {
	for i := range scenarios {
		if scenarios[i].name == name {
			return &scenarios[i]
		}
	}
	return nil
}

func runScenario(state *envState, sc scenario, noDelay bool) error {
	// Header card
	badges := []string{}
	if sc.change != nil {
		badges = append(badges, sevChip("change"))
	}
	card(sc.name, badges, []string{gray(sc.desc)})
	fmt.Println()

	// 1. Bad GitHub push (if the scenario has one).
	if sc.change != nil {
		key := state.ChangeKeys[sc.change.service]
		switch {
		case key == "" && len(state.ChangeKeys) == 0:
			fmt.Printf("  %s %s  %s\n", sevChip("change"), cyan(sc.change.service), dim("skipped (no per-service keys — using --routing-key?)"))
		case key == "":
			fmt.Printf("  %s %s  %s\n", sevChip("change"), cyan(sc.change.service), dim("skipped (no integration key for service)"))
		default:
			if err := sendChange(state.regionOf(), key, *sc.change); err != nil {
				return fmt.Errorf("change event for %s: %w", sc.change.service, err)
			}
			fmt.Printf("  %s %s  %s\n", sevChip("change"), cyan(sc.change.service), sc.change.summary)
			if !noDelay {
				time.Sleep(5 * time.Second)
			}
		}
		fmt.Println()
	}

	// 2. Alert cascade.
	fmt.Printf("  %s %s\n\n", bold("▸ firing"), dim(fmt.Sprintf("%d alert(s)", len(sc.steps))))
	start := time.Now()
	hosts := primaryHosts(sc)
	sentByService := map[string]int{}
	failed := 0
	for i, st := range sc.steps {
		if !noDelay {
			if d := st.delay - time.Since(start); d > 0 {
				time.Sleep(d)
			}
		}
		ev := pdEvent{
			RoutingKey:  state.RoutingKey,
			EventAction: "trigger",
			DedupKey:    dedupKey(state.Env, sc.name, st.service, i),
			Payload: payload{
				Summary:       st.summary,
				Source:        hosts[st.service],
				Severity:      st.severity,
				Component:     st.service,
				Group:         sc.name,
				Class:         "greenagonia-scenario",
				CustomDetails: mergeDetails(st.service, sc.name, hosts[st.service], st.source, st.desc, sc.shared, st.extra),
			},
		}
		err := sendAlert(state.regionOf(), ev)
		elapsed := time.Since(start).Truncate(time.Second)
		ts := gray(fmt.Sprintf("T+%-4s", elapsed))
		svc := cyan(fmt.Sprintf("%-22s", st.service))
		summary := st.summary
		if err != nil {
			failed++
			fmt.Printf("  %s %s %s %s\n", ts, sevChip(st.severity), svc, red("✗ "+err.Error()))
			continue
		}
		sentByService[st.service]++
		fmt.Printf("  %s %s %s %s  %s\n", ts, sevChip(st.severity), svc, summary, dim("· "+st.source))
	}

	// 3. Summary.
	total := time.Since(start).Truncate(time.Second)
	sent := len(sc.steps) - failed
	body := []string{
		fmt.Sprintf("%s   %s alerts fired in %s", green("✓"), bold(strconv.Itoa(sent)), bold(total.String())),
		fmt.Sprintf("    %s ~%d incidents after grouping (one per affected service)", dim("→"), len(sentByService)),
	}
	if failed > 0 {
		body = append(body, fmt.Sprintf("%s   %s alert(s) failed to post", red("✗"), red(strconv.Itoa(failed))))
	}
	body = append(body, "")
	body = append(body, dim("    open the queue: ")+hyperlink("https://app.pagerduty.com/incidents", cyan("app.pagerduty.com/incidents")))
	fmt.Println()
	card("complete", nil, body)
	return nil
}

// mergeDetails builds custom_details for an alert. Field priority (last
// wins): commonFields → serviceMeta[service] → scenario shared → identity →
// extra. `service` is the contract the orchestration router matches on
// (keep in sync with main.tf locals).
//
// `host` is the service's PRIMARY affected host — the same value for every
// alert on that service within a scenario, so the AIOps grouper sees exact
// host-field matches across the burst. The per-step origin is preserved
// separately as `reporting_host`.
func mergeDetails(service, scenarioName, host, reportingHost, description string, shared, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range commonFields {
		out[k] = v
	}
	if meta, ok := serviceMeta[service]; ok {
		for k, v := range meta {
			out[k] = v
		}
	}
	for k, v := range shared {
		out[k] = v
	}
	out["service"] = service
	out["scenario"] = scenarioName
	out["host"] = host
	out["reporting_host"] = reportingHost
	if description != "" {
		out["description"] = description
	}
	out["synthetic"] = true
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// primaryHosts maps each service in a scenario to its first step's source —
// the "primary affected host" that all of that service's alerts share.
func primaryHosts(sc scenario) map[string]string {
	hosts := map[string]string{}
	for _, st := range sc.steps {
		if _, ok := hosts[st.service]; !ok {
			hosts[st.service] = st.source
		}
	}
	return hosts
}

// ---------------------------------------------------------------------------
// HTTP — alerts and changes
// ---------------------------------------------------------------------------

var httpClient = &http.Client{Timeout: 15 * time.Second}

func sendAlert(region string, ev pdEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return postJSON(alertsURLFor(region), body)
}

// sendResolve closes an incident by sending an event_action=resolve with the
// same dedup_key the original trigger used. Payload isn't required by the
// Events API for resolves, so we send a minimal envelope.
func sendResolve(region, routingKey, dk string) error {
	body, err := json.Marshal(map[string]string{
		"routing_key":  routingKey,
		"event_action": "resolve",
		"dedup_key":    dk,
	})
	if err != nil {
		return err
	}
	return postJSON(alertsURLFor(region), body)
}

// derivePlusEmail inserts "+<tag>" before the "@" of an email, e.g.
// "you@x.io" + "oncall2" → "you+oncall2@x.io". Used to mint two extra
// PagerDuty user emails from the primary one — different PD users, same
// inbox via plus-addressing.
func derivePlusEmail(email, tag string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return email + "+" + tag
	}
	return email[:at] + "+" + tag + email[at:]
}

// dedupKey is the stable identity of a single alert in a scenario run. The
// same (env, scenario, service, step-index) tuple always produces the same
// key — so re-triggering re-uses the existing incident, and `scenarios
// resolve` can close it.
func dedupKey(env, scenarioName, service string, stepIdx int) string {
	return fmt.Sprintf("greenagonia/%s/%s/%s/%d", env, scenarioName, service, stepIdx)
}

func sendChange(region, integrationKey string, ch changeEvent) error {
	body, err := json.Marshal(pdChange{
		RoutingKey: integrationKey,
		Payload: changePayload{
			Summary:       ch.summary,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Source:        ch.sourceTool,
			CustomDetails: ch.custom,
		},
		Links: ch.links,
	})
	if err != nil {
		return err
	}
	return postJSON(changesURLFor(region), body)
}

func postJSON(url string, body []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s — %s", url, resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

// ---------------------------------------------------------------------------
// web — PagerDuty-styled browser UI for browsing and triggering scenarios.
// ---------------------------------------------------------------------------

func cmdWeb(args []string) {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address (host:port); if the port is taken, the next free one is used")
	maxTries := fs.Int("port-tries", 20, "how many consecutive ports to try if --addr is busy")
	env := fs.String("env", "demo", "env to use (reads ~/.greenagonia/<env>.json)")
	_ = fs.Parse(args)

	state, stateErr := loadState(*env)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, webHTML)
	})
	mux.HandleFunc("/api/state", apiStateHandler(*env, state))
	mux.HandleFunc("/api/scenarios", apiScenariosHandler)
	mux.HandleFunc("/api/scenarios/", apiScenarioSubpathHandler(state))

	ln, chosen, err := listenWithPortFallback(*addr, *maxTries)
	if err != nil {
		die(err.Error())
	}
	banner()
	url := fmt.Sprintf("http://localhost:%d", chosen)
	region := state.regionOf()
	fmt.Printf("  %s  %s  %s\n", pdGreen("▸ web"), bold(hyperlink(url, url)), dim(fmt.Sprintf("· env=%s · region=%s", *env, strings.ToUpper(region))))
	if stateErr != nil {
		fmt.Printf("  %s no state file for env %q — UI loads but trigger buttons are disabled.\n", yellow("⚠"), *env)
	}
	fmt.Println()
	if err := http.Serve(ln, mux); err != nil {
		die(err.Error())
	}
}

// listenWithPortFallback binds the requested addr; if the port is in use,
// it tries the next ones (up to maxTries) before giving up. Returns the
// chosen port for printing.
func listenWithPortFallback(addr string, maxTries int) (net.Listener, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	startPort, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid port in --addr %q: %w", addr, err)
	}
	if maxTries < 1 {
		maxTries = 1
	}
	var lastErr error
	for i := 0; i < maxTries; i++ {
		port := startPort + i
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			if i > 0 {
				fmt.Printf("port %d was busy — using %d instead.\n", startPort, port)
			}
			return ln, port, nil
		}
		lastErr = err
		if !isAddrInUse(err) {
			return nil, 0, err
		}
	}
	return nil, 0, fmt.Errorf("no free port in [%d..%d]: %w", startPort, startPort+maxTries-1, lastErr)
}

func isAddrInUse(err error) bool {
	return err != nil && strings.Contains(err.Error(), "address already in use")
}

func apiStateHandler(env string, state *envState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		out := map[string]any{"env": env, "deployed": state != nil}
		if state != nil {
			out["routing_key_masked"] = maskKey(state.RoutingKey)
			out["change_keys_count"] = len(state.ChangeKeys)
			out["updated_at"] = state.UpdatedAt
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return strings.Repeat("•", len(k))
	}
	return k[:4] + "…" + k[len(k)-4:]
}

func apiScenariosHandler(w http.ResponseWriter, r *http.Request) {
	type stepJSON struct {
		DelaySec int    `json:"delay_sec"`
		Service  string `json:"service"`
		Severity string `json:"severity"`
	}
	type scenarioJSON struct {
		Name        string         `json:"name"`
		Title       string         `json:"title"`
		Impact      string         `json:"impact"`
		Affects     []string       `json:"affects"`
		Desc        string         `json:"desc"`        // technical line for "details" view
		Change      map[string]any `json:"change"`      // simplified: who deployed what
		Services    []string       `json:"services"`    // technical service IDs (kebab-case)
		ServicesFriendly map[string]string `json:"services_friendly"` // kebab → "Title Case"
		AlertsPerService map[string]int    `json:"alerts_per_service"`
		Steps       []stepJSON     `json:"steps"`       // minimal — for modal progress
	}
	out := make([]scenarioJSON, 0, len(scenarios))
	for _, s := range scenarios {
		title := s.title
		if title == "" {
			title = s.name
		}
		impact := s.impact
		if impact == "" {
			impact = s.desc
		}
		sj := scenarioJSON{
			Name:    s.name,
			Title:   title,
			Impact:  impact,
			Affects: s.affects,
			Desc:    s.desc,
		}
		if s.change != nil {
			ch := map[string]any{}
			if v, ok := s.change.custom["release_tag"].(string); ok && v != "" {
				ch["version"] = v
			}
			if v, ok := s.change.custom["author"].(string); ok && v != "" {
				ch["author"] = v
			}
			if v, ok := s.change.custom["pr_number"]; ok {
				ch["pr_number"] = v
			}
			if v, ok := s.change.custom["pr_title"].(string); ok && v != "" {
				ch["pr_title"] = v
			}
			ch["service"] = s.change.service
			sj.Change = ch
		}
		seen := map[string]bool{}
		perSvc := map[string]int{}
		friendly := map[string]string{}
		for _, st := range s.steps {
			perSvc[st.service]++
			if !seen[st.service] {
				seen[st.service] = true
				sj.Services = append(sj.Services, st.service)
				friendly[st.service] = titleCaseKebab(st.service)
			}
			sj.Steps = append(sj.Steps, stepJSON{
				DelaySec: int(st.delay / time.Second),
				Service:  st.service,
				Severity: st.severity,
			})
		}
		sj.AlertsPerService = perSvc
		sj.ServicesFriendly = friendly
		out = append(out, sj)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func titleCaseKebab(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		// keep common acronyms upper-cased
		switch strings.ToLower(p) {
		case "api", "db", "ui", "id":
			parts[i] = strings.ToUpper(p)
		default:
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// /api/scenarios/<name>/run     — SSE stream that fires the scenario
// /api/scenarios/<name>/resolve  — SSE stream that closes its incidents
func apiScenarioSubpathHandler(state *envState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/scenarios/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		name, action := parts[0], parts[1]
		sc := findScenario(name)
		if sc == nil {
			http.Error(w, "unknown scenario", http.StatusNotFound)
			return
		}
		if state == nil {
			http.Error(w, "no state file — deploy first", http.StatusPreconditionFailed)
			return
		}
		switch action {
		case "run":
			streamScenarioRun(w, r, *sc, state)
		case "resolve":
			streamScenarioResolve(w, r, *sc, state)
		default:
			http.NotFound(w, r)
		}
	}
}

func streamScenarioResolve(w http.ResponseWriter, r *http.Request, sc scenario, state *envState) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, payload any) {
		b, err := json.Marshal(payload)
		if err != nil {
			b = []byte(`{}`)
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	send("info", map[string]any{"message": fmt.Sprintf("resolving %s (%d step(s))", sc.name, len(sc.steps))})

	resolved, failed := 0, 0
	for i, st := range sc.steps {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		dk := dedupKey(state.Env, sc.name, st.service, i)
		if err := sendResolve(state.regionOf(), state.RoutingKey, dk); err != nil {
			failed++
			send("err", map[string]any{"message": fmt.Sprintf("%s/%d: %v", st.service, i, err)})
			continue
		}
		resolved++
		send("resolved", map[string]any{
			"service": st.service,
			"source":  st.source,
			"summary": st.summary,
			"step":    i,
		})
	}
	send("done", map[string]any{"resolved": resolved, "failed": failed})
}

func streamScenarioRun(w http.ResponseWriter, r *http.Request, sc scenario, state *envState) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, payload any) {
		b, err := json.Marshal(payload)
		if err != nil {
			b = []byte(`{}`)
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	send("info", map[string]any{"message": fmt.Sprintf("starting %s (%d alert step(s))", sc.name, len(sc.steps))})

	// 1. Change event (if any).
	if sc.change != nil {
		key := state.ChangeKeys[sc.change.service]
		if key == "" {
			send("err", map[string]any{"message": fmt.Sprintf("no change-event key for service %s", sc.change.service)})
		} else if err := sendChange(state.regionOf(), key, *sc.change); err != nil {
			send("err", map[string]any{"message": fmt.Sprintf("change event: %v", err)})
		} else {
			send("change", map[string]any{
				"service": sc.change.service,
				"summary": sc.change.summary,
			})
			select {
			case <-time.After(3 * time.Second):
			case <-r.Context().Done():
				return
			}
		}
	}

	// 2. Alert cascade.
	start := time.Now()
	hosts := primaryHosts(sc)
	for i, st := range sc.steps {
		if d := st.delay - time.Since(start); d > 0 {
			select {
			case <-time.After(d):
			case <-r.Context().Done():
				return
			}
		}
		ev := pdEvent{
			RoutingKey:  state.RoutingKey,
			EventAction: "trigger",
			DedupKey:    dedupKey(state.Env, sc.name, st.service, i),
			Payload: payload{
				Summary:       st.summary,
				Source:        hosts[st.service],
				Severity:      st.severity,
				Component:     st.service,
				Group:         sc.name,
				Class:         "greenagonia-scenario",
				CustomDetails: mergeDetails(st.service, sc.name, hosts[st.service], st.source, st.desc, sc.shared, st.extra),
			},
		}
		if err := sendAlert(state.regionOf(), ev); err != nil {
			send("err", map[string]any{"message": fmt.Sprintf("alert %s: %v", st.service, err)})
			continue
		}
		send("step", map[string]any{
			"elapsed_sec": int(time.Since(start) / time.Second),
			"service":     st.service,
			"severity":    st.severity,
			"summary":     st.summary,
			"source":      st.source,
		})
	}

	send("done", map[string]any{"elapsed_sec": int(time.Since(start) / time.Second)})
}
