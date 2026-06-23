# Greenagonia Shared Environment — Admin Guide

How to stand up, operate, and demo the shared Greenagonia environment.
Written for colleagues who haven't seen this repo before.

## What this is

Greenagonia is a fictional outdoor-gear company used for PagerDuty demos. The
shared environment gives **each admin their own full demo stack inside one
PagerDuty account**, driven by a hosted e-commerce storefront whose checkout
fails on demand.

```
shared-usage/shared-site        the storefront (static site, runs anywhere)
        │  1. POST change events (GitHub deploy, LaunchDarkly flag — backdated)
        │  2. POST alert event → your personal routing key
        ▼
Greenagonia Event Router — JR   your event orchestration
        ▼
JR-payment-gateway              your service → incident → pages YOU
        ▼
JR-customer-checkout            your business service lights up
```

Everyone shares a cast of five fictional responders — Sarah SRE, Dan
Developer, Matt Manager, Pablo Platform, Sam Security — who staff every
team's schedules so the account looks like a real company. There is also a
shared **Greenagonia** team and five platform services visible to everyone.

## What you get as an admin

For your initials (say `JR`):

| Resource | Name |
|---|---|
| Team | `JR-SRE-TEAM` (you = manager, personas = responders) |
| 8 technical services | `JR-payment-gateway`, `JR-checkout-api`, … |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` |
| 3 schedules | `JR Primary / Secondary / Tertiary On-Call` |
| Escalation policy | `JR SRE On-Call` (primary → secondary → tertiary) |
| Event orchestration | `Greenagonia Event Router — JR` + your routing key |
| (optional) Slack channel | `jr-incidents` — permanent channel, PD Slack connection |
| (optional) Slack workflow | `JR — Open Incident Channel` — auto-creates a per-incident channel |

**Shared with all admins** (visible to everyone in the account):

| Resource | Name |
|---|---|
| Team | `Greenagonia` (you are a manager) |
| 5 platform services | `api-gateway`, `data-platform`, `identity-service`, `infrastructure`, `platform-engineering` |
| Platform orchestration | `Greenagonia Platform Router` + platform routing key |
| Slack channel (optional) | `greenagonia-incidents` |

Your **primary schedule has you on call Mon–Fri 09:00–17:00** (Europe/London
by default); the personas cover nights and weekends, and staff the secondary
and tertiary rotations. You get the real pages — personas' emails are
fictional and deliver nowhere.

**Isolation:** your PagerDuty login has the Restricted Access role, scoped
to your team. You see your own team, services, schedules, orchestration and
incidents — plus the shared personas — and nothing belonging to other
admins. Inside your team you're a manager, so you can tweak anything that's
yours. If something you expect to see is missing, it probably belongs to
someone else's stack (or it's the shared catch-all, which only the
environment operator sees).

## Prerequisites

- 🟢 macOS/Linux with `terraform` (≥ 1.5) and `python3`
- 🟢 A PagerDuty account where you have admin access, and a **REST API key**
  with read/write scope (*Integrations → API Access Keys*)
- 🟢 Your email must **not** already exist as a user in that account
- 🟡 Optional — AIOps add-on (alert grouping), Incident Workflows entitlement,
  Slack V2 integration + bot token + user-level API token (for Slack channels/connections)

## First-time setup (one person does this)

```bash
cd ~/work/greenagonia/shared-terraform

./setup.sh setup     # wizard: token (hidden), region, time zone, site URL,
                     # workflows y/n, Slack y/n, first admin
./setup.sh deploy    # terraform plan → review → confirm → apply
```

`deploy` ends by printing each admin's personal storefront link. The token is
stored in `secrets.auto.tfvars.json` (chmod 600, gitignored) — it never
leaves this directory.

## Adding yourself (or anyone) as an admin

```bash
./setup.sh admin add JR "Justyn Roberts" justyn@example.com
./setup.sh deploy
```

- Initials are 2–4 capital letters and name every resource.
- **If your initials are taken**, the tool derives free ones from your name
  automatically: `JR → JRO → JROB → JOR`. It tells you what it picked.
- Re-adding the same email under the same initials updates the entry.
- `./setup.sh admin remove JR` + deploy destroys that admin's stack (only
  theirs — plans never touch other admins' resources).
- `./setup.sh admin list` shows who's configured.

## Running a demo

1. Get your link: `./setup.sh urls` — it looks like
   `https://<site>/?pdkey=R0…`. Opening it stores your routing key in the
   browser (and scrubs it from the address bar). One-time per browser.
