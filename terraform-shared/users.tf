# ===========================================================================
# users.tf — every pagerduty_user in the stack.
# ---------------------------------------------------------------------------
#   persona  the 5 shared fictional responders — pages to them never deliver;
#            they exist to staff teams and schedules.
#            Default emails: @greenagonia.io (fine on most accounts).
#            Restricted accounts (e.g. @pagerduty.com only): set
#            var.persona_email_base to a real address you control, e.g.
#            "you@pagerduty.com"; personas then get plus-addressed emails
#            (you+sarah-sre@pagerduty.com, you+dan-developer@pagerduty.com …).
#   admin    Two modes, controlled per-admin by the `exists` flag:
#              exists = true  — user already has a PD account → data lookup
#              exists = false — new user → resource creation (default)
# ===========================================================================

locals {
  persona_emails = {
    for slug, p in local.personas :
    slug => (
      var.persona_email_base == ""
      ? p.email
      : replace(var.persona_email_base, "@", "+${slug}@")
    )
  }

  existing_admins = { for k, v in var.admins : k => v if v.exists }
  new_admins      = { for k, v in var.admins : k => v if !v.exists }

  # Unified map used by every resource that references an admin user ID.
  all_admin_ids = merge(
    { for k, u in data.pagerduty_user.existing_admin : k => u.id },
    { for k, u in pagerduty_user.new_admin : k => u.id },
  )
}

resource "pagerduty_user" "persona" {
  for_each = local.personas

  name  = each.value.name
  email = local.persona_emails[each.key]
  role  = "user"
}

# Admins that already have a PD account — look them up, don't touch them.
data "pagerduty_user" "existing_admin" {
  for_each = local.existing_admins
  email    = each.value.email
}

# Admins that don't have a PD account yet — create them.
resource "pagerduty_user" "new_admin" {
  for_each = local.new_admins

  name  = each.value.name
  email = each.value.email
  role  = "user"
}
