# Greenagonia

Greenagonia is a realistic fake company you use to demo PagerDuty. It looks like a real e-commerce business — complete website, real-looking services, on-call schedules, escalation policies, the works. You break the checkout, an incident fires, PagerDuty pages you.

**The key idea:** everyone on your team gets their own private copy of the full environment inside one shared PagerDuty account. You each have your own routing key, your own services, your own on-call schedule. One storefront, many demos running at the same time.

---

## Get started in 5 commands

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia
./get-binary.sh                   # downloads the right binary for your OS
./greenagonia setup               # guided setup: token, region, first admin
./greenagonia deploy              # builds everything in PagerDuty (~2 min)
```

Then get your personal storefront link:

```bash
./greenagonia urls
```

Open the link, click **Pay**, and watch the incident land in PagerDuty.

---

## What the demo looks like

1. **Open your storefront link** — a polished outdoor-gear e-commerce site.
   Your PagerDuty routing key is pre-loaded into the page automatically.

2. **Click Pay** — the checkout pipeline runs through 5 stages: validate →
   inventory → payment → order → confirm. It fails at payment.

3. **Two change events fire first**, backdated to look like real deployments
   that happened minutes before the outage:
   - `payment-service v2.41.0` deployed to production (−3 min)
   - `checkout-v2-enabled` feature flag turned ON (−2 min)

4. **The alert hits PagerDuty.** An incident opens on your `payment-gateway`
   service. Your business service (`customer-checkout`) shows business impact.
   The *Recent Changes* tab shows the two deploys. If Slack is wired up, your
   `#incidents` channel gets the notification.

5. **Repeated failures deduplicate** — clicking Pay again adds to the same
   incident, not a flood of new ones.

6. **Resolve** from PagerDuty, or hit **Resolve all** in the storefront ops
   console (open it with Ctrl/Cmd+Shift+K).

---

## Everything you get as an admin

When you add yourself (example initials: `JR`), Greenagonia creates:

| What | Name |
|---|---|
| Your PagerDuty user | your real name + email — this is who gets paged |
| Your team | `JR-SRE-TEAM` |
| 8 microservices | `JR-payment-gateway`, `JR-checkout-api`, `JR-user-auth`, `JR-product-catalog`, `JR-recommendation-engine`, `JR-search-service`, `JR-order-service`, `JR-notification-service` |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` |
| On-call schedules | Primary (you, Mon–Fri 9–5), Secondary and Tertiary (fictional personas) |
| Escalation policy | Primary → Secondary → Tertiary, 2 escalation loops |
| Event orchestration | Your personal routing key — alerts sent here land on your services |
| Slack (optional) | `#jr-incidents` channel with PagerDuty connected |

Five fictional teammates — Sarah SRE, Dan Developer, Matt Manager, Pablo Platform, Sam Security — staff every team's nights, weekends, and backup rotations. They make the account look real; pages to them go nowhere.

Each admin's resources are **invisible to other admins**. You can run 10 demos at the same time from the same storefront — each person just uses their own `?pdkey=` link.

---

## CLI commands

```bash
# Setup
./greenagonia setup                        # first-time wizard
./greenagonia token                        # update your PagerDuty API token
./greenagonia user-token                   # set user-level token (needed for Slack)
./greenagonia slack-token                  # set Slack bot token

# Managing admins
./greenagonia admin add JR "Jo Roberts" jo@example.com
./greenagonia admin add JR "Jo Roberts" jo@example.com jo@slack.com   # include Slack email
./greenagonia admin remove JR             # removes JR on next deploy
./greenagonia admin list                  # see everyone configured

# Deploy
./greenagonia deploy                      # apply all changes to PagerDuty
./greenagonia destroy                     # tear everything down

# Links
./greenagonia urls                        # print all per-admin storefront links
./greenagonia urls JR                     # just one admin
./greenagonia site-url https://mysite.com # set where the storefront is hosted
```

---

## Hosting the storefront

The storefront is a plain HTML/CSS/JS site — no build step, no dependencies.

**Quick local demo:**
```bash
cd site && python3 -m http.server 8080
# open http://localhost:8080/?pdkey=<your-routing-key>
```

**Shared deployment** (one URL for the whole team):

Host the contents of `site/` anywhere — GitHub Pages, S3, an EC2 instance.
Then tell Greenagonia where it lives:

```bash
./greenagonia site-url https://demo.yourcompany.com
./greenagonia deploy
./greenagonia urls                        # links now include your URL
```

The `?pdkey=` value is saved in the browser's local storage on first visit
and removed from the URL bar, so the link stays clean for sharing.

---

## Installing

### Recommended (macOS + Linux)

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia
./get-binary.sh
```

`get-binary.sh` detects your OS and chip (macOS Apple Silicon/Intel, Linux
x86_64/ARM64) and downloads the right binary from the
[latest release](https://github.com/justynroberts/greenagonia/releases/latest).

You also need Terraform ≥ 1.5:
```bash
brew install terraform      # macOS
```

### Linux server (installs globally)

```bash
curl -fsSL https://raw.githubusercontent.com/justynroberts/greenagonia/master/install.sh | bash
```

Puts `greenagonia` in `/usr/local/bin` and installs Terraform 1.9.8.

### Build from source

```bash
cd greenagonia && make build    # needs Go ≥ 1.22
# On macOS: ./quickstart.sh installs Go + Terraform via Homebrew first
```

---

## Repo layout

```
greenagonia/
├── get-binary.sh         ← start here: downloads the binary for your OS
├── greenagonia           ← the CLI (after running get-binary.sh)
├── site/                 ← the storefront (static site — just serve it)
├── terraform-shared/     ← all the PagerDuty infrastructure as code
├── state/                ← terraform state lives here (gitignored)
├── install.sh            ← Linux global install script
├── quickstart.sh         ← macOS build-from-source helper
└── backup-state.sh       ← backs up state/ to a dated archive
```

Full setup walkthrough: **[ADMIN-GUIDE.md](ADMIN-GUIDE.md)**
