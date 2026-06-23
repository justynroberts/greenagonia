# ===========================================================================
# automation.tf — SHARED automation actions, bound to every technical
# service of every admin.
# ---------------------------------------------------------------------------
# Four script-type actions (the single-user trio + a security forensics
# action backing the Security workflow). Gated behind
# var.enable_automation_actions (requires Process Automation entitlement;
# returns 402 without it).
# ===========================================================================

resource "pagerduty_automation_actions_action" "diagnostics" {
  count = var.enable_automation_actions ? 1 : 0

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
  count = var.enable_automation_actions ? 1 : 0

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
  count = var.enable_automation_actions ? 1 : 0

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

resource "pagerduty_automation_actions_action" "forensics" {
  count = var.enable_automation_actions ? 1 : 0

  name        = "Isolate & Capture Forensics"
  description = "Quarantine the affected workload, snapshot disks, and capture network flows for the security team."
  action_type = "script"

  action_data_reference {
    invocation_command = "/bin/bash"
    script             = <<-EOT
      #!/usr/bin/env bash
      set -euo pipefail
      svc="$${PD_INCIDENT_TITLE:-unknown}"
      echo "[forensics] isolating $$svc"
      kubectl label pods -l "app=$$svc" quarantine=true --overwrite
      kubectl annotate networkpolicy "$$svc-default" lockdown=true --overwrite || true
      echo "[forensics] snapshotting volumes"; velero backup create "forensics-$$svc-$$(date +%s)" --include-resources pvc -l "app=$$svc"
      echo "[forensics] capturing flows"; tcpdump -i any -w "/forensics/$$svc.pcap" -G 300 -W 1 &
    EOT
  }
}

# ---- Bind every action to every admin's technical services -----------------

resource "pagerduty_automation_actions_action_service_association" "diagnostics" {
  for_each   = var.enable_automation_actions ? local.admin_tech : {}
  action_id  = pagerduty_automation_actions_action.diagnostics[0].id
  service_id = pagerduty_service.tech[each.key].id
}

resource "pagerduty_automation_actions_action_service_association" "rollback" {
  for_each   = var.enable_automation_actions ? local.admin_tech : {}
  action_id  = pagerduty_automation_actions_action.rollback[0].id
  service_id = pagerduty_service.tech[each.key].id
}

resource "pagerduty_automation_actions_action_service_association" "cleardown" {
  for_each   = var.enable_automation_actions ? local.admin_tech : {}
  action_id  = pagerduty_automation_actions_action.cleardown[0].id
  service_id = pagerduty_service.tech[each.key].id
}

resource "pagerduty_automation_actions_action_service_association" "forensics" {
  for_each   = var.enable_automation_actions ? local.admin_tech : {}
  action_id  = pagerduty_automation_actions_action.forensics[0].id
  service_id = pagerduty_service.tech[each.key].id
}
