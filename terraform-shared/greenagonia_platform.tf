# ===========================================================================
# greenagonia_platform.tf — shared/public Greenagonia team + platform services.
# ---------------------------------------------------------------------------
# One PUBLIC team (Greenagonia) — all 5 personas as responders, all admins
# as managers. default_role = "viewer" means every account member can see it.
#
# Five platform-level services owned by this team, each with alert grouping
# and an Events v2 integration. A dedicated event orchestration (separate
# routing key) routes to them by payload.custom_details.service — the same
# field the per-admin routers use, so scenarios can target platform services
# without any storefront changes.
#
# Services:
#   api-gateway          Central API gateway — inbound routing, rate limiting, auth.
#   data-platform        Data warehouse, pipelines, and analytics.
#   identity-service     SSO, directory, and access management.
#   infrastructure       Core cloud infrastructure: networking, compute, storage.
#   platform-engineering Internal developer platform and tooling.
# ===========================================================================

locals {
  platform_services = {
    "api-gateway"          = "Central API gateway — inbound routing, rate limiting, auth."
    "data-platform"        = "Data warehouse, pipelines, and analytics."
    "identity-service"     = "SSO, directory, and access management."
    "infrastructure"       = "Core cloud infrastructure: networking, compute, storage."
    "platform-engineering" = "Internal developer platform and tooling."
  }
}

# ---- Public team -----------------------------------------------------------

resource "pagerduty_team" "greenagonia" {
  name        = "Greenagonia"
  description = "Shared platform team. All personas and admins are members."
}

resource "pagerduty_team_membership" "greenagonia_persona" {
  for_each = local.personas

  user_id = pagerduty_user.persona[each.key].id
  team_id = pagerduty_team.greenagonia.id
  role    = "responder"
}

resource "pagerduty_team_membership" "greenagonia_admin" {
  for_each = var.admins

  user_id = local.all_admin_ids[each.key]
  team_id = pagerduty_team.greenagonia.id
  role    = "manager"
}

# ---- Schedule & escalation policy -----------------------------------------

resource "pagerduty_schedule" "greenagonia" {
  name      = "Greenagonia Platform On-Call"
  time_zone = var.schedule_time_zone
  teams     = [pagerduty_team.greenagonia.id]

  layer {
    name                         = "Platform persona rotation"
    start                        = local.rotation_epoch
    rotation_virtual_start       = local.rotation_epoch
    rotation_turn_length_seconds = 604800
    users                        = local.persona_ids
  }
}

resource "pagerduty_escalation_policy" "greenagonia" {
  name      = "Greenagonia Platform On-Call"
  num_loops = 2
  teams     = [pagerduty_team.greenagonia.id]

  rule {
    escalation_delay_in_minutes = 10
    target {
      type = "schedule_reference"
      id   = pagerduty_schedule.greenagonia.id
    }
  }
}

# ---- 5 platform services ---------------------------------------------------

resource "pagerduty_service" "platform" {
  for_each = local.platform_services

  name                    = each.key
  description             = each.value
  escalation_policy       = pagerduty_escalation_policy.greenagonia.id
  auto_resolve_timeout    = 14400
  acknowledgement_timeout = 1800
  alert_creation          = "create_alerts_and_incidents"
}

resource "pagerduty_alert_grouping_setting" "platform" {
  for_each = local.platform_services

  name     = "${each.key}-grouping"
  type     = "content_based"
  services = [pagerduty_service.platform[each.key].id]

  config {
    aggregate   = "all"
    fields      = ["group", "class"]
    time_window = 600
  }
}

resource "pagerduty_service_integration" "platform_events_v2" {
  for_each = local.platform_services

  service = pagerduty_service.platform[each.key].id
  name    = "Events API v2"
  type    = "events_api_v2_inbound_integration"
}

# ---- Shared event orchestration -------------------------------------------

resource "pagerduty_event_orchestration" "greenagonia" {
  name        = "Greenagonia Platform Router"
  description = "Routes events to platform services by payload.custom_details.service."
  team        = pagerduty_team.greenagonia.id
}

resource "pagerduty_event_orchestration_router" "greenagonia" {
  event_orchestration = pagerduty_event_orchestration.greenagonia.id

  set {
    id = "start"

    dynamic "rule" {
      for_each = local.platform_services
      content {
        label = "Route to ${rule.key}"
        condition {
          expression = "event.custom_details.service matches '${rule.key}'"
        }
        actions {
          route_to = pagerduty_service.platform[rule.key].id
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
