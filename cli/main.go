// greenagonia — manage the Greenagonia shared demo environment.
//
// Subcommands:
//
//	greenagonia shared setup
//	greenagonia shared token | user-token | slack-token
//	greenagonia shared admin add <INI> "Name" <email> [slack-email]
//	greenagonia shared admin remove <INI> | list
//	greenagonia shared deploy | destroy
//	greenagonia shared urls [INITIALS]
//	greenagonia shared site-url [URL]
//	greenagonia shared slack-channels
//	greenagonia scenarios list
//	greenagonia scenarios dump-json
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// ===========================================================================
// Style — TTY-aware colours, hyperlinks, boxed cards
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

func red(s string) string         { return ansi("31", s) }
func green(s string) string       { return ansi("32", s) }
func yellow(s string) string      { return ansi("33", s) }
func cyan(s string) string        { return ansi("36", s) }
func gray(s string) string        { return ansi("90", s) }
func bold(s string) string        { return ansi("1", s) }
func dim(s string) string         { return ansi("2", s) }
func pdGreen(s string) string     { return ansi("38;5;35;1", s) }
func pdGreenSoft(s string) string { return ansi("38;5;35", s) }

func sevChip(sev string) string {
	const w = 8
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

func hyperlink(url, text string) string {
	if noColor {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

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

func card(title string, badges []string, body []string) {
	const w = 96
	inner := w - 2
	contentW := w - 4

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

func homeShort(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" && strings.HasPrefix(p, h) {
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}

// ===========================================================================
// Scenarios — data types and rendering
// ===========================================================================

type step struct {
	delay    time.Duration
	service  string
	summary  string
	desc     string
	severity string
	source   string
	extra    map[string]any
}

type changeLink struct {
	Href string `json:"href"`
	Text string `json:"text,omitempty"`
}

type changeEvent struct {
	service    string
	summary    string
	sourceTool string
	agoMinutes int
	custom     map[string]any
	links      []changeLink
}

func (c changeEvent) ago(min int) changeEvent {
	c.agoMinutes = min
	return c
}

type scenarioUI struct {
	FailStep    string `json:"fail_step"`
	SlowFactor  int    `json:"slow_factor"`
	ErrorCode   string `json:"error_code"`
	Component   string `json:"component"`
	UserMessage string `json:"user_message"`
}

type scenario struct {
	name    string
	desc    string
	title   string
	impact  string
	affects []string
	ui      scenarioUI
	shared  map[string]any
	changes []changeEvent
	steps   []step
}

var techServices = []string{
	"payment-gateway", "checkout-api", "user-auth", "product-catalog",
	"recommendation-engine", "search-service", "order-service", "notification-service",
}

var commonFields = map[string]any{
	"environment":       "production",
	"region":            "us-east-1",
	"cluster":           "use1-prod-1",
	"kubernetes":        true,
	"monitoring_system": "datadog",
	"datacenter":        "aws-use1",
	"runbook_url":       map[string]any{"github": "https://raw.githubusercontent.com/greenagonia/runbooks/refs/heads/main/runbook.md"},
}

var serviceMeta = map[string]map[string]any{
	"payment-gateway":       {"namespace": "payments", "deployment": "payment-gateway", "team": "platform-payments", "runbook": "https://wiki.greenagonia.io/runbooks/payment-gateway"},
	"checkout-api":          {"namespace": "commerce", "deployment": "checkout-api", "team": "checkout", "runbook": "https://wiki.greenagonia.io/runbooks/checkout-api"},
	"user-auth":             {"namespace": "identity", "deployment": "user-auth", "team": "identity", "runbook": "https://wiki.greenagonia.io/runbooks/user-auth"},
	"product-catalog":       {"namespace": "catalog", "deployment": "product-catalog", "team": "catalog", "runbook": "https://wiki.greenagonia.io/runbooks/product-catalog"},
	"recommendation-engine": {"namespace": "recommendations", "deployment": "recommendation-engine", "team": "data-platform", "runbook": "https://wiki.greenagonia.io/runbooks/recommendation-engine"},
	"search-service":        {"namespace": "search", "deployment": "search-service", "team": "search", "runbook": "https://wiki.greenagonia.io/runbooks/search-service"},
	"order-service":         {"namespace": "commerce", "deployment": "order-service", "team": "orders", "runbook": "https://wiki.greenagonia.io/runbooks/order-service"},
	"notification-service":  {"namespace": "platform", "deployment": "notification-service", "team": "platform-notifications", "runbook": "https://wiki.greenagonia.io/runbooks/notification-service"},
}

func ghChange(service, repo, version, author string, prNumber int, prTitle, sha string, extra map[string]any) changeEvent {
	short := sha
	if len(short) > 7 {
		short = sha[:7]
	}
	custom := map[string]any{
		"repository": repo, "branch": "main", "commit_sha": sha, "commit_short": short,
		"author": author, "pr_number": prNumber, "pr_title": prTitle,
		"deployed_to": "production", "release_tag": version, "deploy_status": "success",
		"service": service, "workflow": "deploy.yml", "trigger": "merge_to_main",
	}
	for k, v := range extra {
		custom[k] = v
	}
	return changeEvent{
		service: service, summary: fmt.Sprintf("Deployed %s %s → production (PR #%d: %s)", service, version, prNumber, prTitle),
		sourceTool: "GitHub Actions", custom: custom,
		links: []changeLink{
			{Href: fmt.Sprintf("https://github.com/%s/pull/%d", repo, prNumber), Text: fmt.Sprintf("PR #%d", prNumber)},
			{Href: fmt.Sprintf("https://github.com/%s/commit/%s", repo, short), Text: "Commit " + short},
			{Href: fmt.Sprintf("https://github.com/%s/actions/workflows/deploy.yml", repo), Text: "CI deploy workflow"},
		},
	}
}

func ldChange(service, flagKey, change, author string, oldValue, newValue string, extra map[string]any) changeEvent {
	custom := map[string]any{
		"source": "launchdarkly", "flag_key": flagKey, "environment": "production",
		"previous_value": oldValue, "new_value": newValue, "changed_by": author, "service": service,
	}
	for k, v := range extra {
		custom[k] = v
	}
	return changeEvent{
		service: service, summary: fmt.Sprintf("Feature flag %s %s (%s → %s)", flagKey, change, oldValue, newValue),
		sourceTool: "LaunchDarkly", custom: custom,
		links: []changeLink{
			{Href: "https://app.launchdarkly.com/projects/greenagonia/flags/" + flagKey, Text: "Flag: " + flagKey},
			{Href: "https://app.launchdarkly.com/projects/greenagonia/audit", Text: "LaunchDarkly audit log"},
		},
	}
}

func alertStep(delaySec int, svc, summary, desc, severity, source string, extra map[string]any) step {
	return step{delay: time.Duration(delaySec) * time.Second, service: svc, summary: summary, desc: desc, severity: severity, source: source, extra: extra}
}

var scenarios = []scenario{
	{
		name: "bad-payment-deploy", title: "Payment Outage",
		impact:  "Customers can't complete payment at checkout. Orders are failing and confirmation emails are backing up.",
		affects: []string{"Customer Checkout", "Order Processing"},
		desc:    "payment-gateway v2.4.1 cascade — payment, checkout, orders, notifications.",
		ui:      scenarioUI{FailStep: "payment", SlowFactor: 4, ErrorCode: "GATEWAY_TIMEOUT_504", Component: "payment-gateway", UserMessage: "Our payment provider is taking too long to respond. You have not been charged."},
		shared:  map[string]any{"release": "v2.4.1", "deploy_id": "github-deploy-412", "root_cause_service": "payment-gateway"},
		changes: []changeEvent{
			ghChange("payment-gateway", "greenagonia/payment-gateway", "v2.4.1", "alice@greenagonia.io", 412, "Refactor Amex card auth flow", "a3f9c12ee45b8a2c1d9f8b3c7e5a9d1f2b6c4e8a", map[string]any{"reviewers": []string{"bob", "carol"}, "files_changed": 14, "lines_added": 287, "lines_removed": 41}).ago(22),
			ldChange("payment-gateway", "amex-new-auth-flow", "ramped", "alice@greenagonia.io", "25% rollout", "100% rollout", map[string]any{"flag_kind": "release", "targeting": "all card-brand=amex traffic"}).ago(9),
		},
		steps: []step{
			alertStep(0, "payment-gateway", "Authentication failures elevated", "Card authorization requests failing for the majority of Amex transactions since the v2.4.1 deploy.", "critical", "payment-gw-prod-3", map[string]any{"release": "v2.4.1", "error_class": "java.lang.NullPointerException", "card_brand": "amex"}),
			alertStep(5, "payment-gateway", "Card processor timeouts", "Calls to the Adyen processor exceeding the 10s timeout; retries are compounding the load.", "critical", "payment-gw-prod-7", map[string]any{"processor": "adyen", "timeout_count": 184}),
			alertStep(10, "payment-gateway", "Service health check failing", "The /healthz endpoint has failed 12 consecutive probes; instances are being pulled from the load balancer.", "critical", "payment-gw-prod-3", map[string]any{"endpoint": "/healthz", "consecutive_failures": 12}),
			alertStep(14, "payment-gateway", "Error budget burn-rate exceeded", "Availability SLO burning 14x faster than sustainable; budget exhausted in under 2 hours at this rate.", "error", "slo-monitor", map[string]any{"slo": "payment-gateway-availability", "burn_rate_x": 14}),
			alertStep(20, "checkout-api", "Upstream payment timeouts", "Requests to payment-gateway timing out at p99 of 9.8s; checkout requests queuing behind them.", "error", "checkout-prod-7", map[string]any{"upstream": "payment-gateway", "timeout_p99_ms": 9800}),
			alertStep(25, "checkout-api", "Checkout completion rate dropped", "Completed checkouts down from ~280/min to 12/min; customers abandoning after payment errors.", "error", "checkout-prod-7", map[string]any{"complete_per_min": 12, "baseline_per_min": 280}),
			alertStep(30, "checkout-api", "Cart abandonment elevated", "Real-user monitoring shows 64% of carts abandoned at the payment step.", "warning", "rum-monitor", map[string]any{"abandonment_pct": 64}),
			alertStep(36, "order-service", "Failed-order rate above baseline", "Orders failing at 412/min against a 34/min baseline; failures correlate with payment declines.", "error", "order-prod-2", map[string]any{"failed_per_min": 412, "baseline_per_min": 34}),
			alertStep(41, "order-service", "Order processing degraded", "Order pipeline p99 at 8.2s; payment-confirmation steps dominating processing time.", "error", "order-prod-2", map[string]any{"p99_ms": 8200}),
			alertStep(46, "order-service", "Downstream payment errors propagating", "79% of order failures trace to upstream payment-gateway errors.", "error", "order-prod-5", map[string]any{"upstream_error_pct": 79}),
			alertStep(52, "notification-service", "Confirmation email queue backlog growing", "order_confirmations queue depth at 18,450 and climbing; emails delayed until orders clear.", "warning", "notif-prod-1", map[string]any{"queue": "order_confirmations", "queue_depth": 18450}),
			alertStep(58, "notification-service", "Notification delivery delayed", "Delivery delayed by more than 60s; current lag is 4 minutes behind real time.", "warning", "notif-prod-3", map[string]any{"delivery_lag_sec": 240}),
		},
	},
	{
		name: "db-migration-fail", title: "Login Failure",
		impact:  "Customers can't log in. Product browsing and search are degraded across the site.",
		affects: []string{"Customer Login", "Browse & Discover"},
		desc:    "Long-running schema migration locks user_session — auth, catalog, search, recs degrade.",
		ui:      scenarioUI{FailStep: "validate", SlowFactor: 3, ErrorCode: "AUTH_SERVICE_UNAVAILABLE", Component: "user-auth", UserMessage: "We couldn't verify your account details. Please try again in a moment."},
		shared:  map[string]any{"migration_id": "0427_session_uuid", "deploy_id": "github-deploy-1207", "root_cause_service": "user-auth"},
		changes: []changeEvent{ghChange("user-auth", "greenagonia/user-auth", "v4.11.0", "bob@greenagonia.io", 1207, "Migrate user_session.id from BIGINT to UUID", "7b2e9d34c8a1f6b5e3d2c7a9b8f1e6d4c3b2a1f0", map[string]any{"migration_id": "0427_session_uuid", "estimated_runtime_sec": 45, "actual_runtime_sec": ">300 (still running)"}).ago(12)},
		steps: []step{
			alertStep(0, "user-auth", "Login latency elevated", "Login p99 at 14s against a 320ms baseline; the user_session table is locked by a running migration.", "critical", "auth-prod-4", map[string]any{"p99_ms": 14000, "baseline_p99_ms": 320}),
			alertStep(5, "user-auth", "Connection pool saturated", "All 200 database connections in use; new login attempts queuing until connections free up.", "critical", "auth-prod-4", map[string]any{"pool_in_use": 200, "pool_size": 200}),
			alertStep(10, "user-auth", "Session creation failing", "INSERTs into user_session blocked by migration 0427_session_uuid holding a table lock.", "critical", "auth-prod-1", map[string]any{"db_lock_table": "user_session", "migration_id": "0427_session_uuid"}),
			alertStep(14, "user-auth", "Auth health check timing out", "The /healthz probe exceeding its 30s timeout because the readiness query is stuck behind the lock.", "error", "auth-prod-1", map[string]any{"endpoint": "/healthz", "timeout_ms": 30000}),
			alertStep(20, "product-catalog", "Database read timeouts elevated", "38% of catalog reads timing out; shared database under contention from the auth migration.", "error", "catalog-prod-9", map[string]any{"read_timeout_pct": 38}),
			alertStep(26, "product-catalog", "Cache miss rate elevated", "Cache misses at 71%; refresh queries failing so stale entries are being evicted without replacement.", "error", "catalog-prod-5", map[string]any{"miss_pct": 71}),
			alertStep(32, "product-catalog", "Catalog query failures", "28% of product queries returning errors after exhausting database retries.", "error", "catalog-prod-9", map[string]any{"query_error_pct": 28}),
			alertStep(38, "search-service", "Empty result rate elevated", "22% of searches returning zero results because the catalog backend is unavailable.", "error", "search-prod-2", map[string]any{"empty_result_pct": 22, "cause": "catalog backend unavailable"}),
			alertStep(44, "search-service", "Index lookups failing", "1,840 index lookups per minute failing on catalog enrichment calls.", "error", "search-prod-1", map[string]any{"failed_lookups_per_min": 1840}),
			alertStep(50, "search-service", "Search latency degraded", "Search p99 at 5.2s while enrichment calls wait on catalog timeouts.", "warning", "search-prod-2", map[string]any{"p99_ms": 5200}),
			alertStep(56, "recommendation-engine", "Falling back to default recommendations", "Personalised recommendations unavailable; 100% of traffic now served generic defaults.", "warning", "recs-prod-5", map[string]any{"fallback_active": true, "fallback_pct": 100}),
			alertStep(62, "recommendation-engine", "Personalization pipeline degraded", "4 pipeline stages stalled waiting on user-profile reads that depend on the locked session store.", "warning", "recs-prod-3", map[string]any{"pipeline_stalled_steps": 4}),
		},
	},
	{
		name: "memory-leak-recs", title: "Recommendations Crashing",
		impact:  "Product recommendations are unavailable. Search is slower and the catalog is overloaded.",
		affects: []string{"Browse & Discover"},
		desc:    "recommendation-engine v3.2 leaks heap — OOM-kill loop drives load into search & catalog.",
		ui:      scenarioUI{FailStep: "inventory", SlowFactor: 3, ErrorCode: "INVENTORY_SERVICE_TIMEOUT", Component: "recommendation-engine", UserMessage: "We couldn't load product availability right now. Please try again shortly."},
		shared:  map[string]any{"release": "v3.2.0", "deploy_id": "github-deploy-89", "root_cause_service": "recommendation-engine"},
		changes: []changeEvent{ghChange("recommendation-engine", "greenagonia/recommendation-engine", "v3.2.0", "carol@greenagonia.io", 89, "Add cohort-based ranking experiment", "e1d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0", map[string]any{"feature_flag": "cohort_ranking_v3", "rollout_pct": 100, "experiment_owner": "data-platform"}).ago(18)},
		steps: []step{
			alertStep(0, "recommendation-engine", "OOMKilled containers", "Containers killed at the 8GB memory limit 3 times in 5 minutes following the v3.2 deploy.", "critical", "recs-prod-1", map[string]any{"release": "v3.2", "restart_count": 3, "window_min": 5}),
			alertStep(5, "recommendation-engine", "Restart loop detected", "Pods restarting 8 times per minute; each instance leaks heap and dies within seconds of serving traffic.", "critical", "recs-prod-1", map[string]any{"restarts_per_min": 8}),
			alertStep(10, "recommendation-engine", "Memory pressure critical", "Heap at 7.8GB of an 8GB ceiling within 90 seconds of startup; allocation profile points at the new ranking cache.", "critical", "recs-prod-2", map[string]any{"heap_used_mb": 7820, "heap_max_mb": 8192}),
			alertStep(14, "recommendation-engine", "Health endpoint not responding", "The /healthz endpoint unreachable; instances dying before the readiness window completes.", "error", "recs-prod-2", map[string]any{"endpoint": "/healthz"}),
			alertStep(20, "search-service", "Search latency degraded", "Search p99 at 4.2s (baseline 850ms); recommendation sidecar calls timing out and retrying.", "error", "search-prod-1", map[string]any{"p99_ms": 4200, "p50_ms": 920}),
			alertStep(26, "search-service", "Tail latency elevated", "p999 at 12s; worst-case requests waiting through multiple recommendation retries.", "error", "search-prod-3", map[string]any{"p999_ms": 12000}),
			alertStep(32, "search-service", "Queue depth growing", "8,400 requests queued; capacity consumed by slow downstream calls.", "warning", "search-prod-1", map[string]any{"queue_depth": 8400}),
			alertStep(38, "product-catalog", "Request rate above baseline", "Traffic at 3x baseline as search falls back to direct catalog queries.", "warning", "catalog-prod-2", map[string]any{"rps": 4200, "baseline_rps": 1400}),
			alertStep(44, "product-catalog", "Cache hit ratio dropped", "Hit ratio down to 41%; fallback queries have a different access pattern than the warmed cache.", "warning", "catalog-prod-4", map[string]any{"hit_ratio_pct": 41}),
			alertStep(50, "product-catalog", "Database load elevated", "Database CPU at 87% absorbing the cache misses.", "warning", "catalog-prod-2", map[string]any{"db_cpu_pct": 87}),
		},
	},
	{
		name: "config-push-gateway", title: "Site-Wide Authentication Failure",
		impact:  "Site-wide impact. Customers can't log in, pay, or browse — everything is returning 401.",
		affects: []string{"Customer Login", "Customer Checkout", "Browse & Discover"},
		desc:    "Bad API-gateway config drops the Authorization header — every downstream 401s.",
		ui:      scenarioUI{FailStep: "validate", SlowFactor: 2, ErrorCode: "AUTHENTICATION_REQUIRED", Component: "user-auth", UserMessage: "We couldn't verify your account details. Please try again in a moment."},
		shared:  map[string]any{"config_version": "gateway-cfg-2026.06.09-3", "deploy_id": "github-deploy-88", "root_cause_service": "api-gateway"},
		changes: []changeEvent{ghChange("user-auth", "greenagonia/api-gateway-config", "cfg-2026.06.09-3", "dave@greenagonia.io", 88, "Tighten upstream header allowlist", "f5e4d3c2b1a0f9e8d7c6b5a4f3e2d1c0b9a8f7e6", map[string]any{"config_file": "routes/auth.yaml", "change_type": "header_allowlist", "header_removed": "Authorization", "rollout_minutes": 0}).ago(6)},
		steps: []step{
			alertStep(0, "user-auth", "Authorization failures elevated", "71% of requests rejected with 401 since gateway config push; the Authorization header is being stripped upstream.", "critical", "auth-prod-1", map[string]any{"config_version": "gateway-cfg-2026.06.09-3", "rate_401_pct": 71}),
			alertStep(4, "user-auth", "Session validation failing", "Session validation calls arriving with no Authorization header; every validation attempt rejected.", "critical", "auth-prod-2", map[string]any{"cause": "missing Authorization header"}),
			alertStep(8, "user-auth", "Token verification errors", "Token verifier raising MissingAuthHeader for all inbound traffic routed through the gateway.", "critical", "auth-prod-3", map[string]any{"error_class": "MissingAuthHeader"}),
			alertStep(13, "user-auth", "Auth health check failing", "Authenticated health probe failing — the probe's own credentials are stripped by the gateway too.", "critical", "auth-prod-1", map[string]any{"endpoint": "/healthz"}),
			alertStep(18, "checkout-api", "Authentication errors elevated", "98% of checkout calls receiving 401 from downstream auth; no customer can reach payment.", "critical", "checkout-prod-1", map[string]any{"rate_401_pct": 98}),
			alertStep(23, "checkout-api", "Checkout failures across the board", "99% of checkout attempts failing; every authenticated step in the flow is rejected.", "critical", "checkout-prod-1", map[string]any{"failure_pct": 99}),
			alertStep(28, "checkout-api", "Service unavailable to clients", "All client traffic effectively blocked; circuit breakers opening across checkout instances.", "critical", "checkout-prod-3", map[string]any{"clients_affected": "all"}),
			alertStep(34, "order-service", "Cannot process orders", "Order processing halted; user-context lookups required for every order are failing.", "critical", "order-prod-1", map[string]any{"orders_blocked": true}),
			alertStep(39, "order-service", "Session lookup failing", "Session lookups against user-auth returning 401; orders cannot be attributed to customers.", "critical", "order-prod-2", map[string]any{"cause": "user-auth 401"}),
			alertStep(44, "order-service", "Order intake blocked", "New order intake suspended to prevent unattributed orders from queueing.", "critical", "order-prod-1", map[string]any{}),
			alertStep(50, "product-catalog", "Authentication required errors", "Personalised-pricing requests rejected with 401; anonymous browsing still functional.", "error", "catalog-prod-1", map[string]any{}),
			alertStep(56, "product-catalog", "Personalised endpoints failing", "All endpoints requiring a user context are failing; serving non-personalised fallbacks.", "error", "catalog-prod-3", map[string]any{}),
		},
	},
	{
		name: "cache-stampede-search", title: "Search Slowdown",
		impact:  "Search is slow and product browsing is degraded. Recommendations are serving stale results.",
		affects: []string{"Browse & Discover"},
		desc:    "Bad TTL change in search-service — cache stampede overloads catalog.",
		ui:      scenarioUI{FailStep: "inventory", SlowFactor: 3, ErrorCode: "CATALOG_SERVICE_TIMEOUT", Component: "search-service", UserMessage: "We couldn't check item availability right now. Please try again shortly."},
		shared:  map[string]any{"release": "v1.8.0", "deploy_id": "github-deploy-304", "root_cause_service": "search-service"},
		changes: []changeEvent{ghChange("search-service", "greenagonia/search-service", "v1.8.0", "eve@greenagonia.io", 304, "Tighten cache TTL to improve freshness", "c4b3a2f1e0d9c8b7a6f5e4d3c2b1a0f9e8d7c6b5", map[string]any{"old_ttl_sec": 60, "new_ttl_sec": 5, "intent": "improve search result freshness"}).ago(15)},
		steps: []step{
			alertStep(0, "search-service", "Cache hit ratio dropped", "Hit ratio collapsed from 94% to 12% after v1.8.0 cut cache TTL from 60s to 5s.", "critical", "search-prod-3", map[string]any{"release": "v1.8.0", "hit_ratio_pct": 12, "change": "ttl 60s → 5s"}),
			alertStep(5, "search-service", "Backend QPS elevated", "7,100 queries/sec hitting the catalog backend — formerly absorbed by cache.", "critical", "search-prod-3", map[string]any{"backend_qps": 7100}),
			alertStep(10, "search-service", "Search latency degraded", "Search p99 at 1.8s; nearly every query now pays the full backend round-trip.", "critical", "search-prod-1", map[string]any{"p99_ms": 1800}),
			alertStep(14, "search-service", "Connection pool saturated", "All 64 backend connections in use; queries queueing for a free connection.", "error", "search-prod-2", map[string]any{"pool_in_use": 64, "pool_size": 64}),
			alertStep(20, "product-catalog", "Read load critical", "Read traffic at 5x baseline from the search cache stampede.", "critical", "catalog-prod-4", map[string]any{"qps": 7100, "baseline_qps": 1400}),
			alertStep(26, "product-catalog", "Database connections elevated", "412 database connections in use; approaching the cluster ceiling.", "error", "catalog-prod-4", map[string]any{"db_connections_in_use": 412}),
			alertStep(32, "product-catalog", "Query latency degraded", "Catalog query p99 at 1.8s under the sustained read load.", "error", "catalog-prod-2", map[string]any{"p99_ms": 1800}),
			alertStep(38, "recommendation-engine", "Upstream catalog timeouts", "31% of catalog enrichment calls timing out under load.", "warning", "recs-prod-2", map[string]any{"upstream": "product-catalog", "timeout_pct": 31}),
			alertStep(44, "recommendation-engine", "Stale recommendations served", "41% of recommendation responses served from stale snapshots while fresh reads time out.", "warning", "recs-prod-1", map[string]any{"stale_pct": 41}),
			alertStep(50, "recommendation-engine", "Personalization degraded", "Live personalisation disabled; serving cached cohort defaults until catalog recovers.", "warning", "recs-prod-3", map[string]any{"fallback_active": true}),
		},
	},
	{
		name: "k8s-bad-image-rollout", title: "Bad Deploy Crashed Pods",
		impact:  "Recommendation pods are crash-looping. Browse experience is degraded.",
		affects: []string{"Browse & Discover"},
		desc:    "Bad container image rollout — recommendation-engine pods CrashLoopBackOff, downstream degrades.",
		ui:      scenarioUI{FailStep: "inventory", SlowFactor: 3, ErrorCode: "RECOMMENDATION_SERVICE_UNAVAILABLE", Component: "recommendation-engine", UserMessage: "We couldn't load your personalised selections. Please try again shortly."},
		shared:  map[string]any{"release": "v3.3.0", "deploy_id": "argocd-sync-2026.06.09-04", "replicaset": "recommendation-engine-7d4f9c", "root_cause_service": "recommendation-engine"},
		changes: []changeEvent{ghChange("recommendation-engine", "greenagonia/recommendation-engine", "v3.3.0", "frank@greenagonia.io", 94, "Switch to slimmer alpine base image", "b9a8f7e6d5c4b3a2f1e0d9c8b7a6f5e4d3c2b1a0", map[string]any{"image": "ghcr.io/greenagonia/recommendation-engine:v3.3.0", "image_size_mb": 187, "previous_image": "ghcr.io/greenagonia/recommendation-engine:v3.2.0", "deployer": "argocd", "k8s_cluster": "use1-prod-1", "k8s_namespace": "recommendations", "sync_revision": "argocd-sync-2026.06.09-04"}).ago(8)},
		steps: []step{
			alertStep(0, "recommendation-engine", "Pod CrashLoopBackOff", "Pod killed with exit code 137 (OOM) 6 times; the v3.3.0 alpine image is missing the jemalloc the service tunes for.", "critical", "recommendation-engine-7d4f9c-x2k8m", map[string]any{"pod_phase": "CrashLoopBackOff", "restart_count": 6, "exit_code": 137, "replicaset": "recommendation-engine-7d4f9c", "image": "ghcr.io/greenagonia/recommendation-engine:v3.3.0"}),
			alertStep(4, "recommendation-engine", "Pod CrashLoopBackOff", "Second pod of the new ReplicaSet in CrashLoopBackOff with the same exit code 137 signature.", "critical", "recommendation-engine-7d4f9c-p9m4n", map[string]any{"pod_phase": "CrashLoopBackOff", "restart_count": 5, "exit_code": 137, "replicaset": "recommendation-engine-7d4f9c"}),
			alertStep(8, "recommendation-engine", "Pod CrashLoopBackOff", "Third pod crash-looping — the entire new ReplicaSet is failing identically; zero healthy replicas.", "critical", "recommendation-engine-7d4f9c-q3w8j", map[string]any{"pod_phase": "CrashLoopBackOff", "restart_count": 4, "exit_code": 137, "replicaset": "recommendation-engine-7d4f9c"}),
			alertStep(12, "recommendation-engine", "Liveness probe failing", "Liveness probe failed 3 consecutive times; kubelet restarting the container on a backoff timer.", "error", "recommendation-engine-7d4f9c-x2k8m", map[string]any{"probe": "liveness", "endpoint": "/healthz", "consecutive_failures": 3, "replicaset": "recommendation-engine-7d4f9c"}),
			alertStep(16, "recommendation-engine", "ReplicaSet rollout stalled", "ReplicaSet at 0 of 3 ready replicas; rollout cannot make progress.", "error", "kube-controller-manager", map[string]any{"replicaset": "recommendation-engine-7d4f9c", "ready_replicas": 0, "desired_replicas": 3}),
			alertStep(20, "recommendation-engine", "Deployment rollout timed out", "Progress deadline exceeded; ArgoCD reports the sync OutOfSync and is awaiting manual intervention.", "error", "argocd", map[string]any{"progress_deadline_exceeded": true, "sync_status": "OutOfSync"}),
			alertStep(28, "search-service", "Upstream recommendations unavailable", "Circuit breaker to recommendation-engine open; no healthy upstream endpoints to route to.", "error", "search-prod-2", map[string]any{"upstream": "recommendation-engine", "circuit_breaker": "open"}),
			alertStep(34, "search-service", "Fallback responses elevated", "100% of search responses using non-personalised fallback ranking.", "error", "search-prod-2", map[string]any{"fallback_pct": 100}),
			alertStep(40, "search-service", "Latency degraded", "Search p99 at 1.8s; fallback path is slower than the cached recommendation flow.", "warning", "search-prod-3", map[string]any{"p99_ms": 1800}),
			alertStep(46, "product-catalog", "Request rate above baseline", "Traffic at 2.3x baseline as fallback ranking queries the catalog directly.", "warning", "catalog-prod-2", map[string]any{"rps": 3200, "baseline_rps": 1400}),
			alertStep(52, "product-catalog", "Cache pressure elevated", "Cache evicting 28 entries/min under the shifted access pattern.", "warning", "catalog-prod-4", map[string]any{"eviction_rate_per_min": 28}),
			alertStep(58, "product-catalog", "Database load elevated", "Database CPU at 71% absorbing fallback traffic.", "warning", "catalog-prod-2", map[string]any{"db_cpu_pct": 71}),
		},
	},
	{
		name: "noisy-neighbour", title: "Background System Noise",
		impact:  "Low-priority alerts from across the platform — used to test that the router and grouping handle volume cleanly.",
		affects: []string{"Platform-Wide"},
		desc:    "Low-priority noise spread across every service — stress-tests the router.",
		ui:      scenarioUI{FailStep: "order", SlowFactor: 2, ErrorCode: "ORDER_SERVICE_DEGRADED", Component: "order-service", UserMessage: "Something went wrong creating your order. You have not been charged."},
		changes: []changeEvent{ghChange("notification-service", "greenagonia/notification-service", "v5.2.1", "grace@greenagonia.io", 451, "Bump dependencies (routine weekly update)", "d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3", map[string]any{"files_changed": 2, "lines_added": 18, "lines_removed": 18, "risk": "low", "automated": true}).ago(24)},
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

// ===========================================================================
// CLI entry
// ===========================================================================

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "shared":
		cmdShared(os.Args[2:])
	case "scenarios":
		cmdScenarios(os.Args[2:])
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
  greenagonia shared setup                                          first-run wizard
  greenagonia shared token | user-token | slack-token
  greenagonia shared admin add <INI> "Name" <email> [slack-email]
  greenagonia shared admin remove <INI> | list
  greenagonia shared deploy | destroy
  greenagonia shared urls [INITIALS]
  greenagonia shared site-url [URL]
  greenagonia shared slack-channels

  greenagonia scenarios list
  greenagonia scenarios dump-json

environment variables:
  NO_COLOR    disable colour output`)
}

// ===========================================================================
// Scenarios commands
// ===========================================================================

func cmdScenarios(args []string) {
	if len(args) < 1 {
		die("scenarios needs a subcommand: list | dump-json")
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
	case "dump-json":
		dumpScenariosJSON()
	default:
		die("scenarios needs a subcommand: list | dump-json")
	}
}

// dump-json output types

type scenariosDoc struct {
	Version     string                    `json:"version"`
	Common      map[string]any            `json:"common"`
	ServiceMeta map[string]map[string]any `json:"service_meta"`
	Scenarios   []scenarioOut             `json:"scenarios"`
}

type scenarioOut struct {
	Name    string         `json:"name"`
	Title   string         `json:"title"`
	Impact  string         `json:"impact"`
	Affects []string       `json:"affects"`
	Desc    string         `json:"desc"`
	UI      scenarioUI     `json:"ui"`
	Shared  map[string]any `json:"shared"`
	Changes []changeOut    `json:"changes"`
	Steps   []stepOut      `json:"steps"`
}

type changeOut struct {
	Service    string         `json:"service"`
	Summary    string         `json:"summary"`
	SourceTool string         `json:"source_tool"`
	AgoMinutes int            `json:"ago_minutes"`
	Custom     map[string]any `json:"custom"`
	Links      []changeLink   `json:"links"`
}

type stepOut struct {
	DelaySec int            `json:"delay_sec"`
	Service  string         `json:"service"`
	Summary  string         `json:"summary"`
	Desc     string         `json:"desc"`
	Severity string         `json:"severity"`
	Source   string         `json:"source"`
	Extra    map[string]any `json:"extra"`
}

func dumpScenariosJSON() {
	doc := scenariosDoc{Version: "1", Common: commonFields, ServiceMeta: serviceMeta}
	for _, sc := range scenarios {
		out := scenarioOut{Name: sc.name, Title: sc.title, Impact: sc.impact, Affects: sc.affects, Desc: sc.desc, UI: sc.ui, Shared: sc.shared}
		for _, ch := range sc.changes {
			out.Changes = append(out.Changes, changeOut{Service: ch.service, Summary: ch.summary, SourceTool: ch.sourceTool, AgoMinutes: ch.agoMinutes, Custom: ch.custom, Links: ch.links})
		}
		for _, st := range sc.steps {
			extra := st.extra
			if extra == nil {
				extra = map[string]any{}
			}
			out.Steps = append(out.Steps, stepOut{DelaySec: int(st.delay / time.Second), Service: st.service, Summary: st.summary, Desc: st.desc, Severity: st.severity, Source: st.source, Extra: extra})
		}
		doc.Scenarios = append(doc.Scenarios, out)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

func renderScenarioCard(sc scenario) {
	badges := []string{}
	if len(sc.changes) > 0 {
		badges = append(badges, sevChip("change"))
	}

	body := []string{}
	body = append(body, gray(sc.desc))
	body = append(body, "")

	for _, ch := range sc.changes {
		var tag, author, repo, prTitle string
		var prNum any
		if v, ok := ch.custom["release_tag"].(string); ok {
			tag = v
		}
		if v, ok := ch.custom["author"].(string); ok {
			author = v
		} else if v, ok := ch.custom["changed_by"].(string); ok {
			author = v
		}
		if v, ok := ch.custom["repository"].(string); ok {
			repo = v
		}
		if v, ok := ch.custom["pr_number"]; ok {
			prNum = v
		}
		if v, ok := ch.custom["pr_title"].(string); ok {
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
		if flag, ok := ch.custom["flag_key"].(string); ok {
			head = append(head, bold("flag:"+flag))
		}
		head = append(head, dim(fmt.Sprintf("%s · %dm ago", ch.sourceTool, ch.agoMinutes)))
		body = append(body, pdGreen("▸ change ")+" "+strings.Join(head, " · "))
		if prTitle != "" {
			body = append(body, "            "+dim(prTitle))
		}
		if repo != "" {
			body = append(body, "            "+dim(repo))
		}
	}
	if len(sc.changes) > 0 {
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

// ===========================================================================
// Terraform helpers (used by shared.go)
// ===========================================================================

func tfRun(dir string, args ...string) error {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

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

func die(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}
