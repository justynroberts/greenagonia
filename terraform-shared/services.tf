# ===========================================================================
# services.tf — every pagerduty_service (technical + shared catch-all).
# ---------------------------------------------------------------------------
# Names are prefixed with the admin's initials (JR-payment-gateway) so N
# copies of the catalogue coexist in one account. The router
# (orchestration.tf) matches the PLAIN service name from
# payload.custom_details.service, so the storefront payload works against
# any admin's stack unchanged.
#
# Each technical service gets ONE Events v2 integration (change events +
# direct alerts). payment-gateway gets a SECOND integration so you can
# attach distinct change sources: one key for GitHub Actions deploys and one
# for LaunchDarkly flag changes — both appear in PagerDuty's Recent Changes
# tab and the incident timeline.
# ===========================================================================

resource "pagerduty_service" "tech" {
  for_each = local.admin_tech

  name                    = "${each.value.admin}-${each.value.service}"
  description             = local.technical_services[each.value.service]
  escalation_policy       = pagerduty_escalation_policy.admin[each.value.admin].id
  auto_resolve_timeout    = 14400 # 4h
  acknowledgement_timeout = 1800  # 30m
  alert_creation          = "create_alerts_and_incidents"
}

# Alert grouping split out of the service block (provider migration requirement).
# Deterministic content-based grouping: alerts carry group=<scenario> and
# class="greenagonia-scenario", so every alert from the same scenario burst
# rolls up into one incident per service.
resource "pagerduty_alert_grouping_setting" "tech" {
  for_each = local.admin_tech

  name     = "${each.value.admin}-${each.value.service}-grouping"
  type     = "content_based"
  services = [pagerduty_service.tech[each.key].id]

  config {
    aggregate   = "all"
    fields      = ["group", "class"]
    time_window = 600
  }
}

# Events API v2 integration on every technical service — the key this
# produces is used to post change events directly to the service (e.g. GitHub
# Actions deploys). Also works for direct alert injection.
resource "pagerduty_service_integration" "events_v2" {
  for_each = local.admin_tech

  service = pagerduty_service.tech[each.key].id
  name    = "Events API v2 / GitHub Actions"
  type    = "events_api_v2_inbound_integration"
}

# Second change-event integration on payment-gateway only — dedicated to
# LaunchDarkly flag changes. Having two distinct keys makes the PD integrations
# tab show two named sources and lets you demo separate change-event streams.
resource "pagerduty_service_integration" "events_v2_ld" {
  for_each = { for k, v in local.admin_tech : k => v if v.service == "payment-gateway" }

  service = pagerduty_service.tech[each.key].id
  name    = "Events API v2 / LaunchDarkly"
  type    = "events_api_v2_inbound_integration"
}

# Shared catch-all: every admin's orchestration falls through to this single
# service, so an event with a typo'd custom_details.service is never
# silently swallowed.
resource "pagerduty_service" "catchall" {
  name              = "unrouted-events"
  description       = "Catches events no admin's orchestration router could match."
  escalation_policy = pagerduty_escalation_policy.catchall.id
  alert_creation    = "create_alerts_and_incidents"
}
