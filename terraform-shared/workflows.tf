# ===========================================================================
# workflows.tf — every pagerduty_incident_workflow + trigger.
# ---------------------------------------------------------------------------
# SHARED, gated behind var.enable_incident_workflows (paid feature):
#   1. Major Incident Escalation — declare, diagnostics, confirm. (manual)
#   2. Security Incident Response — restrict comms, isolate + forensics. (manual)
#   3. Post to Status Page — customer-facing update. (manual)
#
# PER ADMIN, gated behind var.enable_slack (also needs the workflows
# entitlement + Slack V2 integration):
#   4. <INITIALS> — Open Incident Channel — creates a dedicated Slack
#      channel named after their team; fires automatically on high-urgency
#      incidents on their services.
#
# Steps use the universally-available actions (send-status-update,
# run automation action) plus the Slack create-channel action, whose ID is
# account-specific — see var.slack_create_channel_action_id.
# ===========================================================================

# ---- 1. Major Incident Escalation ------------------------------------------

resource "pagerduty_incident_workflow" "major_incident" {
  count = var.enable_incident_workflows ? 1 : 0

  name        = "Major Incident Escalation"
  description = "Declare a major incident: notify stakeholders, open the bridge, start diagnostics."

  step {
    name   = "Declare major incident"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "MAJOR INCIDENT declared. Incident commander engaged, leadership notified. Bridge: meet.greenagonia.io/major"
    }
  }

  dynamic "step" {
    for_each = var.enable_automation_actions ? [1] : []
    content {
      name   = "Run diagnostics"
      action = "pagerduty.com:automation-actions:run-action:1"
      input {
        name  = "Action ID"
        value = pagerduty_automation_actions_action.diagnostics[0].id
      }
    }
  }

  step {
    name   = "Confirm escalation"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "Escalation complete: secondary and tertiary rotations alerted, diagnostics collecting. Next update in 15 minutes."
    }
  }
}

resource "pagerduty_incident_workflow_trigger" "major_incident" {
  count = var.enable_incident_workflows ? 1 : 0

  type                       = "manual"
  workflow                   = pagerduty_incident_workflow.major_incident[0].id
  subscribed_to_all_services = true
}

# ---- 2. Security Incident Response -----------------------------------------

resource "pagerduty_incident_workflow" "security" {
  count = var.enable_incident_workflows ? 1 : 0

  name        = "Security Incident Response"
  description = "Engage the security response: restrict comms, isolate the workload, capture forensics."

  step {
    name   = "Engage security response"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "Security response engaged. Communications restricted to the security channel. Do not discuss outside #sec-incident."
    }
  }

  dynamic "step" {
    for_each = var.enable_automation_actions ? [1] : []
    content {
      name   = "Isolate and capture forensics"
      action = "pagerduty.com:automation-actions:run-action:1"
      input {
        name  = "Action ID"
        value = pagerduty_automation_actions_action.forensics[0].id
      }
    }
  }
}

resource "pagerduty_incident_workflow_trigger" "security" {
  count = var.enable_incident_workflows ? 1 : 0

  type                       = "manual"
  workflow                   = pagerduty_incident_workflow.security[0].id
  subscribed_to_all_services = true
}

# ---- 3. Post to Status Page -------------------------------------------------
# Uses a status update (incident subscribers see it). If the account has
# native PagerDuty Status Pages, swap in the native post action from the
# account's action catalogue.

resource "pagerduty_incident_workflow" "status_page" {
  count = var.enable_incident_workflows ? 1 : 0

  name        = "Post to Status Page"
  description = "Publish a customer-facing update for this incident."

  step {
    name   = "Publish status update"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "We are investigating an issue affecting checkout on greenagonia.io. Customers may see payment errors. Next update within 30 minutes — status.greenagonia.io"
    }
  }
}

resource "pagerduty_incident_workflow_trigger" "status_page" {
  count = var.enable_incident_workflows ? 1 : 0

  type                       = "manual"
  workflow                   = pagerduty_incident_workflow.status_page[0].id
  subscribed_to_all_services = true
}

# ---- 4. Rollback Deployment to Previous ------------------------------------

resource "pagerduty_incident_workflow" "rollback" {
  count = var.enable_incident_workflows ? 1 : 0

  name        = "Rollback Deployment to Previous"
  description = "Initiate a rollback to the last known-good deployment and notify the team."

  step {
    name   = "Notify team of rollback"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "ROLLBACK initiated. Reverting to last known-good deployment. All deploys frozen until incident is resolved. Estimated completion: 5-10 minutes."
    }
  }

  dynamic "step" {
    for_each = var.enable_automation_actions ? [1] : []
    content {
      name   = "Trigger rollback automation"
      action = "pagerduty.com:automation-actions:run-action:1"
      input {
        name  = "Action ID"
        value = pagerduty_automation_actions_action.diagnostics[0].id
      }
    }
  }

  step {
    name   = "Confirm rollback complete"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "ROLLBACK complete. Previous version is live. Monitoring for stability — do not deploy until this incident is resolved."
    }
  }
}

resource "pagerduty_incident_workflow_trigger" "rollback" {
  count = var.enable_incident_workflows ? 1 : 0

  type                       = "manual"
  workflow                   = pagerduty_incident_workflow.rollback[0].id
  subscribed_to_all_services = true
}

# ---- 5. Per-admin Slack incident channel ------------------------------------

resource "pagerduty_incident_workflow" "slack_channel" {
  for_each = var.enable_slack && var.enable_incident_workflows ? var.admins : {}

  name        = "${each.key} — Open Incident Channel"
  description = "Creates a dedicated Slack channel named after ${each.key}-SRE-TEAM for the incident."
  team        = pagerduty_team.admin[each.key].id

  step {
    name   = "Create dedicated Slack channel"
    action = var.slack_create_channel_action_id

    input {
      name  = "Workspace"
      value = var.slack_workspace_id
    }
    # PagerDuty lowercases channel names automatically; the incident id
    # keeps each channel unique.
    input {
      name  = "Channel Name"
      value = "${lower(each.key)}-sre-team-{{incident.id}}"
    }
    input {
      name  = "Channel Visibility"
      value = "Public"
    }
  }
}

# Fires automatically on high-urgency incidents on this admin's services —
# the storefront's payment failures are severity critical → urgency high.
resource "pagerduty_incident_workflow_trigger" "slack_channel" {
  for_each = var.enable_slack && var.enable_incident_workflows ? var.admins : {}

  type                       = "conditional"
  workflow                   = pagerduty_incident_workflow.slack_channel[each.key].id
  condition                  = "incident.urgency matches 'high'"
  subscribed_to_all_services = false
  services = [
    for svc in keys(local.technical_services) :
    pagerduty_service.tech["${each.key}/${svc}"].id
  ]
}
