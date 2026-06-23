# Greenagonia storefront

A polished demo e-commerce site — sustainable outdoor gear — with a built-in chaos engine that simulates realistic checkout failures and dispatches them to **PagerDuty** via the Events API v2.

Zero dependencies. Zero build step. Pure HTML/CSS/JS.

## Run it

```bash
cd shared-usage/shared-site
python3 -m http.server 8080
# open http://localhost:8080
```

Works from `file://` too, though `?pdkey=` URL pre-loading requires a real HTTP origin (CORS).

## Demo flow

1. **Front page leads with checkout** — the hero has a live checkout card (bestseller pre-loaded), so a demo is one click from a failing transaction the moment the page renders.
2. **Check out** — simulated 5-stage pipeline (validate → inventory → payment → order → confirm) with live progress.
3. **Open the Operations Console** (deliberately discreet):
   - press **Ctrl/⌘ + Shift + K**, or
   - **triple-click the Greenagonia logo in the footer**, or
   - visit any page with **`?ops=1`**
4. **Add your PagerDuty routing key** in the console, or pre-load via URL:
   - `?pdkey=<routing-key>` — saved to localStorage, scrubbed from the address bar
   - `?pdchangekey=<change-key>` — key for GitHub deploy change events
   - `?pdldkey=<ld-key>` — key for LaunchDarkly flag change events
5. **Pick a failure scenario** and run a checkout.

## Change events

When an incident fires, the site first posts two **PagerDuty change events** (shown on the incident's *Recent Changes* tab) with backdated timestamps to simulate a deployment pipeline:

| Event | Backdated by | Details |
|---|---|---|
| GitHub deploy | 3 minutes | `v2.41.0` of `payment-service` deployed to `production` by `github-actions` |
| LaunchDarkly flag | 2 minutes | `checkout-v2-enabled` feature flag turned ON for all users |

These use separate integration keys from the main alert key. Set them in the ops console or via URL (`?pdchangekey=` / `?pdldkey=`). The ops console shows them in the "Change event key" fields. If either key is unset its change event is skipped silently.

## Failure scenarios

| Scenario | Fails at | Severity | Alert |
|---|---|---|---|
| All systems healthy | — | — | none |
| Payment gateway timeout | payment | critical | PSP p99 > 30s SLA, circuit breaker open |
| Card declines spiking | payment | error | decline rate vs 2.1% baseline |
| Inventory service 500s | inventory | error | HTTP 500 on `POST /v1/reservations` |
| DB pool exhausted | order | critical | 100/100 connections, writes failing |

## PagerDuty integration

- Alerts go to `https://events.pagerduty.com/v2/enqueue` directly from the browser (CORS-enabled endpoint).
- **Dedup keys** are per-scenario (`greenagonia-payment_timeout`, …) — repeated failures group into one incident.
- Alerts carry rich `custom_details`: error rates, pod names, latency percentiles, order ID, cart value, runbook and dashboard links.
- **Resolve all** in the console sends `resolve` events for every active dedup key.
- **Send test alert** fires an `info` event to verify the key.

## URL parameters reference

| Parameter | Stored as | Purpose |
|---|---|---|
| `?pdkey=` | `gn_pd_key` | Main event routing key (alert events) |
| `?pdchangekey=` | `gn_pd_change_key` | Change event key — GitHub deploy events |
| `?pdldkey=` | `gn_pd_ld_key` | Change event key — LaunchDarkly flag events |
| `?ops=1` | — | Open the ops console directly |

All keys are stored in localStorage and scrubbed from the address bar immediately.

## Files

| File | Purpose |
|---|---|
| `index.html` | Front page: hero, featured gear, story, impact, CTA |
| `shop.html` | Full catalog with category filters |
| `shared.js` | Header, footer, cart drawer, checkout modal, ops console |
| `styles.css` | Design system (dark forest theme, Fraunces/Inter/JetBrains Mono) |
| `products.js` | Catalog with generated SVG product art (no external images) |
| `app.js` | Cart (localStorage-persisted), checkout pipeline, UI glue |
| `chaos.js` | Failure scenarios, Events API v2 client (alerts + change events), console |
