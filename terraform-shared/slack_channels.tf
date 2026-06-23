# ===========================================================================
# slack_channels.tf — per-admin incident channels in Slack.
# ---------------------------------------------------------------------------
# Gated behind var.enable_slack. Requires:
#   slack_bot_token set in secrets.auto.tfvars.json   (xoxb-… bot token)
#   Bot scopes: channels:manage, users:read, users:read.email
#
# Creates {ini}-incidents (jr-incidents, jp-incidents …) as public channels
# and adds the admin user as a permanent member.
# ===========================================================================

locals {
  # Admins who have a Slack account in this workspace (skip_slack = false)
  slack_admins = var.enable_slack ? {
    for k, v in var.admins : k => v if !v.skip_slack
  } : {}
}

data "slack_user" "admin" {
  for_each = local.slack_admins
  email    = each.value.slack_email != "" ? each.value.slack_email : each.value.email
}

resource "slack_conversation" "incidents" {
  for_each = var.enable_slack ? var.admins : {}

  name                                 = "${lower(each.key)}-incidents"
  is_private                           = false
  action_on_update_permanent_members   = "none"
  permanent_members                    = contains(keys(local.slack_admins), each.key) ? [data.slack_user.admin[each.key].id] : []
}

# One Slack connection per admin team → their {ini}-incidents channel.
resource "pagerduty_slack_connection" "incidents" {
  for_each = var.enable_slack ? var.admins : {}

  source_id         = pagerduty_team.admin[each.key].id
  source_type       = "team_reference"
  workspace_id      = var.slack_workspace_id
  channel_id        = slack_conversation.incidents[each.key].id
  notification_type = "responder"

  config {
    events = [
      "incident.triggered",
      "incident.acknowledged",
      "incident.escalated",
      "incident.resolved",
      "incident.reassigned",
      "incident.annotated",
      "incident.unacknowledged",
      "incident.delegated",
      "incident.priority_updated",
      "incident.responder.added",
      "incident.responder.replied",
      "incident.status_update_published",
      "incident.reopened",
    ]
    priorities = ["*"]
  }
}

# Shared Greenagonia team → greenagonia-incidents channel.
resource "slack_conversation" "greenagonia_incidents" {
  count = var.enable_slack ? 1 : 0

  name                                 = "greenagonia-incidents"
  is_private                           = false
  action_on_update_permanent_members   = "none"
  permanent_members                    = [for k in keys(local.slack_admins) : data.slack_user.admin[k].id]
}

resource "pagerduty_slack_connection" "greenagonia_incidents" {
  count = var.enable_slack ? 1 : 0

  source_id         = pagerduty_team.greenagonia.id
  source_type       = "team_reference"
  workspace_id      = var.slack_workspace_id
  channel_id        = slack_conversation.greenagonia_incidents[0].id
  notification_type = "responder"

  config {
    events = [
      "incident.triggered",
      "incident.acknowledged",
      "incident.escalated",
      "incident.resolved",
      "incident.reassigned",
      "incident.annotated",
      "incident.unacknowledged",
      "incident.delegated",
      "incident.priority_updated",
      "incident.responder.added",
      "incident.responder.replied",
      "incident.status_update_published",
      "incident.reopened",
    ]
    priorities = ["*"]
  }
}
