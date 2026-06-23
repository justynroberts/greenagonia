# ===========================================================================
# orchestration.tf — one event orchestration + dynamic router PER ADMIN.
# ---------------------------------------------------------------------------
# Each admin gets their own inbound routing key. The router matches the
# PLAIN service name in payload.custom_details.service ("payment-gateway")
# and routes to the admin's prefixed service (JR-payment-gateway) — so the
# CLI scenarios and the storefront payload work against any admin's stack
# without modification; the key alone decides whose services light up.
# Unmatched events fall through to the shared catch-all.
# ===========================================================================

resource "pagerduty_event_orchestration" "admin" {
  for_each = var.admins

  name        = "Greenagonia Event Router — ${each.key}"
  description = "Routes events to ${each.key}-* services by payload.custom_details.service."
  team        = pagerduty_team.admin[each.key].id
}

resource "pagerduty_event_orchestration_router" "admin" {
  for_each = var.admins

  event_orchestration = pagerduty_event_orchestration.admin[each.key].id

  set {
    id = "start"

    dynamic "rule" {
      for_each = local.technical_services
      content {
        label = "Route to ${each.key}-${rule.key}"
        condition {
          expression = "event.custom_details.service matches '${rule.key}'"
        }
        actions {
          route_to = pagerduty_service.tech["${each.key}/${rule.key}"].id
        }
      }
    }
  }

  catch_all {
    actions {
      route_to = pagerduty_service.catchall.id
    }
  }
}
