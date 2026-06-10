# ===========================================================================
# automation.tf — Diagnostics, Rollback, Clear-down.
# ---------------------------------------------------------------------------
# Three script-type automation actions. In a real deployment these would hit
# a Runbook Automation runner; here they're inline bash stubs that show the
# shape of what would run. Each is associated with every technical service
# (the 3 × 8 = 24 association resources at the bottom).
# ===========================================================================

resource "pagerduty_automation_actions_action" "diagnostics" {
  name        = "Run Diagnostics"
  description = "Collect logs, last-deploy info, and current metrics for the affected service."
  action_type = "script"

  action_data_reference {
    invocation_command = "/bin/bash"
    script             = <<-EOT
      #!/usr/bin/env bash
      set -euo pipefail
      svc="$${PD_INCIDENT_TITLE:-unknown}"
      echo "[diagnostics] gathering data for $$svc"
      kubectl logs --tail=200 "deploy/$$svc" || true
      echo "[diagnostics] recent deploys:"; kubectl rollout history "deploy/$$svc" || true
      echo "[diagnostics] current SLO burn:"; promtool query instant "burn_rate{service=\"$$svc\"}"
    EOT
  }
}

resource "pagerduty_automation_actions_action" "rollback" {
  name        = "Rollback Deployment"
  description = "Roll the affected service back to the previous known-good build."
  action_type = "script"

  action_data_reference {
    invocation_command = "/bin/bash"
    script             = <<-EOT
      #!/usr/bin/env bash
      set -euo pipefail
      svc="$${PD_INCIDENT_TITLE:-unknown}"
      echo "[rollback] rolling $$svc back one revision"
      kubectl rollout undo "deploy/$$svc"
      kubectl rollout status "deploy/$$svc" --timeout=120s
    EOT
  }
}

resource "pagerduty_automation_actions_action" "cleardown" {
  name        = "Clear Down Settings"
  description = "Flush feature flags and clear cached config for the affected service."
  action_type = "script"

  action_data_reference {
    invocation_command = "/bin/bash"
    script             = <<-EOT
      #!/usr/bin/env bash
      set -euo pipefail
      svc="$${PD_INCIDENT_TITLE:-unknown}"
      echo "[cleardown] clearing flags and cache for $$svc"
      consul kv delete --recurse "config/$$svc"
      redis-cli -n 3 --scan --pattern "$$svc:*" | xargs -r redis-cli -n 3 DEL
    EOT
  }
}

# ---- Bind every action to every technical service -------------------------

resource "pagerduty_automation_actions_action_service_association" "diagnostics" {
  for_each   = local.technical_services
  action_id  = pagerduty_automation_actions_action.diagnostics.id
  service_id = pagerduty_service.tech[each.key].id
}

resource "pagerduty_automation_actions_action_service_association" "rollback" {
  for_each   = local.technical_services
  action_id  = pagerduty_automation_actions_action.rollback.id
  service_id = pagerduty_service.tech[each.key].id
}

resource "pagerduty_automation_actions_action_service_association" "cleardown" {
  for_each   = local.technical_services
  action_id  = pagerduty_automation_actions_action.cleardown.id
  service_id = pagerduty_service.tech[each.key].id
}