2. The storefront front page **leads with a checkout card**. Click **Pay**.
3. The order pipeline runs and **fails at "Charging payment"** — the default
   armed scenario is a payment-gateway card-processor timeout. The customer
   sees a realistic error; two **change events** are posted first (backdated
   ~2–3 min): a GitHub deploy of `payment-service v2.41.0` and a LaunchDarkly
   feature flag change. Then the alert fires.
4. In PagerDuty: an incident opens on `JR-payment-gateway`, pages whoever
   your primary schedule says is on call (you, during working hours),
   `JR-customer-checkout` shows impact. The *Recent Changes* tab shows the
   two change events. If Slack is enabled, the `jr-incidents` channel gets
   a notification via the PD Slack connection.
5. Repeated failures **dedupe into the same incident** (stable dedup key),
   so you can retry checkout all day without flooding the queue.

To resolve: resolve the incident in PagerDuty as normal, or use **Resolve
all** in the storefront's ops console.

### The storefront's hidden ops console

The site looks like a normal shop. To reach the controls:

- press **Ctrl/⌘ + Shift + K**, or click the small **"operations console"**
  link in the footer, or triple-click the footer logo
- **double-click the header logo** to change/remove the stored routing key
- the console shows three key fields: routing key, change event key (GitHub),
  change event key (LaunchDarkly) — all pre-loaded from your `?pdkey=` URL
- switch the armed scenario to **All systems healthy** (checkout then succeeds)
- an event log shows everything dispatched in this browser session

The site fires exactly **one scenario** — `bad-payment-deploy` / "Card
processor timeouts" — an exact replica of the payload from the single-user
CLI, including dedup key and `custom_details`.

## Hosting the storefront

It's a zero-build static site. For a laptop demo:

```bash
cd ~/work/greenagonia/shared-usage/shared-site
python3 -m http.server 8080
```

For a team, host it once (GitHub Pages, an EC2 instance, or any static host) and
tell setup.sh the URL so `./setup.sh urls` emits correct links:

```bash
cd shared-terraform
./setup.sh site-url http://3.85.144.140
./setup.sh deploy          # re-applies; outputs get the new base URL
./setup.sh urls            # shows updated links
```

**HTTP vs HTTPS:** the site works over plain HTTP. GitHub Pages and most CDN
hosts serve HTTPS, which also works. Avoid `file://` if you want `?pdkey=`
URL pre-loading (CORS restriction).

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| Failure screen says "alert NOT sent — no routing key configured" | No key in this browser. Open your `?pdkey=` link again, or double-click the header logo and paste the key. |
| "alert NOT sent — HTTP 400" | Wrong key. It must be the **orchestration routing key** from `./setup.sh urls`, not a REST token or another integration key. |
| Change events not appearing in Recent Changes | Change keys not set. Open ops console → paste the keys shown by `./setup.sh urls`, or use the full URL with `?pdchangekey=&pdldkey=`. |
| Checkout succeeds when you wanted a failure | Scenario was switched to healthy in the ops console — switch it back. Default is failing. |
| `terraform apply` → 401 | Token/region mismatch — EU accounts need region `eu` (`./setup.sh token`). |
| Apply fails creating workflows (404) | Account lacks the Incident Workflows entitlement. Re-run `./setup.sh setup` and answer "n" to workflows (and Slack). |
| Apply fails creating `pagerduty_slack_connection` → 401/403 | The `pagerduty_user_token` is missing or wrong. Run `./setup.sh user-token`. This resource requires a user-level token, not the account REST API key. |
| Apply fails on the Slack step's action ID | Action catalogues are account-specific. List yours and set `slack_create_channel_action_id` — the curl is in `shared-terraform/README.md`. |
| Slack channels error `name_taken` on apply | Channels already exist in the workspace. Import them: see the import instructions in `shared-terraform/README.md`. |
| Apply fails on alert grouping | No AIOps add-on. In `services.tf`, switch `alert_grouping_parameters` to `type = "time"`. |
| "user with this email already exists" | The email is already a user in the account — use a different one or remove the existing user. |
| Incident lands on `unrouted-events` | The event's `custom_details.service` didn't match a known service name — check the payload. |

## Teardown

```bash
./setup.sh destroy    # asks you to type "destroy"; removes everything,
                      # including the personas
```

## Repo map

```
greenagonia/
├── ADMIN-GUIDE.md            this file
├── CLAUDE.md                 context for Claude Code sessions
├── single-user/              the original one-person env (Terraform + Go CLI,
│                             7 scenarios, own README) — separate from shared
├── shared-terraform/         the shared environment (this guide) + setup.sh
└── shared-usage/shared-site/ the storefront (own README: console, internals)
```
