# ===========================================================================
# services.tf — technical + business services, dependency edges, change-event
# integrations, catch-all.
# ---------------------------------------------------------------------------
# The technical-service set is driven by locals.technical_services (main.tf).
# Adding a service there cascades through this file via for_each and into
# the router (orchestration.tf) and automation actions (automation.tf).
# ===========================================================================

# ---- Technical services ---------------------------------------------------

resource "pagerduty_service" "tech" {
  for_each = local.technical_services

  name                    = each.key
  description             = each.value
  escalation_policy       = pagerduty_escalation_policy.default.id
  auto_resolve_timeout    = 14400 # seconds — 4h
  acknowledgement_timeout = 1800  # seconds — 30m
  alert_creation          = "create_alerts_and_incidents"

  # Intelligent (AIOps) alert grouping: PagerDuty's ML-based engine groups
  # alerts that look like they're describing the same problem — across
  # different signals, sources, and time. The scenarios deliberately fire
  # 3-4 alerts at each affected service in quick succession to give the
  # grouper something to chew on, so a 12-alert scenario lands as ~4
  # incidents (one per service) instead of 12.
  #
  # Requires the AIOps add-on on your PagerDuty plan. Without it, apply
  # fails with a 403/feature-not-enabled error — fall back to
  # type = "time" in that case.
  #
  # The provider prints a deprecation warning suggesting the separate
  # pagerduty_alert_grouping_setting resource — that resource currently
  # creates the setting but doesn't always bind it to services on every
  # account/plan combination, so we keep the inline block which is the
  # proven path. The warning is harmless.
  alert_grouping_parameters {
    type = "intelligent"
  }
}

# Events API v2 integration on every technical service. The integration key
# this produces doubles as the inbound key for change events:
#   POST https://events.pagerduty.com/v2/change/enqueue
# The CLI uses these keys to post the "bad GitHub deploy" change event that
# precedes each scenario's alert cascade — PagerDuty then correlates the
# change with the incident on that service's timeline.
resource "pagerduty_service_integration" "events_v2" {
  for_each = local.technical_services

  service = pagerduty_service.tech[each.key].id
  name    = "Events API v2 / Change Events"
  type    = "events_api_v2_inbound_integration"
}

# Catch-all service: events that don't match any router rule land here so
# nothing is silently dropped during a demo.
resource "pagerduty_service" "catchall" {
  name              = "unrouted-events"
  description       = "Catches events the orchestration router couldn't match."
  escalation_policy = pagerduty_escalation_policy.default.id
  alert_creation    = "create_alerts_and_incidents"
}

# ---- Business services ----------------------------------------------------

resource "pagerduty_business_service" "biz" {
  for_each = local.business_services

  name        = each.key
  description = each.value.description
  team        = pagerduty_team.sre.id
}

# Wire every technical service up to the business services that depend on it.
resource "pagerduty_service_dependency" "biz_to_tech" {
  for_each = local.service_dependencies

  dependency {
    supporting_service {
      id   = pagerduty_service.tech[each.value.tech].id
      type = "service"
    }
    dependent_service {
      id   = pagerduty_business_service.biz[each.value.biz].id
      type = "business_service"
    }
  }
}
