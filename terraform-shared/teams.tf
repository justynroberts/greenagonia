# ===========================================================================
# teams.tf — teams and team memberships.
# ---------------------------------------------------------------------------
# One team per admin, named from their initials (JR → JR-SRE-TEAM).
# Roster: the admin (manager) + all 5 personas (responders), so every team
# looks fully staffed and personas are schedulable on every rotation.
# ===========================================================================

resource "pagerduty_team" "admin" {
  for_each = var.admins

  name         = "${each.key}-SRE-TEAM"
  description  = "Greenagonia SRE team for ${each.value.name}. Owns the ${each.key}-* service set."
  default_role = "none"
}

resource "pagerduty_team_membership" "admin" {
  for_each = var.admins

  user_id = local.all_admin_ids[each.key]
  team_id = pagerduty_team.admin[each.key].id
  role    = "manager"
}

# persona × team — every persona joins every team.
resource "pagerduty_team_membership" "persona" {
  for_each = {
    for pair in setproduct(keys(var.admins), keys(local.personas)) :
    "${pair[0]}/${pair[1]}" => { admin = pair[0], persona = pair[1] }
  }

  user_id = pagerduty_user.persona[each.value.persona].id
  team_id = pagerduty_team.admin[each.value.admin].id
  role    = "responder"
}
