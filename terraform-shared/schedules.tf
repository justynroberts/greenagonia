# ===========================================================================
# schedules.tf — the three on-call schedules per admin.
# ---------------------------------------------------------------------------
# Primary:   admin on call Mon-Fri 09:00-17:00 (var.schedule_time_zone);
#            outside working hours the persona rotation covers. Layer 1 is
#            the 24/7 persona base; layer 2 overlays working hours — later
#            layers take precedence in PagerDuty.
# Secondary: the 5 personas, weekly rotation.
# Tertiary:  the 5 personas, weekly rotation, staggered by two turns so a
#            different persona is up than on secondary.
# Rotation epochs are fixed dates so plans are deterministic.
# ===========================================================================

locals {
  rotation_epoch = "2025-01-06T00:00:00Z" # a Monday

  persona_ids = [for k in local.persona_order : pagerduty_user.persona[k].id]
  persona_ids_staggered = concat(
    slice(local.persona_ids, 2, length(local.persona_ids)),
    slice(local.persona_ids, 0, 2),
  )
}

resource "pagerduty_schedule" "primary" {
  for_each = var.admins

  name      = "${each.key} Primary On-Call"
  time_zone = var.schedule_time_zone
  teams     = [pagerduty_team.admin[each.key].id]

  # Layer 1 (base, lowest precedence): personas rotate weekly, 24/7.
  layer {
    name                         = "Out-of-hours rotation"
    start                        = local.rotation_epoch
    rotation_virtual_start       = local.rotation_epoch
    rotation_turn_length_seconds = 604800 # 7 days
    users                        = local.persona_ids
  }

  # Layer 2 (overlay, higher precedence): the admin, working hours only.
  layer {
    name                         = "Working hours — ${each.value.name}"
    start                        = local.rotation_epoch
    rotation_virtual_start       = local.rotation_epoch
    rotation_turn_length_seconds = 604800
    users                        = [local.all_admin_ids[each.key]]

    dynamic "restriction" {
      for_each = [1, 2, 3, 4, 5] # Monday..Friday
      content {
        type              = "weekly_restriction"
        start_day_of_week = restriction.value
        start_time_of_day = "09:00:00"
        duration_seconds  = 28800 # 8h → 09:00-17:00
      }
    }
  }
}

resource "pagerduty_schedule" "secondary" {
  for_each = var.admins

  name      = "${each.key} Secondary On-Call"
  time_zone = var.schedule_time_zone
  teams     = [pagerduty_team.admin[each.key].id]

  layer {
    name                         = "Persona weekly rotation"
    start                        = local.rotation_epoch
    rotation_virtual_start       = local.rotation_epoch
    rotation_turn_length_seconds = 604800
    users                        = local.persona_ids
  }
}

resource "pagerduty_schedule" "tertiary" {
  for_each = var.admins

  name      = "${each.key} Tertiary On-Call"
  time_zone = var.schedule_time_zone
  teams     = [pagerduty_team.admin[each.key].id]

  layer {
    name                         = "Persona weekly rotation (staggered)"
    start                        = local.rotation_epoch
    rotation_virtual_start       = local.rotation_epoch
    rotation_turn_length_seconds = 604800
    users                        = local.persona_ids_staggered
  }
}
