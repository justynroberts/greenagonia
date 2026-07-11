# Greenagonia

PagerDuty's fictional demo company — a shared, multi-admin environment where
each person on your team gets their own full PagerDuty stack (services,
schedules, on-call, event orchestration) and a personal link to a hosted
e-commerce storefront whose checkout fails on demand.

---

## Quick start

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia
./get-binary.sh                                    # download binary for your platform
./greenagonia setup                                # wizard: token, region, timezone, first admin
./greenagonia deploy                               # provision everything in PagerDuty (~2 min)
./greenagonia urls                                 # get per-admin storefront links
```

Then open your storefront link, click **Pay**, and watch the incident land in PagerDuty.

> Need Terraform first? `brew install terraform` (macOS) or use `install.sh` on Linux.

---

## Running a demo

1. Open `http://<site>/?pdkey=<your-routing-key>` — the routing key is stored in
   `localStorage` and scrubbed from the URL bar on first visit.
2. Click **Pay** on the hero checkout card. The order pipeline runs and fails
   at "Charging payment".
3. Before the alert fires, the storefront posts two **change events** backdated
   to simulate a real deployment:

   | Change event | Backdated | Summary |
   |---|---|---|
   | GitHub deploy | −3 min | `payment-service v2.41.0` deployed to production |
   | LaunchDarkly flag | −2 min | `checkout-v2-enabled` flag enabled for all users |

4. In PagerDuty: an incident opens on `JR-payment-gateway`, pages whoever is
   on call. `JR-customer-checkout` shows business impact. The *Recent Changes*
   tab shows both change events.
5. Repeated checkouts deduplicate into the same open incident.
6. Resolve in PagerDuty normally, or hit **Resolve all** in the storefront
   ops console (Ctrl/Cmd+Shift+K).

---

## CLI reference

### Setup and tokens

```bash
./greenagonia setup                        # first-run wizard
./greenagonia token                        # replace the PagerDuty REST API token
./greenagonia user-token                   # set PD user-level token (required for Slack)
./greenagonia slack-token                  # set Slack bot token
```

### Admin management

```bash
./greenagonia admin add JR "Justyn Roberts" j@example.com
./greenagonia admin add JR "Justyn Roberts" j@example.com j@slack.com   # with Slack email
./greenagonia admin remove JR              # marks JR for removal on next deploy
./greenagonia admin list                   # show all configured admins
```

Initials are 2–4 uppercase letters. If the requested initials are taken, the
CLI derives free ones automatically (`JR → JRO → JROB`). Re-adding the same
email under the same initials is an update, not an error.

### Deploy and destroy

```bash
./greenagonia deploy                       # terraform init → plan → confirm → apply
./greenagonia destroy                      # tear down everything (prompts for confirmation)
```

Plans are additive by admin — adding or removing one admin touches only that
admin's ~60 resources; all others are untouched.

### URLs and storefront

```bash
./greenagonia urls                         # all per-admin links and routing keys
./greenagonia urls JR                      # one admin
./greenagonia site-url                     # show current storefront base URL
./greenagonia site-url http://1.2.3.4      # set it; re-run deploy to apply
```

### Slack

```bash
./greenagonia slack-channels               # create {ini}-incidents channels for all admins
```

### Scenarios

```bash
./greenagonia scenarios list               # browse available scenarios
./greenagonia scenarios dump-json          # export scenario data as JSON
```

---

## What each admin gets

For initials `JR`:

