# ===========================================================================
# workflows.tf — three responder-triggered incident workflows.
# ---------------------------------------------------------------------------
# Gated behind var.enable_incident_workflows (default false). Incident
# Workflows are a paid PagerDuty feature; on accounts without the entitlement
# POST /incident_workflows returns 404. When the gate is off, the Automation
# Actions in automation.tf still appear on every incident — responders just
# trigger them from the incident Actions menu instead of via a workflow.
# ===========================================================================

resource "pagerduty_incident_workflow" "sev1" {
  count = var.enable_incident_workflows ? 1 : 0

  name        = "SEV-1 Response"
  description = "SEV-1 critical response: post a status update, then gather diagnostics."

  step {
    name   = "Post initial status update"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "SEV-1 declared. SRE engaged. Diagnostics in progress."
    }
  }

  step {
    name   = "Run diagnostics"
    action = "pagerduty.com:automation-actions:run-action:1"
    input {
      name  = "Action ID"
      value = pagerduty_automation_actions_action.diagnostics.id
    }
  }
}

resource "pagerduty_incident_workflow" "rollback" {
  count = var.enable_incident_workflows ? 1 : 0

  name        = "Auto-Rollback"
  description = "Roll the failing service back to the last known-good build, then update status."

  step {
    name   = "Run rollback"
    action = "pagerduty.com:automation-actions:run-action:1"
    input {
      name  = "Action ID"
      value = pagerduty_automation_actions_action.rollback.id
    }
  }

  step {
    name   = "Post rollback status update"
    action = "pagerduty.com:incident-workflows:send-status-update:5"
    input {
      name  = "Message"
      value = "Rollback initiated. Watching error rate for recovery."
    }
  }
}

resource "pagerduty_incident_workflow" "cleardown" {
  count = var.enable_incident_workflows ? 1 : 0

  name        = "Clear Down"
  description = "Reset feature flags and clear cache on the affected service."

  step {
    name   = "Clear down settings"
    action = "pagerduty.com:automation-actions:run-action:1"
    input {
      name  = "Action ID"
      value = pagerduty_automation_actions_action.cleardown.id
    }
  }
}

# ---- Manual triggers — show up on every incident --------------------------

resource "pagerduty_incident_workflow_trigger" "sev1" {
  count = var.enable_incident_workflows ? 1 : 0

  type                       = "manual"
  workflow                   = pagerduty_incident_workflow.sev1[0].id
  subscribed_to_all_services = true
}

resource "pagerduty_incident_workflow_trigger" "rollback" {
  count = var.enable_incident_workflows ? 1 : 0

  type                       = "manual"
  workflow                   = pagerduty_incident_workflow.rollback[0].id
  subscribed_to_all_services = true
}

resource "pagerduty_incident_workflow_trigger" "cleardown" {
  count = var.enable_incident_workflows ? 1 : 0

  type                       = "manual"
  workflow                   = pagerduty_incident_workflow.cleardown[0].id
  subscribed_to_all_services = true
}
