# Greenagonia — terraform-shared

> New here? Start with **[ADMIN-GUIDE.md](../ADMIN-GUIDE.md)** — it walks through
> setup, adding admins, and running a demo. This file is the Terraform-level
> reference.

Terraform for the **multi-admin** Greenagonia environment: several PagerDuty
admins each get a full, isolated demo stack in one shared PagerDuty account,
all staffed by a common cast of generic personas.

---

## The model

### Shared resources — created once, visible to all

| Resource | Notes |
|---|---|
| 5 generic personas | Sarah SRE, Dan Developer, Matt Manager, Pablo Platform, Sam Security — `@greenagonia.io` emails, pages to them don't deliver. Members of every team and schedule rotation. |
| Greenagonia team | All personas and all admins belong to it. Admins are managers; personas are responders. |
| 5 platform services | `api-gateway`, `data-platform`, `identity-service`, `infrastructure`, `platform-engineering` — each with its own escalation policy and schedule, visible to every user in the account. |
| Greenagonia Platform Router | Shared event orchestration. Output: `platform_routing_key`. |
| `unrouted-events` | Catch-all service for events that don't match any orchestration rule. |
| 4 automation actions | Run Diagnostics, Rollback Deployment, Clear Down Settings, Isolate & Capture Forensics — bound to every technical service. |
| 4 incident workflows (opt-in) | Major Incident Escalation, Security Incident Response, Post to Status Page, Rollback Deployment. Manual triggers. Requires `enable_incident_workflows = true` and the Incident Workflows entitlement. |
| `greenagonia-incidents` Slack channel (opt-in) | Shared channel for the Greenagonia team. Requires `enable_slack = true`. |

### Per-admin resources — keyed by initials