| Resource | Name |
|---|---|
| User | their real name and email — gets the actual pages |
| Team | `JR-SRE-TEAM` |
| 8 technical services | `JR-payment-gateway`, `JR-checkout-api`, `JR-user-auth`, `JR-product-catalog`, `JR-recommendation-engine`, `JR-search-service`, `JR-order-service`, `JR-notification-service` |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` |
| 3 schedules | `JR Primary On-Call` (admin, Mon–Fri 09–17), `JR Secondary`, `JR Tertiary` (personas) |
| Escalation policy | `JR SRE On-Call` — primary → secondary → tertiary, 2 loops |
| Event orchestration | `Greenagonia Event Router — JR` + personal routing key |
| (optional) Slack | `jr-incidents` channel + PD Slack connection |

**Shared resources** (created once, visible to all):

| Resource | Notes |
|---|---|
| 5 personas | Sarah SRE, Dan Developer, Matt Manager, Pablo Platform, Sam Security — `@greenagonia.io`, pages don't deliver |
| Greenagonia team | all admins are managers; all personas are responders |
| 5 platform services | `api-gateway`, `data-platform`, `identity-service`, `infrastructure`, `platform-engineering` |
| Greenagonia Platform Router | shared event orchestration routing key |
| `unrouted-events` | catch-all for events that match no rule |
| 4 automation actions | Run Diagnostics, Rollback Deployment, Clear Down Settings, Isolate & Capture Forensics |
| (optional) 4 incident workflows | Major Incident Escalation, Security Incident Response, Post to Status Page, Rollback |

**Isolation:** each admin has the PagerDuty Restricted Access role scoped to
their team — they see their own stack and the shared personas, nothing else.
The Terraform operator token sees and manages everything.

**Routing:** each admin's orchestration matches on `custom_details.service`
(bare name, e.g. `payment-gateway`) and routes to their prefixed service
(`JR-payment-gateway`). Two admins can demo simultaneously from the same
storefront — they just use different `?pdkey=` links.

---

## Storefront

`site/` is a zero-build static site — no npm, no bundler.

```bash
# local demo
cd site && python3 -m http.server 8080
# open http://localhost:8080/?pdkey=<routing-key>
```

For a shared deployment, host `site/` on any static host (GitHub Pages, S3,
EC2) and register the URL:

```bash
./greenagonia site-url https://demo.example.com
./greenagonia deploy
./greenagonia urls        # updated per-admin links
```

The ops console (Ctrl/Cmd+Shift+K, or footer link) shows all stored keys and
a live log of events dispatched in this browser session.

---

## State and backups

Terraform state lives in `state/` (gitignored). One operator applies at a
time. Back up before risky changes:

```bash
./backup-state.sh         # creates state/backups/state-YYYYMMDD-HHMMSS.tar.gz
```

Keeps the 10 most recent backups. For concurrent multi-operator applies,
migrate to a remote backend (HCP Terraform free tier works).

---

## Install

### Download binary (recommended)

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia
./get-binary.sh
```

Detects your platform and downloads the matching binary from the [latest
release](https://github.com/justynroberts/greenagonia/releases/latest).
Platforms: macOS arm64/amd64, Linux amd64/arm64.

Also install Terraform ≥ 1.5: `brew install terraform` (macOS) or see
[releases.hashicorp.com](https://releases.hashicorp.com/terraform/).

### Linux — install globally (server)

```bash
curl -fsSL https://raw.githubusercontent.com/justynroberts/greenagonia/master/install.sh | bash
```

Installs `greenagonia` to `/usr/local/bin/` and Terraform 1.9.8.

### Build from source

```bash
cd greenagonia
make build        # requires Go ≥ 1.22 and Terraform ≥ 1.5
                  # macOS: ./quickstart.sh installs both via Homebrew
```

---

## Repo layout

```
greenagonia/
├── get-binary.sh         download pre-built binary for this platform
├── install.sh            install binary + terraform globally on Linux
├── quickstart.sh         macOS: install prereqs + build from source
├── backup-state.sh       back up state/ to a dated tar.gz
├── Makefile              build / build-all / clean / generate-scenarios
├── site/                 storefront source (static site, serve anywhere)
├── state/                terraform state (gitignored)
├── terraform-shared/     all Terraform for the shared environment
└── cli/                  Go source for the greenagonia CLI
```

For a full walkthrough see **[ADMIN-GUIDE.md](ADMIN-GUIDE.md)**.
