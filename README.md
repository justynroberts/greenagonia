# Greenagonia

Greenagonia is PagerDuty's fictional demo company. This repo stands up a
**shared, multi-admin** Greenagonia environment in one PagerDuty account:
each admin on your team gets their own full demo stack (services, schedules,
escalation policy, event orchestration) and their own personal link to the
hosted storefront. When a customer checkout fails in the storefront, an
incident opens on that admin's services and pages them — nobody else.

---

## Install

### Linux x86_64 — one script

```bash
curl -fsSL https://raw.githubusercontent.com/justynroberts/greenagonia/master/install.sh | bash
```

`install.sh` downloads the latest pre-built `greenagonia-linux-amd64` binary
to `/usr/local/bin/greenagonia` and installs Terraform 1.9.8 if it is not
already present.

### macOS — prereqs + build

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia && ./quickstart.sh
```

`quickstart.sh` checks/installs Terraform and Go via Homebrew, then builds the
binary to `./greenagonia` (repo root).

### Build from source

```bash
cd greenagonia
make build          # generates site/scenarios.json, compiles native binary
make build-all      # cross-compiles for macOS (arm64/amd64), Linux (amd64/arm64), Windows
make clean          # remove compiled binaries
```

`make build` runs two steps in sequence:

1. `greenagonia scenarios dump-json > site/scenarios.json` — exports scenario
   definitions from the Go source (single source of truth).
2. `go build` — compiles the binary to `./greenagonia` (repo root).

Add the repo to your PATH so you can run `greenagonia` from anywhere:

```bash
export PATH="$PATH:$(pwd)"           # from inside the repo directory
echo 'export PATH="$PATH:$HOME/path/to/greenagonia"' >> ~/.zshrc   # permanent
```

---

## CLI reference

### First-run

```bash
greenagonia setup
```

Interactive wizard. Collects: PagerDuty REST API token, PagerDuty region
(global / eu), time zone for schedules, storefront base URL, optional Slack
configuration, and details for the first admin. Writes
`terraform-shared/secrets.auto.tfvars.json` (chmod 600, gitignored) and
`terraform-shared/config.auto.tfvars.json` (gitignored).

### Tokens and secrets

```bash
greenagonia token           # set or replace the PagerDuty REST API token
greenagonia user-token      # set the PagerDuty user-level token (required for Slack connections)
greenagonia slack-token     # set the Slack bot token
```

Each command prompts for the value with hidden input and writes it to the
gitignored secrets file.

### Admin management

```bash
greenagonia admin add <INI> "Full Name" <email> [slack-email]
greenagonia admin add JR "Justyn Roberts" justyn@example.com
greenagonia admin add JR "Justyn Roberts" justyn@example.com justyn@company.slack.com

greenagonia admin remove JR      # marks JR for destruction on next deploy
greenagonia admin list           # show all configured admins with initials and email
```

Initials are 2–4 uppercase letters and prefix every resource that belongs to
that admin. If the requested initials are already taken, the CLI derives free
ones from the name automatically: `JR → JRO → JROB → JOR …`. It tells you
what it chose. Re-adding the same email under the same initials is an update,
not a collision.

The optional `slack-email` argument is the email used in the Slack workspace
(only needed when it differs from the PagerDuty email).

### Deploy and destroy

```bash
greenagonia deploy      # terraform init → plan → confirm → apply
greenagonia destroy     # destroy the entire environment (prompts for confirmation)
```

`deploy` prints each admin's personal storefront link at the end. Plans are
additive by admin: adding or removing one admin touches only that admin's
resources; everything else is untouched.

### URLs and site

```bash
greenagonia urls            # show all per-admin storefront links and routing keys
greenagonia urls JR         # show just one admin
greenagonia site-url        # show the current storefront base URL
greenagonia site-url http://3.85.144.140    # set the storefront base URL
```

After changing the site URL, run `greenagonia deploy` so the outputs reflect
the new base.

### Slack channels

```bash
greenagonia slack-channels  # create {ini}-incidents channels for all admins
```

Creates the per-admin Slack channels and wires up the PagerDuty Slack
connection. Requires `slack-token` and `user-token` to be set. You can also
let `greenagonia deploy` handle this — `slack-channels` is a convenience
shortcut.

### Scenarios

```bash
greenagonia scenarios list        # browse available scenarios with descriptions
greenagonia scenarios dump-json   # export scenario data as JSON (used by make build)
```

The shared environment is driven exclusively by the storefront. The storefront
fires the `bad-payment-deploy` scenario; `scenarios list` shows what the
binary knows about all scenarios. There are no `run` or `resolve` commands in
this environment — use the storefront or resolve incidents directly in
PagerDuty.

---

## Architecture

### What each admin gets

For initials `JR`:

| Resource | Name |
|---|---|
| User | their real name and email — gets the actual pages |
| Team | `JR-SRE-TEAM` (admin = manager, 5 personas = responders) |
| 8 technical services | `JR-payment-gateway`, `JR-checkout-api`, `JR-user-auth`, `JR-product-catalog`, `JR-recommendation-engine`, `JR-search-service`, `JR-order-service`, `JR-notification-service` |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` |
| 3 schedules | `JR Primary On-Call`, `JR Secondary On-Call`, `JR Tertiary On-Call` |
| Escalation policy | `JR SRE On-Call` (primary → secondary → tertiary, 2 loops) |
| Event orchestration | `Greenagonia Event Router — JR` with a personal routing key |
| (optional) Slack | `jr-incidents` channel + PD Slack connection |

