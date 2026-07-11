# Greenagonia Shared Environment — Admin Guide

How to stand up, operate, and demo the shared Greenagonia environment.
Written for colleagues who have not seen this repo before.

---

## What this is

Greenagonia is a fictional outdoor-gear company used for PagerDuty demos. The
shared environment gives **each admin their own full demo stack inside one
PagerDuty account**, driven by a hosted e-commerce storefront whose checkout
fails on demand.

```
site/                               the storefront (static site, runs anywhere)
        │  1. POST change events (GitHub deploy, LaunchDarkly flag — backdated)
        │  2. POST alert event → your personal routing key
        ▼
Greenagonia Event Router — JR       your event orchestration
        ▼
JR-payment-gateway                  your service → incident → pages YOU
        ▼
JR-customer-checkout                your business service lights up
```

Everyone shares a cast of five fictional responders — Sarah SRE, Dan
Developer, Matt Manager, Pablo Platform, Sam Security — who staff every
team's schedules so the account looks like a real company. There is also a
shared Greenagonia team and five platform services visible to everyone.

### What you get as an admin

For your initials (example: `JR`):

| Resource | Name |
|---|---|
| User | your real name and email — gets actual pages |
| Team | `JR-SRE-TEAM` (you = manager, personas = responders) |
| 8 technical services | `JR-payment-gateway`, `JR-checkout-api`, `JR-user-auth`, `JR-product-catalog`, `JR-recommendation-engine`, `JR-search-service`, `JR-order-service`, `JR-notification-service` |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` |
| 3 schedules | `JR Primary On-Call`, `JR Secondary On-Call`, `JR Tertiary On-Call` |
| Escalation policy | `JR SRE On-Call` (primary → secondary → tertiary, 2 loops) |
| Event orchestration | `Greenagonia Event Router — JR` + your personal routing key |
| (optional) Slack | `jr-incidents` permanent channel + PD Slack connection |

Your **primary schedule has you on call Mon–Fri 09:00–17:00** (configurable
time zone, default `Europe/London`); the personas cover nights and weekends
and staff the secondary and tertiary rotations. Pages to persona emails are
fictional and deliver nowhere.

**Isolation:** your PagerDuty login has the Restricted Access role, scoped to
your team. You see your own team, services, schedules, orchestration and
incidents — plus the shared personas — and nothing belonging to other admins.
Inside your team you are a manager, so you can adjust anything that is yours.

---

## Prerequisites

- 🟢 macOS or Linux with `terraform` (≥ 1.5) on PATH
- 🟢 The `greenagonia` binary — see Install below
- 🟢 A PagerDuty account where you have admin access and a REST API key with
  read/write scope (Integrations → API Access Keys)
- 🟢 The email address you want to use must not already exist as a user in
  the PagerDuty account
- 🟡 Optional: AIOps add-on (content-based alert grouping), Incident
  Workflows entitlement, Slack V2 integration with bot token and user-level
  API token

---

## First-time setup

### Step 1 — Install

**macOS:**

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia && ./quickstart.sh
```

`quickstart.sh` installs Terraform and Go via Homebrew if missing, then
builds the binary to `./greenagonia` (repo root).

**Linux x86_64:**

```bash
curl -fsSL https://raw.githubusercontent.com/justynroberts/greenagonia/master/install.sh | bash
```

Downloads the pre-built binary to `/usr/local/bin/greenagonia` and installs
Terraform.

**Build from source (any platform):**

```bash
cd greenagonia && make build
```

Binary lands at `./greenagonia` (repo root). Add it to your PATH so you can
run it from anywhere:

```bash
# from inside the repo directory:
export PATH="$PATH:$(pwd)"
# make permanent:
echo 'export PATH="$PATH:$HOME/path/to/greenagonia"' >> ~/.zshrc
```

Confirm it works:

```bash
greenagonia --help
```

### Step 2 — Run the setup wizard

```bash
greenagonia setup
```

The wizard walks through, in order:

1. PagerDuty REST API token (hidden input)
2. PagerDuty region — `global` (default) or `eu` for `*.eu.pagerduty.com`
   accounts
3. Schedule time zone (default `Europe/London`)
4. Storefront base URL — where you plan to host `site/`; can be updated later
5. Whether to enable Incident Workflows (requires entitlement; say `n` if
   unsure)
