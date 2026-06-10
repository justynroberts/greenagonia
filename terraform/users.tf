# ===========================================================================
# users.tf — three on-call users, SRE team, weekly rotation, escalation policy.
# ---------------------------------------------------------------------------
# Three PagerDuty users are created:
#
#   primary    var.user_email                       admin / team manager
#   secondary  var.secondary_user_email             user  / team responder
#   tertiary   var.tertiary_user_email              user  / team responder
#
# The CLI derives the secondary/tertiary emails by adding "+oncall2" /
# "+oncall3" before the "@" of the primary email — three distinct PD
# users, one human inbox. The weekly rotation cycles between them; the
# escalation policy points at the schedule first, then falls back to the
# secondary user as a hard backup.
# ===========================================================================

resource "pagerduty_user" "primary" {
  name  = var.user_name
  email = var.user_email
  role  = "admin"
}

resource "pagerduty_user" "secondary" {
  name  = "Secondary On-Call"
  email = var.secondary_user_email
  role  = "user"
}

resource "pagerduty_user" "tertiary" {
  name  = "Tertiary On-Call"
  email = var.tertiary_user_email
  role  = "user"
}

resource "pagerduty_team" "sre" {
  name        = "Site Reliability Engineering"
  description = "Primary on-call team for Greenagonia services."
}

resource "pagerduty_team_membership" "primary" {
  user_id = pagerduty_user.primary.id
  team_id = pagerduty_team.sre.id
  role    = "manager"
}

resource "pagerduty_team_membership" "secondary" {
  user_id = pagerduty_user.secondary.id
  team_id = pagerduty_team.sre.id
  role    = "responder"
}

resource "pagerduty_team_membership" "tertiary" {
  user_id = pagerduty_user.tertiary.id
  team_id = pagerduty_team.sre.id
  role    = "responder"
}

# ---- Weekly rotation across the three on-call users -----------------------
# Fixed Monday 2025-01-06 UTC as the rotation epoch so the cycle is
# deterministic across deploys. Turn length is 7 days.
resource "pagerduty_schedule" "primary" {
  name      = "Primary On-Call Rotation"
  time_zone = "Etc/UTC"
  teams     = [pagerduty_team.sre.id]

  layer {
    name                         = "Weekly Rotation"
    start                        = "2025-01-06T00:00:00Z"
    rotation_virtual_start       = "2025-01-06T00:00:00Z"
    rotation_turn_length_seconds = 604800 # 7 days
    users = [
      pagerduty_user.primary.id,
      pagerduty_user.secondary.id,
      pagerduty_user.tertiary.id,
    ]
  }
}

# ---- Escalation policy ----------------------------------------------------
# Level 1: whoever is on call this week (rotated by the schedule above)
# Level 2: secondary user as a hard backup after 15 min
# Loops twice.
resource "pagerduty_escalation_policy" "default" {
  name      = "SRE Primary On-Call"
  num_loops = 2
  teams     = [pagerduty_team.sre.id]

  rule {
    escalation_delay_in_minutes = 10
    target {
      type = "schedule_reference"
      id   = pagerduty_schedule.primary.id
    }
  }

  rule {
    escalation_delay_in_minutes = 15
    target {
      type = "user_reference"
      id   = pagerduty_user.secondary.id
    }
  }
}
