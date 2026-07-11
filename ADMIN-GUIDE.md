# Greenagonia — Admin Guide

Everything you need to set up, run, and demo the Greenagonia shared environment.
Start here if you've never touched this repo before.

---

## What you're building

Greenagonia is a fake outdoor-gear company that exists purely to make PagerDuty demos look real. When you run `./greenagonia deploy`, it builds a complete company inside your PagerDuty account:

- A realistic team with an on-call schedule (you're on call Mon–Fri 9–5)
- 8 microservices and 2 business services, all named and connected properly
- Event orchestration so alerts from the storefront land on the right service
- Five fictional teammates (Sarah SRE, Dan Developer, Matt Manager, Pablo Platform, Sam Security) who staff nights and weekends and make the account look like a real company

Multiple admins can share one PagerDuty account. Each person gets their own private stack — their own services, their own routing key, their own on-call schedule. You can run 10 demos at the same time from the same storefront website.

**How it flows:**

```
You click Pay on the storefront
        │
        ├── change event 1: payment-service v2.41.0 deployed  (backdated -3 min)
        ├── change event 2: checkout-v2 feature flag enabled   (backdated -2 min)
        │
        ▼
Your event orchestration router
        │
        ▼
JR-payment-gateway (your service) → incident created → YOU get paged
        │
        ▼
JR-customer-checkout (your business service) → shows business impact
```

---

## Before you start

You need:

- 🟢 **macOS or Linux**
- 🟢 **A PagerDuty account** where you have admin access
- 🟢 **A PagerDuty REST API key** with read/write scope — create one at
  Integrations → API Access Keys
- 🟢 **An email address** that isn't already a user in that PagerDuty account
  (Greenagonia creates a user with it)
- 🟢 **Terraform ≥ 1.5** — `brew install terraform` on macOS; `install.sh`
  handles it on Linux

Optional (you can add these later):
- 🟡 Slack — requires Slack V2 integration in PagerDuty, a bot token, and a user-level API token
- 🟡 Incident Workflows — requires the Incident Workflows entitlement on your account
- 🟡 AIOps content-based alert grouping — requires the AIOps add-on

---

## First-time setup

### Step 1 — Get the CLI

**macOS or Linux:**

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia
./get-binary.sh
```

`get-binary.sh` figures out what OS and CPU chip you have, then downloads the
right pre-built binary from GitHub releases and saves it as `./greenagonia`.

Install Terraform if you don't have it:

```bash
brew install terraform    # macOS
```

On Linux, skip the above and use this instead — it installs both the binary
and Terraform in one go:

```bash
curl -fsSL https://raw.githubusercontent.com/justynroberts/greenagonia/master/install.sh | bash
```

Check everything is working:

```bash
./greenagonia --help
```

> All examples in this guide use `./greenagonia`. If you installed globally
> via `install.sh`, you can drop the `./` and just type `greenagonia`.

---

### Step 2 — Run setup

```bash
./greenagonia setup
```

This is an interactive wizard. It asks you for:

1. **PagerDuty REST API token** — paste it in (hidden, won't be echoed)
2. **PagerDuty region** — `global` for most accounts; `eu` for accounts on
   `*.eu.pagerduty.com`
3. **Time zone** — default is `Europe/London`; this sets when you're on-call
4. **Storefront URL** — where you plan to host the demo site (you can set this
   later if you don't know yet)
5. **Incident Workflows** — say `n` if you're not sure whether your account
   has the entitlement
6. **Slack** — say `n` for now; you can enable it after the first deploy
7. **First admin** — your initials (e.g. `JR`), full name, and email

When setup finishes, two files are written into `terraform-shared/`:

- `secrets.auto.tfvars.json` — your tokens (locked down to owner-only read)
- `config.auto.tfvars.json` — admins list, time zone, feature flags

Both are gitignored — they never get committed.

---

### Step 3 — Add yourself as an admin

If you didn't add yourself during setup:

```bash
./greenagonia admin add JR "Jo Roberts" jo@example.com
```

**About initials:** they become the prefix for all your PagerDuty resources.
`JR` becomes `JR-payment-gateway`, `JR-SRE-TEAM`, etc. Use 2–4 uppercase
letters. If the initials are already taken by someone else, the CLI
automatically picks the next free ones (`JR → JRO → JROB`) and tells you
what it chose.

Got a different email in Slack? Add it as a fourth argument:

```bash
./greenagonia admin add JR "Jo Roberts" jo@work.com jo@company.slack.com
```

---

### Step 4 — Deploy

```bash
./greenagonia deploy
```

This runs `terraform init`, shows you a plan of everything it's about to
create, asks you to confirm, then applies it. First deploy with one admin
takes about 2–4 minutes and creates roughly 60–70 resources.

At the end you'll see something like:

```
JR  https://demo.example.com/?pdkey=R0abcd1234efgh5678...
```

That's your personal storefront link. Save it.

---

### Step 5 — Get your links

```bash
./greenagonia urls        # all admins
./greenagonia urls JR     # just you
```

This prints your routing key and storefront URL. If you've set up change event
integrations, it also shows those keys.

---

### Step 6 — Host the storefront

The storefront is pure HTML/CSS/JS — no build step, no npm, nothing to install.

**For a quick local demo:**

```bash
cd site && python3 -m http.server 8080
# open http://localhost:8080/?pdkey=<your-routing-key>
```

**For a shared team deployment (recommended — host it once, everyone uses it):**

Copy the `site/` folder to any static host — GitHub Pages, an S3 bucket, an
EC2 instance, anything that serves files over HTTP. Then tell Greenagonia
where it is:

```bash
./greenagonia site-url https://demo.example.com
./greenagonia deploy          # re-runs Terraform so outputs include the new URL
./greenagonia urls            # check your updated link
```

---

## Running a demo

**Before you start:** make sure you have your personal storefront link from
`./greenagonia urls`. It looks like `https://demo.example.com/?pdkey=R0…`.

---

**Step 1 — Open your link**

The `?pdkey=` part gets saved into the browser's local storage automatically,
and disappears from the URL bar. Your PagerDuty routing key is now locked in
for this browser.

---

**Step 2 — Trigger the failure**

The homepage has a checkout card front and centre. Click **Pay**. You'll see
the order pipeline run through 5 stages and fail at "Charging payment".

Just before the failure alert fires, the storefront sends two **change
events** to PagerDuty, backdated to look like they happened minutes before the
outage:

| Change event | Appears as | What it says |
|---|---|---|
| GitHub deploy | −3 minutes ago | `payment-service v2.41.0` deployed to production by `github-actions` |
| LaunchDarkly flag | −2 minutes ago | `checkout-v2-enabled` turned ON for 100% of users |

---

**Step 3 — Show it in PagerDuty**

- An incident opens on **`JR-payment-gateway`** and pages whoever is on call
  (you, if it's a weekday between 9 and 5)
- **`JR-customer-checkout`** shows business impact
- The **Recent Changes** tab on the incident shows both change events —
  perfect for the "what changed just before this broke?" moment
- If Slack is set up, **`#jr-incidents`** gets a notification

---

**Step 4 — Keep demoing**

Clicking Pay again adds to the same open incident — it won't create a flood of
new ones. The dedup key is stable per scenario.

---

**Step 5 — Resolve**

Resolve the incident in PagerDuty normally. Or open the ops console in the
storefront (Ctrl/Cmd+Shift+K) and click **Resolve all**.

---

### The ops console

The storefront looks like a normal shop. The ops console is hidden by design — 
your audience doesn't see it. Open it with:

- **Ctrl/Cmd + Shift + K**, or
- The small "operations console" link in the footer, or
- Add `?ops=1` to the URL

The console shows your routing key, change event keys, and a live log of every
event dispatched in the current browser session.

To pre-load all three keys into the URL at once:

```
https://demo.example.com/?pdkey=<routing-key>&pdchangekey=<github-key>&pdldkey=<ld-key>
```

`./greenagonia urls JR` shows all three values.

---

## Day-to-day operations

### Add another admin

```bash
./greenagonia admin add AB "Alice Bell" alice@example.com
./greenagonia deploy
```

The deploy plan only touches Alice's new resources. Everyone else's stack is
untouched.

### Remove an admin

```bash
./greenagonia admin remove AB
./greenagonia deploy
```

Only AB's resources are destroyed. Everyone else is fine.

### See who's configured

```bash
./greenagonia admin list
```

### Update your PagerDuty token

```bash
./greenagonia token
./greenagonia deploy
```

### Change the storefront URL

```bash
./greenagonia site-url https://new-url.example.com
./greenagonia deploy
./greenagonia urls     # confirm the updated links
```

---

## Slack setup

Slack is optional and easy to skip on a first deploy. Add it after you've
confirmed the basic demo works.

### What you need

| Thing | Where to get it |
|---|---|
| Slack V2 integration enabled in PagerDuty | PagerDuty → Integrations → Slack V2 |
| Slack bot token (`xoxb-…`) | Your Slack app → OAuth & Permissions → Bot User OAuth Token |
| PagerDuty user-level token | PagerDuty → My Profile → User Settings → Create API User Token |
| Slack workspace ID (starts with `T`) | PagerDuty → Integrations → Slack V2 → workspace ID |

Required Slack bot scopes: `channels:manage`, `channels:join`, `users:read`, `users:read.email`

### Enable Slack

```bash
./greenagonia slack-token    # paste your xoxb-… token
./greenagonia user-token     # paste your PD user-level token
./greenagonia setup          # set enable_slack = true when prompted
./greenagonia deploy
```

After deploy, each admin gets a `{initials}-incidents` Slack channel (e.g.
`#jr-incidents`) with a PagerDuty Slack connection. There's also a shared
`#greenagonia-incidents` channel for the whole team.

### Channel already exists?

If Terraform fails with `name_taken`, the channel already exists in Slack.
Import it first:

```bash
# Find the channel ID
curl -s -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  "https://slack.com/api/conversations.list" \
  | jq '.channels[] | select(.name=="jr-incidents") | .id'

# Import it into Terraform state
terraform -chdir=terraform-shared import 'slack_conversation.incidents["JR"]' <channel-id>
```

Then re-run `./greenagonia deploy`.

---

## Troubleshooting

| What you see | What's wrong | How to fix it |
|---|---|---|
| "alert NOT sent — no routing key configured" | No key saved in this browser | Open your `?pdkey=` link again, or double-click the header logo in the storefront to paste the key |
| "alert NOT sent — HTTP 400" | Wrong key type | Must be the orchestration routing key from `./greenagonia urls` — not a REST token |
| Change events missing from *Recent Changes* | Change event keys not set | Open the ops console, paste the keys from `./greenagonia urls`, or use the full `?pdkey=&pdchangekey=&pdldkey=` URL |
| `terraform apply` → 401 Unauthorized | Wrong token or wrong region | EU accounts need region `eu` — run `./greenagonia token` then `./greenagonia setup` |
| Apply fails on incident workflows (404) | Account doesn't have the entitlement | Set `enable_incident_workflows = false` in `terraform-shared/config.auto.tfvars.json` and redeploy |
| Apply fails on `pagerduty_slack_connection` (401/403) | Missing or wrong user-level token | Run `./greenagonia user-token` — this needs a *user* token, not the account REST API key |
| Slack: `name_taken` | Channel already exists | Import it — see Slack setup above |
| Alert grouping fails | No AIOps add-on | In `terraform-shared/services.tf` change `alert_grouping_parameters` to `type = "time"` |
| "user with this email already exists" | Email is already a PagerDuty user | Use a different email address |
| Incident lands on `unrouted-events` | Alert payload didn't match any routing rule | Check the routing key is the orchestration key from `./greenagonia urls` |

---

## Teardown

```bash
./greenagonia destroy
```

Type `destroy` to confirm. All PagerDuty resources are removed, including the
shared personas. Your local state files stay in `state/` — delete them
manually when you're done.

---

## Backing up state

Terraform keeps track of everything it's created in `state/`. It's gitignored,
so it lives only on your machine. Back it up before anything risky:

```bash
./backup-state.sh
```

Creates a timestamped archive in `state/backups/` and keeps the 10 most
recent. If you ever need to share control with another operator, migrate to a
remote backend (HCP Terraform free tier works well).

---

## Repo layout

```
greenagonia/
├── get-binary.sh         download the right pre-built binary for this OS
├── greenagonia           the CLI (appears after running get-binary.sh)
├── install.sh            Linux: installs binary + Terraform globally
├── quickstart.sh         macOS: installs Go + Terraform via Homebrew, builds from source
├── backup-state.sh       backs up state/ to a dated archive
├── Makefile              build / build-all / clean
├── site/                 the storefront (static HTML/CSS/JS — just serve it)
├── state/                Terraform state (gitignored — don't delete this)
├── terraform-shared/     all the PagerDuty infrastructure as Terraform code
│   ├── main.tf           provider config, service catalogue, persona definitions
│   ├── users.tf          PagerDuty users (personas + one per admin)
│   ├── teams.tf          teams and memberships
│   ├── schedules.tf      on-call schedules
│   ├── escalation_policies.tf
│   ├── services.tf       technical services
│   ├── business_services.tf
│   ├── orchestration.tf  event orchestration and routing rules
│   ├── greenagonia_platform.tf  shared team + 5 platform services
│   ├── slack_channels.tf
│   ├── automation.tf     automation actions
│   ├── workflows.tf      incident workflows
│   ├── variables.tf
│   └── outputs.tf        routing keys, storefront URLs
└── cli/
    ├── main.go           scenario data + CLI helpers
    └── shared.go         all environment management commands
```
