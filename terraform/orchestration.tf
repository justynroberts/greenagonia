# ===========================================================================
# orchestration.tf — single inbound key, dynamic routing by service name.
# ---------------------------------------------------------------------------
# The router below generates one rule per entry in locals.technical_services.
# Each rule matches events where custom_details.service == "<name>" and
# routes the event to that service's PagerDuty service. Anything unmatched
# falls through to the catch-all so the demo never silently swallows events.
# ===========================================================================

resource "pagerduty_event_orchestration" "main" {
  name        = "Greenagonia Event Router"
  description = "Dynamic router: dispatches events to services by payload.custom_details.service."
  team        = pagerduty_team.sre.id
}

resource "pagerduty_event_orchestration_router" "main" {
  event_orchestration = pagerduty_event_orchestration.main.id

  set {
    id = "start"

    dynamic "rule" {
      for_each = local.technical_services
      content {
        label = "Route to ${rule.key}"
        condition {
          expression = "event.custom_details.service matches '${rule.key}'"
        }
        actions {
          route_to = pagerduty_service.tech[rule.key].id
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
