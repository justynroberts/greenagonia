# ===========================================================================
# outputs.tf — what the CLI reads after `terraform apply`.
# ---------------------------------------------------------------------------
# `routing_key` is the only one strictly needed by the CLI; the others are
# convenient for humans poking at the env.
# ===========================================================================

output "environment" {
  description = "Environment slug — also the Terraform workspace name."
  value       = var.environment
}

output "routing_key" {
  description = "Inbound integration key for the dynamic event orchestration. Feed this to `greenagonia scenarios run --routing-key`."
  value       = pagerduty_event_orchestration.main.integration[0].parameters[0].routing_key
  sensitive   = true
}

output "technical_services" {
  description = "Map of technical-service name → PagerDuty service ID."
  value       = { for k, s in pagerduty_service.tech : k => s.id }
}

output "change_event_keys" {
  description = "Per-service integration keys. Use as `routing_key` when POSTing to https://events.pagerduty.com/v2/change/enqueue."
  value       = { for k, i in pagerduty_service_integration.events_v2 : k => i.integration_key }
  sensitive   = true
}

output "business_services" {
  description = "Map of business-service name → PagerDuty business-service ID."
  value       = { for k, s in pagerduty_business_service.biz : k => s.id }
}

output "primary_user_id" {
  description = "PagerDuty user ID of the primary on-call (the email you supplied)."
  value       = pagerduty_user.primary.id
}
