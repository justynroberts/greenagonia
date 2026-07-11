# Greenagonia

![Latest Release](https://img.shields.io/github/v/release/justynroberts/greenagonia)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-blue)
![Terraform](https://img.shields.io/badge/terraform-%3E%3D1.5-844FBA?logo=terraform)
![PagerDuty](https://img.shields.io/badge/PagerDuty-demo%20environment-06AC38?logo=pagerduty)

PagerDuty's fictional demo company. One command stands up a complete, realistic
on-call environment — services, schedules, escalation policies, event
orchestration — for every person on your team, inside one shared PagerDuty
account. Break the checkout on a live demo storefront; watch the incident fire.

---

## Quick start

```bash
git clone https://github.com/justynroberts/greenagonia.git
cd greenagonia
./get-binary.sh                   # downloads the right binary for your OS
./greenagonia setup               # guided setup: token, region, first admin
./greenagonia deploy              # builds everything in PagerDuty (~2 min)
./greenagonia urls                # get your personal storefront link
```

Open the link, click **Pay**, watch the incident land in PagerDuty.

> **Need Terraform?** `brew install terraform` on macOS. Linux: `install.sh` handles it.

---

## The demo

1. Open your `?pdkey=` storefront link — routing key saves to the browser automatically
2. Click **Pay** — the checkout pipeline fails at payment
3. Two backdated **change events** fire first (a deploy and a feature flag), so
   the *Recent Changes* tab already has context when the incident opens
4. Incident lands on your service, pages you, lights up your business service
5. Click Pay again — same incident, no duplicates
6. Resolve from PagerDuty or hit **Resolve all** in the ops console (Ctrl/Cmd+Shift+K)

---

## Commands

```bash
# First run
./greenagonia setup

# Tokens
./greenagonia token                        # PagerDuty REST API token
./greenagonia user-token                   # user-level token (Slack)
./greenagonia slack-token                  # Slack bot token

# Admins
./greenagonia admin add JR "Jo Roberts" jo@example.com
./greenagonia admin remove JR
./greenagonia admin list

# Deploy
./greenagonia deploy
./greenagonia destroy

# Links
./greenagonia urls                         # all admins
./greenagonia urls JR                      # one admin
./greenagonia site-url https://mysite.com  # set storefront URL
```

---

## What each admin gets

| Resource | Example (initials: `JR`) |
|---|---|
| PagerDuty user | your real name + email — gets paged |
| Team | `JR-SRE-TEAM` |
| 8 microservices | `JR-payment-gateway`, `JR-checkout-api`, `JR-user-auth` … |
| 2 business services | `JR-customer-checkout`, `JR-product-discovery` |
| On-call schedules | Primary (you, Mon–Fri 9–5) + Secondary/Tertiary (fictional personas) |
| Event orchestration | Personal routing key → your services |
| Slack (optional) | `#jr-incidents` + PagerDuty connection |

Five fictional teammates staff nights, weekends, and backup rotations so the
account looks like a real company. Each admin's stack is **invisible to other
admins** — multiple demos can run simultaneously from the same storefront.

---

## Hosting the storefront

The storefront is plain HTML/CSS/JS — no build step.

```bash
# Local
cd site && python3 -m http.server 8080

# Docker / nginx (included docker-compose.yml)
docker compose up -d

# Shared (host site/ anywhere, then register the URL)
./greenagonia site-url https://demo.example.com
./greenagonia deploy && ./greenagonia urls
```

---

## Installing

| Method | Command |
|---|---|
| **Download binary** (recommended) | `./get-binary.sh` — detects macOS arm64/amd64 or Linux amd64/arm64 |
| **Linux global install** | `curl -fsSL .../install.sh \| bash` — puts binary in `/usr/local/bin`, installs Terraform |
| **Build from source** | `make build` (needs Go ≥ 1.25) · macOS: `./quickstart.sh` installs prereqs |

Full walkthrough: **[ADMIN-GUIDE.md](ADMIN-GUIDE.md)**