6. Whether to enable Slack (requires Slack V2 integration; say `n` if unsure)
7. Details for the first admin: initials, full name, email

When it finishes, two gitignored files exist in `terraform-shared/`:

- `secrets.auto.tfvars.json` — tokens (chmod 600)
- `config.auto.tfvars.json` — admins, time zone, site URL, feature flags

### Step 3 — Add the first admin (if not done in setup)

```bash
greenagonia admin add JR "Justyn Roberts" justyn@example.com
```

If those initials are already taken, the CLI derives free alternatives
automatically (`JR → JRO → JROB`) and tells you what it chose. Re-running
with the same email and initials is an update, not an error.

### Step 4 — Deploy

```bash
greenagonia deploy
```

This runs `terraform init`, shows the plan, asks for confirmation, then
applies. On a first deploy with one admin, expect around 60–70 resources and
2–4 minutes.

At the end it prints each admin's personal storefront link, for example:

```
JR  http://3.85.144.140/?pdkey=R0abcd1234...
```

### Step 5 — Get per-admin URLs

```bash
greenagonia urls        # all admins
greenagonia urls JR     # one admin
```

Shows the routing key (for the storefront's `?pdkey=` parameter) and, if
change event integrations exist, the GitHub and LaunchDarkly change keys.

### Step 6 — Host the storefront

The storefront is a zero-build static site. Pick one:

**Laptop demo (localhost):**

```bash
cd site && python3 -m http.server 8080
# open http://localhost:8080/?pdkey=<routing-key>
```

**Shared team deployment (host once, share the URL):**

Copy the contents of `site/` to any static host (GitHub Pages, S3, EC2).
Tell the CLI the URL:

```bash
greenagonia site-url http://3.85.144.140
greenagonia deploy          # re-applies; outputs include the new base URL
greenagonia urls            # prints updated per-admin links
```

Opening a `?pdkey=` URL stores the routing key in `localStorage` and removes
it from the address bar. That is a one-time action per browser.

---

## Day-to-day operations

### Add an admin

```bash
greenagonia admin add AB "Alice Bell" alice@example.com
greenagonia deploy
```

The plan touches only Alice's new resources. All other admins are untouched.

### Remove an admin

```bash
greenagonia admin remove AB
greenagonia deploy
```

The plan destroys only AB's stack. Every other admin's resources are
untouched.

### List admins

```bash
greenagonia admin list
```

### Replace the PagerDuty token

```bash
greenagonia token
greenagonia deploy      # applies the new token
```

### Update the storefront URL

```bash
greenagonia site-url https://demo.example.com
greenagonia deploy
greenagonia urls        # confirm the updated links
```

---

## Running a demo

1. **Get your personal link.** Run `greenagonia urls JR` or use the link
   printed at the end of deploy. It looks like
   `http://<site>/?pdkey=R0…`.

2. **Open the storefront.** The routing key is stored in `localStorage` on
   first open; the `?pdkey=` parameter disappears from the URL bar after that.

3. **Trigger the failure.** The front page leads with a checkout card.
   Click **Pay**. The order pipeline runs and fails at "Charging payment".
   Before the alert fires, the storefront posts two change events backdated
   ~2–3 minutes:

   | Change event | Backdated | Summary |
   |---|---|---|
   | GitHub deploy | −3 min | `payment-service v2.41.0` deployed to production |
   | LaunchDarkly flag | −2 min | `checkout-v2-enabled` flag enabled for all users |

4. **In PagerDuty:** an incident opens on `JR-payment-gateway`, pages whoever
   is on call (you, during working hours). `JR-customer-checkout` shows
   impact. The *Recent Changes* tab shows the two change events. If Slack is
   configured, `jr-incidents` gets a notification.

5. **Repeated checkouts dedupe** into the same open incident (stable dedup
   key), so you can retry without flooding the queue.

6. **Resolve** the incident in PagerDuty as normal, or use **Resolve all**
   in the storefront ops console.

### The storefront ops console

The storefront looks like a normal shop. To reach the controls:

- Press **Ctrl/Cmd + Shift + K**, or click the small **"operations console"**
  link in the footer, or triple-click the footer logo.
- **Double-click the header logo** to change or remove the stored routing key.
- The console shows: routing key, change event key (GitHub), change event key
  (LaunchDarkly) — all pre-loaded from the `?pdkey=` URL.
- An event log shows every event dispatched in the current browser session.

The pre-load URL format with all keys:

```
http://<site>/?pdkey=<routing-key>&pdchangekey=<github-key>&pdldkey=<ld-key>
```

`greenagonia urls JR` shows all three keys.

---

## Slack setup

Slack is optional. Set it up after the initial deploy if you decide you want
it.

### What you need

| Requirement | Where to get it |
|---|---|
| Slack V2 integration installed in PagerDuty | PagerDuty → Integrations → Slack V2 |
| Slack bot token (`xoxb-…`) | Slack app → OAuth & Permissions → Bot User OAuth Token |
| PagerDuty user-level token | PagerDuty → My Profile → User Settings → Create API User Token |
| Slack workspace ID (starts with `T`) | PagerDuty → Integrations → Slack V2 → workspace ID |

Required Slack bot scopes: `channels:manage`, `channels:join`, `users:read`,
`users:read.email`.

### Configuration steps

```bash
greenagonia slack-token     # paste Slack bot token (xoxb-…)
greenagonia user-token      # paste PagerDuty user-level token
greenagonia setup           # or edit config.auto.tfvars.json: set enable_slack = true
greenagonia deploy
```

After deploy, each admin has a `{ini}-incidents` Slack channel with a
PagerDuty Slack connection that posts triggered/acknowledged/resolved/
escalated events. The shared `greenagonia-incidents` channel receives events
for the shared Greenagonia team.

### If channels already exist in the workspace

Terraform will fail with `name_taken`. Import the existing channels first:

```bash
# find the channel ID
curl -s -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  "https://slack.com/api/conversations.list" \
  | jq '.channels[] | select(.name=="jr-incidents") | .id'

terraform -chdir=terraform-shared import 'slack_conversation.incidents["JR"]' <channel-id>
```

Then re-run `greenagonia deploy`.

---

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| Failure screen: "alert NOT sent — no routing key configured" | No key in this browser. Open your `?pdkey=` link again, or double-click the header logo and paste the key. |
| "alert NOT sent — HTTP 400" | Wrong key. It must be the orchestration routing key from `greenagonia urls`, not a REST token or a direct integration key. |
| Change events not appearing in *Recent Changes* | Change keys not set. Open the ops console and paste the keys from `greenagonia urls`, or use the full URL with `?pdchangekey=&pdldkey=`. |
| `terraform apply` → 401 | Token or region mismatch. EU accounts need region `eu`. Run `greenagonia token` and re-run `greenagonia setup` to set the region. |
| Apply fails creating workflows (404) | Account lacks the Incident Workflows entitlement. Set `enable_incident_workflows = false` in `config.auto.tfvars.json` and re-deploy. |
| Apply fails on `pagerduty_slack_connection` → 401/403 | The user-level token is missing or wrong. Run `greenagonia user-token`. This resource requires a user token, not the account REST API key. |
| Apply fails on Slack action ID | Action catalogues are account-specific. See `terraform-shared/README.md` for how to list action IDs and set the right one. |
| Slack channels: `name_taken` | Channels already exist. Import them — see the Slack setup section above. |
| Alert grouping fails on apply | No AIOps add-on. In `terraform-shared/services.tf` switch `alert_grouping_parameters` to `type = "time"`. |
| "user with this email already exists" | That email is already a user in the account. Use a different address or remove the existing user. |
| Incident lands on `unrouted-events` | The event's `custom_details.service` did not match any rule — check the routing key and payload. |

---

## Teardown

```bash
greenagonia destroy
```

Asks you to type `destroy` to confirm, then removes all PagerDuty resources
including the shared personas. State files in `state/` remain locally for
reference; delete them manually when done.

---

## Repo map

```
greenagonia/                this repo
├── README.md               CLI and architecture reference
├── ADMIN-GUIDE.md          this file
├── CLAUDE.md               context for Claude Code sessions
├── Makefile                build CLI, generate scenarios.json
├── install.sh              Linux x86_64 install script
├── backup-state.sh         back up state/ to dated tar.gz
├── quickstart.sh           macOS prereqs + build
├── site/                   storefront source (static site, serve anywhere)
├── state/                  terraform state (gitignored)
├── terraform-shared/       shared environment Terraform
└── cli/
    ├── main.go             scenarios + helpers
    └── shared.go           environment management commands
```
