# Greenagonia — shared (multi-admin) environment

> New here? Start with the **[Admin Guide](../ADMIN-GUIDE.md)** — it walks
> through setup, adding yourself, and running a demo. This file is the
> Terraform-level reference.

Terraform for a **multi-admin** Greenagonia: several PagerDuty admins (e.g. an
SE team) each get a full, isolated copy of the single-user demo stack in one
shared PagerDuty account — staffed by a common cast of generic personas.

## The model

**Shared, created once:**

- **5 generic personas** — Sarah SRE, Dan Developer, Matt Manager, Pablo
  Platform, Sam Security (`@greenagonia.io` emails — fictional, pages to them
  don't deliver). Members of every team, present in every schedule rotation.
- **Greenagonia team** — public team that all personas and admins belong to.
  Admins are managers; personas are responders.
- **5 shared platform services** — `api-gateway`, `data-platform`,
  `identity-service`, `infrastructure`, `platform-engineering` — each with its
  own escalation policy, schedule, and content-based alert grouping, all
  visible to every user in the account.
- **1 shared event orchestration** — `Greenagonia Platform Router` — routes
  events by service name to the five platform services. Routing key output:
  `platform_routing_key`.
- **4 automation actions** — Run Diagnostics, Rollback Deployment, Clear Down
  Settings, Isolate & Capture Forensics — bound to every technical service.
- **4 incident workflows** (opt-in via `enable_incident_workflows`) — Major
  Incident Escalation, Security Incident Response, Post to Status Page,
  Rollback Deployment to Previous. Manual triggers, available on every incident.
- **1 catch-all service** (`unrouted-events`) shared by all routers.

**Per admin** (`admins` map, keyed by initials, e.g. `JR`):

| Resource | Naming | Notes |
|---|---|---|
| 1 user | their real name/email | gets the actual pages |
| 1 team | `JR-SRE-TEAM` | admin = manager, personas = responders |
| 8 technical services | `JR-payment-gateway` … | same catalogue as single-user, content-based grouping |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` | dependency edges to their tech services |
| 3 schedules | `JR Primary/Secondary/Tertiary On-Call` | see below |
| 1 escalation policy | `JR SRE On-Call` | primary → secondary → tertiary, 2 loops |
| 1 event orchestration | `Greenagonia Event Router — JR` | personal routing key |

**Schedules:** the primary has the admin on call **Mon–Fri 09:00–17:00**
(`schedule_time_zone`, default Europe/London) with the persona rotation
covering nights/weekends; secondary and tertiary are weekly persona
rotations, staggered so different personas are up on each.

**Routing — one scenario, from the storefront:** this environment is driven
exclusively by the shared storefront (`../shared-usage/shared-site`), which
fires exactly one scenario: `bad-payment-deploy` / "Card processor timeouts"
with `custom_details.service = "payment-gateway"`. Each admin's router
matches the plain service name and routes to their prefixed service
(`JR-payment-gateway`), which lights up their `JR-customer-checkout`
business service. The routing key alone decides whose stack lights up.
Unmatched events land on the shared `unrouted-events`. (No CLI here — no
per-service change-event integrations either; the single-user stack keeps
those.)

## Usage — via the setup CLI (recommended)

```bash
# First-time setup
./setup.sh setup                                 # wizard: token, settings, first admin
./setup.sh deploy                                # plan → confirm → apply; prints per-admin links

# Day-to-day
./setup.sh urls                                  # per-admin storefront links with keys
./setup.sh urls JR                               # just one admin

# Admins
./setup.sh admin add AB "Alice Bell" alice@example.com
./setup.sh admin remove AB
./setup.sh deploy                                # plan touches only AB's resources

# Secrets and settings (each writes to gitignored files, then deploy to apply)
./setup.sh token                                 # replace PagerDuty REST API token
./setup.sh user-token                            # set PagerDuty user-level token (needed for Slack connections)
./setup.sh slack-token                           # set Slack bot token
./setup.sh site-url http://3.85.144.140          # set storefront base URL
```

The CLI stores everything as Terraform auto-loaded var files (both gitignored):
`secrets.auto.tfvars.json` (tokens, chmod 600) and `config.auto.tfvars.json`
(admins, time zone, site URL). Bypass and run terraform directly if you prefer —
`terraform.tfvars.example` shows the full variable shape including all optional fields.

Add an admin = one CLI command + deploy; the plan touches only their
resources. Removing one destroys only their stack on the next deploy.

## Isolation between admins

Admins are created with PagerDuty's **Restricted Access** base role: they can
only see objects associated with teams they're a member of. Each admin
belongs to exactly one team — their own — where their team role is
**manager**, giving them full control inside it. The result, per admin:

- 🟢 visible: their team, its 10 services, 3 schedules, escalation policy,
  event orchestration, their incidents — and the shared personas (the user
  directory is global)
- 🔴 invisible: every other admin's team, services, incidents, and
  orchestration; the shared `unrouted-events` catch-all

Team association is what drives this: escalation policies, schedules,
orchestrations and business services carry `team(s)` explicitly; technical
services inherit the team through their escalation policy. The Terraform
operator's REST token (account admin) sees and manages everything.

## File layout — one file per resource type

To change something, open the file named after the thing:

| File | Contains |
|---|---|
| `users.tf` | all `pagerduty_user` (5 personas + admins) |
| `teams.tf` | per-admin teams + memberships |
| `schedules.tf` | the 3 per-admin schedules (working-hours layer lives here) |
| `escalation_policies.tf` | per-admin policy + shared catch-all policy |
| `services.tf` | technical services + the shared `unrouted-events` |
| `business_services.tf` | business services + dependency edges |
| `orchestration.tf` | per-admin orchestration + router |
| `greenagonia_platform.tf` | Greenagonia team, 5 platform services, shared orchestration |
| `slack_channels.tf` | per-admin Slack channels + `pagerduty_slack_connection` |
| `automation.tf` | the 4 shared automation actions + service bindings |
| `workflows.tf` | all incident workflows + triggers, incl. per-admin Slack |
| `main.tf` | provider + the service/persona catalogue (locals) |
| `variables.tf` / `outputs.tf` | inputs / outputs |

Common edits: add a service → the catalogue in `main.tf` (everything else
cascades via `for_each`); change working hours → `schedules.tf`; add a
persona → `main.tf` locals; change workflow wording → `workflows.tf`.

## Where state lives

Local, in this directory (`terraform.tfstate`, gitignored). **One person
applies at a time.** If multiple admins start running applies, migrate to a
remote backend with locking — HCP Terraform's free tier is the easy option:
create a workspace, add a `cloud {}` block to `main.tf`, run
`terraform init` and approve the state migration.

## Slack incident channels and PagerDuty connections (optional)

With `enable_slack = true` (the setup wizard asks), Terraform creates:

- **Per-admin Slack channel** — `jr-incidents`, `jp-incidents`, etc. — a permanent
  channel in your workspace for each admin, with them as a member.
- **`greenagonia-incidents`** — a shared channel for the whole Greenagonia team.
- **`pagerduty_slack_connection`** — one connection per team (per-admin team +
  Greenagonia team) that routes triggered/acknowledged/resolved/escalated
  events into the matching channel.

Required variables (set via `./setup.sh slack-token` / `./setup.sh user-token`):

| Variable | Where to get it |
|---|---|
| `slack_bot_token` | Slack app → OAuth & Permissions → Bot User OAuth Token (`xoxb-…`) |
| `pagerduty_user_token` | PagerDuty → My Profile → User Settings → Create API User Token |
| `slack_workspace_id` | PagerDuty → Integrations → Slack V2 → workspace ID starts with `T` |

The `pagerduty_slack_connection` resource specifically requires a **user-level** token
(`pagerduty_user_token`), not the account REST API key.

Required Slack bot scopes: `channels:manage`, `channels:join`, `users:read`, `users:read.email`.

If `jr-incidents` / similar channels already exist in the workspace, import them first:

```bash
# find the channel ID
curl -s -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  "https://slack.com/api/conversations.list" | jq '.channels[] | select(.name=="jr-incidents") | .id'

terraform import 'slack_conversation.incidents["JR"]' <channel-id>
```

Incident Workflows entitlement is **not** required for the Slack connections or channels —
they're independent resources. The per-admin "Open Incident Channel" Incident Workflow
(which auto-creates channels per-incident) does require the entitlement
(`enable_incident_workflows = true`).

## Initials collisions

`./setup.sh admin add JR "J Roberts" …` when `JR` is taken derives the next
free initials from the name automatically: `JR → JRO → JROB → JOR …` (2-4
uppercase letters). Re-adding the same email under the same initials is an
update, not a collision.

## Change events (storefront → Recent Changes tab)

When a checkout failure fires, the storefront first posts two **PagerDuty change events**
that appear on the incident's *Recent Changes* tab, backdated to simulate a real deployment:

| Event | Backdated | Summary |
|---|---|---|
| GitHub deploy | −3 min | `v2.41.0` of `payment-service` deployed to `production` |
| LaunchDarkly flag | −2 min | `checkout-v2-enabled` feature flag turned ON for all users |

These use separate integration keys (distinct from the alert routing key). The storefront
ops console shows two extra fields: "Change event key — GitHub deploys" and
"Change event key — LaunchDarkly flags". Pre-load them via URL:

```
http://<site>/?pdkey=<routing>&pdchangekey=<github-key>&pdldkey=<ld-key>
```

`./setup.sh urls` shows all three keys per admin. If a change key is unset the event
is silently skipped — the incident still fires.

## Notes & gotchas

- Admin/persona emails must not already exist as users in the account.
- Alert grouping uses the AIOps content-based grouper; without the AIOps
  add-on, switch `alert_grouping_parameters` in `services.tf` to `type = "time"`.
- Incident Workflows are a paid feature (404 without entitlement) — hence the
  flag. The "Post to Status Page" workflow uses a status update (incident
  subscribers see it); if the account has native PagerDuty Status Pages,
  swap in the native post-to-status-page action from the action catalogue.
- Resource count scales at ~60 per admin (services, integrations, schedules,
  associations, router rules) — plans stay fast to ~10 admins.