The primary schedule has the admin on call Mon–Fri 09:00–17:00 (configurable
time zone, default `Europe/London`). The five shared personas cover nights,
weekends, and all secondary/tertiary rotations.

**Isolation:** each admin has the Restricted Access PagerDuty role, scoped to
their team. They see their own stack and the shared personas; they cannot see
any other admin's resources. The Terraform operator's REST token sees and
manages everything.

### Shared resources

Created once, visible to all admins:

| Resource | Notes |
|---|---|
| 5 personas | Sarah SRE, Dan Developer, Matt Manager, Pablo Platform, Sam Security — `@greenagonia.io` emails, pages don't deliver |
| Greenagonia team | all admins are managers; all personas are responders |
| 5 platform services | `api-gateway`, `data-platform`, `identity-service`, `infrastructure`, `platform-engineering` |
| Greenagonia Platform Router | shared event orchestration with its own routing key |
| `unrouted-events` | catch-all service for events that don't match any rule |
| 4 automation actions | Run Diagnostics, Rollback Deployment, Clear Down Settings, Isolate & Capture Forensics — bound to every technical service |
| (optional) 4 incident workflows | Major Incident Escalation, Security Incident Response, Post to Status Page, Rollback Deployment — requires Incident Workflows entitlement |
| (optional) `greenagonia-incidents` Slack channel | shared channel for the whole Greenagonia team |

### How routing works

Each admin has **one inbound routing key** (their event orchestration). The
storefront posts alerts to that key with `custom_details.service` set to a
bare service name (e.g. `payment-gateway`). The orchestration router matches
on that field and routes to the admin's prefixed service
(`JR-payment-gateway`), which lights up `JR-customer-checkout`.

Anything that does not match any rule lands on the shared `unrouted-events`
catch-all.

The routing key alone decides whose stack lights up. Two people can demo
simultaneously from the same hosted storefront — they just use different
`?pdkey=` links.

### Change events

When a checkout fails, the storefront first posts two change events (backdated
to simulate a real deployment sequence):

| Event | Backdated | Summary |
|---|---|---|
| GitHub deploy | −3 min | `payment-service v2.41.0` deployed to production |
| LaunchDarkly flag | −2 min | `checkout-v2-enabled` flag enabled for all users |

These appear on the incident's *Recent Changes* tab. If a change key is
missing, the event is silently skipped — the alert still fires.

---

## Storefront

The storefront (`site/`) is a zero-build static site. It reads
`site/scenarios.json` at runtime to display scenario details.

```bash
# local laptop demo
cd site && python3 -m http.server 8080
# open http://localhost:8080/?pdkey=<your-routing-key>
```

For a shared team deployment, host the contents of `site/` on any static host
(GitHub Pages, S3, an EC2 instance) and tell the CLI the URL:

```bash
greenagonia site-url http://3.85.144.140
greenagonia deploy      # re-runs so outputs include the new base URL
greenagonia urls        # prints updated per-admin links
```

Opening the `?pdkey=` URL stores the routing key in the browser's
`localStorage` and scrubs it from the address bar. It is a one-time action
per browser. The ops console (Ctrl/Cmd+Shift+K, or the footer link) shows all
configured keys and a log of events dispatched in the current session.

HTTP and HTTPS both work. Avoid `file://` — CORS blocks the `?pdkey=` URL
pre-loading.

---

## State management

Terraform state lives in `state/` (gitignored). One person runs applies at a
time. To back up state:

```bash
./backup-state.sh
```

Creates a dated `.tar.gz` in `state/backups/` and retains the 10 most recent
backups. If you need concurrent applies across multiple operators, migrate to a
remote backend with locking (HCP Terraform free tier: create a workspace, add
a `cloud {}` block to `terraform-shared/main.tf`, run `terraform init` and
approve the migration).

---

## Repo layout

```
greenagonia/              this repo — justynroberts/greenagonia
├── README.md             this file
├── ADMIN-GUIDE.md        step-by-step admin walkthrough
├── CLAUDE.md             context for Claude Code
├── Makefile              make build / build-all / clean / generate-scenarios
├── install.sh            install binary + terraform on Linux x86_64
├── backup-state.sh       back up state/ to a dated tar.gz
├── quickstart.sh         macOS prereqs + build
├── site/                 storefront source (static site)
│   ├── index.html
│   ├── chaos.js          chaos engine; fetches scenarios.json at runtime
│   ├── scenarios.json    generated by make build; committed for static hosting
│   └── ...
├── state/                terraform state dir (gitignored)
│   └── backups/          created by backup-state.sh
├── terraform-shared/     shared environment Terraform
│   ├── main.tf           provider + locals (service catalogue, personas)
│   ├── users.tf
│   ├── teams.tf
│   ├── schedules.tf
│   ├── escalation_policies.tf
│   ├── services.tf
│   ├── business_services.tf
│   ├── orchestration.tf
│   ├── greenagonia_platform.tf
│   ├── slack_channels.tf
│   ├── automation.tf
│   ├── workflows.tf
│   ├── variables.tf
│   └── outputs.tf
└── cli/
    ├── go.mod            stdlib only (plus golang.org/x/term)
    ├── main.go           scenarios + colour helpers + terraform helpers
    └── shared.go         environment management commands
```
