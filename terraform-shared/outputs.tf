# ===========================================================================
# outputs.tf — what admins need after `terraform apply`.
# ---------------------------------------------------------------------------
# Key-bearing outputs are sensitive; print explicitly:
#   terraform output -json admin_site_urls | jq
#   terraform output -json admin_routing_keys | jq
# ===========================================================================

output "admin_routing_keys" {
  description = "Map of admin initials → their event-orchestration routing key. Works for both the storefront and `greenagonia scenarios run --routing-key`."
  value = {
    for k, o in pagerduty_event_orchestration.admin :
    k => o.integration[0].parameters[0].routing_key
  }
  sensitive = true
}

output "admin_site_urls" {
  description = "Map of admin initials → ready-to-share storefront URL with their routing key pre-loaded."
  value = {
    for k, o in pagerduty_event_orchestration.admin :
    k => "${var.site_url}/?pdkey=${o.integration[0].parameters[0].routing_key}"
  }
  sensitive = true
}

output "admin_teams" {
  description = "Map of admin initials → team ID."
  value       = { for k, t in pagerduty_team.admin : k => t.id }
}

output "technical_services" {
  description = "Map of \"INITIALS/service\" → PagerDuty service ID."
  value       = { for k, s in pagerduty_service.tech : k => s.id }
}

output "change_event_keys" {
  description = "Per-service integration keys for posting change events (GitHub Actions). Map of \"INITIALS/service\" → key."
  value       = { for k, i in pagerduty_service_integration.events_v2 : k => i.integration_key }
  sensitive   = true
}

output "change_event_keys_ld" {
  description = "LaunchDarkly change-event keys for payment-gateway. Map of \"INITIALS/payment-gateway\" → key."
  value       = { for k, i in pagerduty_service_integration.events_v2_ld : k => i.integration_key }
  sensitive   = true
}

output "personas" {
  description = "Map of persona slug → PagerDuty user ID (shared across all teams)."
  value       = { for k, u in pagerduty_user.persona : k => u.id }
}

output "admin_full_urls" {
  description = "Storefront URL with routing key + both change event keys pre-loaded. Share this."
  value = {
    for k, o in pagerduty_event_orchestration.admin :
    k => join("", [
      "${var.site_url}/?pdkey=${o.integration[0].parameters[0].routing_key}",
      "&pdchangekey=${pagerduty_service_integration.events_v2["${k}/payment-gateway"].integration_key}",
      "&pdldkey=${pagerduty_service_integration.events_v2_ld["${k}/payment-gateway"].integration_key}",
    ])
  }
  sensitive = true
}

output "platform_routing_key" {
  description = "Routing key for the shared Greenagonia Platform Router orchestration."
  value       = pagerduty_event_orchestration.greenagonia.integration[0].parameters[0].routing_key
  sensitive   = true
}

output "platform_services" {
  description = "Map of platform service name → PagerDuty service ID."
  value       = { for k, s in pagerduty_service.platform : k => s.id }
}
