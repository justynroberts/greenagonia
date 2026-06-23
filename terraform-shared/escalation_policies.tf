# ===========================================================================
# escalation_policies.tf — every pagerduty_escalation_policy.
# ---------------------------------------------------------------------------
#   admin     per admin: primary schedule → secondary → tertiary, 2 loops
#   catchall  shared, for the unrouted-events service (pages Sarah SRE,
#             i.e. nobody real — check it occasionally)
# ===========================================================================

resource "pagerduty_escalation_policy" "admin" {
  for_each = var.admins

  name      = "${each.key} SRE On-Call"
  num_loops = 2
  teams     = [pagerduty_team.admin[each.key].id]

  rule {
    escalation_delay_in_minutes = 10
    target {
      type = "schedule_reference"
      id   = pagerduty_schedule.primary[each.key].id
    }
  }

  rule {
    escalation_delay_in_minutes = 15
    target {
      type = "schedule_reference"
      id   = pagerduty_schedule.secondary[each.key].id
    }
  }

  rule {
    escalation_delay_in_minutes = 15
    target {
      type = "schedule_reference"
      id   = pagerduty_schedule.tertiary[each.key].id
    }
  }
}

resource "pagerduty_escalation_policy" "catchall" {
  name      = "Unrouted Events (shared)"
  num_loops = 1

  rule {
    escalation_delay_in_minutes = 30
    target {
      type = "user_reference"
      id   = pagerduty_user.persona["sarah-sre"].id
    }
  }
}