| Resource | Naming | Notes |
|---|---|---|
| 1 user | their real name and email | gets the actual pages |
| 1 team | `JR-SRE-TEAM` | admin = manager, personas = responders |
| 8 technical services | `JR-payment-gateway` … | content-based alert grouping |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` | dependency edges to their tech services |
| 3 schedules | `JR Primary / Secondary / Tertiary On-Call` | primary: admin Mon–Fri 09:00–17:00; secondary/tertiary: persona rotations |
| 1 escalation policy | `JR SRE On-Call` | primary → secondary → tertiary, 2 loops |
| 1 event orchestration | `Greenagonia Event Router — JR` | personal routing key output: `admin_routing_keys["JR"]` |
| (opt-in) Slack channel | `jr-incidents` | permanent channel + PD Slack connection |

**Schedule time zone:** set by the `schedule_time_zone` variable (default
`Europe/London`). The admin is on call Mon–Fri 09:00–17:00 in that zone.
Nights and weekends are covered by persona rotations.

**Routing:** each admin's orchestration router matches on the bare service
name (`payment-gateway`) and routes to their prefixed service
(`JR-payment-gateway`). Events that don't match any rule fall to
`unrouted-events`. The routing key alone determines whose stack lights up —
two demos can run simultaneously from the same hosted storefront.

---

## File layout

One file per resource type. To change something, open the file named after it:

| File | Contents |
|---|---|
| `main.tf` | Provider configuration, the service catalogue (`locals.technical_services`), and persona definitions |
| `users.tf` | All `pagerduty_user` resources — 5 personas plus one per admin |
| `teams.tf` | Per-admin `pagerduty_team` + all team memberships |
| `schedules.tf` | The 3 per-admin schedules (working-hours primary layer and persona rotations) |
| `escalation_policies.tf` | Per-admin escalation policy + shared catch-all policy |
| `services.tf` | Per-admin technical services + shared `unrouted-events` |
| `business_services.tf` | Per-admin business services + dependency edges |
| `orchestration.tf` | Per-admin event orchestration + router rules |
| `greenagonia_platform.tf` | Greenagonia team, 5 platform services, shared orchestration |
| `slack_channels.tf` | Per-admin Slack channels + `pagerduty_slack_connection` |
| `automation.tf` | 4 shared automation actions + service bindings |
| `workflows.tf` | All incident workflows + manual triggers (per-admin Slack workflow included) |
| `variables.tf` | All input variable declarations |
| `outputs.tf` | Per-admin routing keys, change event keys, storefront links |

**Common edits:**

- Add a service: add it to `locals.technical_services` in `main.tf`. A
  PagerDuty service and an orchestration rule are generated automatically via
  `for_each` — no other files to edit.
- Change working hours: edit the schedule layer in `schedules.tf`.
- Add a persona: add them to `locals.personas` in `main.tf`.
- Change workflow wording: edit `workflows.tf`.

---

## Variables

The CLI writes two gitignored auto-loaded var files in this directory:

| File | Contents |
|---|---|
| `secrets.auto.tfvars.json` | `pagerduty_token`, `pagerduty_user_token`, `slack_bot_token` — chmod 600 |
| `config.auto.tfvars.json` | `admins` map, `schedule_time_zone`, `site_base_url`, feature flags |

Use `terraform.tfvars.example` (if present) to see the full variable shape
when running Terraform directly.

Key variables:

| Variable | Type | Description |
|---|---|---|
| `pagerduty_token` | string | PagerDuty REST API token (read/write, account admin) |
| `pagerduty_user_token` | string | PagerDuty user-level token (required for Slack connections) |
| `pagerduty_region` | string | `"global"` or `"eu"` |
| `admins` | map | Keyed by initials: `{name, email, slack_email}` |
| `schedule_time_zone` | string | IANA time zone for all schedules |
| `site_base_url` | string | Storefront base URL; used to build per-admin links in outputs |
| `enable_incident_workflows` | bool | Create the 4 incident workflows (needs entitlement) |
| `enable_slack` | bool | Create Slack channels and PD connections |
| `slack_workspace_id` | string | Slack workspace ID (starts with `T`) |
| `slack_bot_token` | string | Slack bot token (`xoxb-…`) |

---

## Outputs

| Output | Description |
|---|---|
| `admin_routing_keys` | Map of initials → personal routing key |
| `admin_storefront_urls` | Map of initials → full storefront URL with `?pdkey=` |
| `platform_routing_key` | Shared Greenagonia Platform Router key |

Access an output directly:

```bash
terraform -chdir=terraform-shared output -json admin_routing_keys
```

The `greenagonia urls` CLI command wraps these outputs into a formatted table.

---

## State location

State lives in `../state/` relative to this directory (i.e. `single-user/state/`,
gitignored). One person applies at a time.

Back up state before a risky apply:

```bash
cd .. && ./backup-state.sh
```

This creates a dated `.tar.gz` in `state/backups/` and keeps the 10 most
recent backups.

**Remote state:** if multiple operators need to run applies concurrently, migrate
to a remote backend with locking. HCP Terraform free tier is the simplest path:
create a workspace, add a `cloud {}` block to `main.tf`, run
`terraform -chdir=terraform-shared init`, and approve the state migration.

---

## Isolation between admins

Each admin has PagerDuty's **Restricted Access** base role, scoped to their
team. Per admin:

- 🟢 Visible: their team, 8 technical services, 2 business services,
  3 schedules, escalation policy, event orchestration, their incidents, and
  the shared personas
- 🔴 Invisible: every other admin's resources; the shared `unrouted-events`
  catch-all

Team association drives this. Escalation policies, schedules, orchestrations,
and business services carry explicit team references; technical services
inherit the team through their escalation policy. The Terraform operator
(account admin REST token) sees and manages everything.

---

## Slack channels and PagerDuty connections

With `enable_slack = true`, Terraform creates:

- **Per-admin Slack channel** (`jr-incidents`, etc.) — a permanent channel
  for each admin, with them as a member.
- **`greenagonia-incidents`** — a shared channel for the whole Greenagonia team.
- **`pagerduty_slack_connection`** — one connection per team that routes
  triggered/acknowledged/resolved/escalated events into the matching channel.

### Required values

| Variable | Where to get it |
|---|---|
| `slack_bot_token` (`xoxb-…`) | Slack app → OAuth & Permissions → Bot User OAuth Token |
| `pagerduty_user_token` | PagerDuty → My Profile → User Settings → Create API User Token |
| `slack_workspace_id` (`T…`) | PagerDuty → Integrations → Slack V2 → workspace ID |

Set them via the CLI:

```bash
greenagonia slack-token     # prompts for xoxb-… token
greenagonia user-token      # prompts for PD user-level token
```

Required Slack bot scopes: `channels:manage`, `channels:join`, `users:read`,
`users:read.email`.

The `pagerduty_slack_connection` resource requires a **user-level token**
(`pagerduty_user_token`), not the account REST API key.

### If channels already exist in the workspace

Terraform fails with `name_taken`. Import the existing channels first, then
re-deploy:

```bash
# find the channel ID
curl -s -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  "https://slack.com/api/conversations.list" \
  | jq '.channels[] | select(.name=="jr-incidents") | .id'

terraform -chdir=terraform-shared import 'slack_conversation.incidents["JR"]' <channel-id>
```

Note: Incident Workflows entitlement is **not** required for Slack channels
or connections. The per-admin "Open Incident Channel" workflow (which creates
per-incident channels) does require `enable_incident_workflows = true`.

---

## Initials collisions

`greenagonia admin add JR "J Roberts" …` when `JR` is already in use
automatically derives the next free initials from the name: `JR → JRO →
JROB → JOR …` (2–4 uppercase letters). The CLI tells you what it chose.
Re-adding the same email under the same initials is an update, not a
collision.

---

## Notes and gotchas

- Admin and persona emails must not already exist as users in the account.
- Alert grouping uses the AIOps content-based grouper. Without the AIOps
  add-on, change `alert_grouping_parameters` in `services.tf` to
  `type = "time"`.
- Incident Workflows return 404 on accounts without the entitlement. Use
  `enable_incident_workflows = false` on those accounts.
- The "Post to Status Page" workflow uses a status update (incident
  subscribers see it). If the account has native PagerDuty Status Pages,
  swap in the native action from the action catalogue.
- Resource count scales at roughly 60 per admin. Plans stay fast up to about
  10 admins.
- There are no per-service change event integrations in the shared
  environment. Change events are posted by the storefront using keys baked
  into the site at deploy time.
