# ===========================================================================
# main.tf — provider, version pins, and the service catalogue.
# ---------------------------------------------------------------------------
# Per-resource files (terraform concatenates them all):
#
#   users.tf          user, team, team membership, escalation policy
#   services.tf       technical services, business services, dependencies,
#                     change-event integrations, catch-all service
#   orchestration.tf  event orchestration + dynamic router
#   automation.tf     3 automation actions + per-service associations
#   workflows.tf      3 incident workflows + manual triggers
#
# The contract between the CLI and PagerDuty is one variable: the
# technical-service NAME. The CLI puts it in custom_details.service of every
# event; the router (orchestration.tf) matches on it and fans out. Add a
# service to locals.technical_services below and the router/automation/etc.
# all pick it up via for_each — no other edits needed.
#
# `var.environment` is used by the CLI as the Terraform workspace name (state
# isolation) but is NOT used in any resource name — names are deliberately
# "real-company" looking. This means one PagerDuty account hosts exactly one
# deployment; for parallel environments, use separate PagerDuty accounts.
# ===========================================================================

terraform {
  required_version = ">= 1.5"
  required_providers {
    pagerduty = {
      source  = "PagerDuty/pagerduty"
      version = "~> 3.0"
    }
  }
}

provider "pagerduty" {
  token          = var.pagerduty_token
  service_region = var.pagerduty_region
}

locals {
  # ---- The service catalogue ----------------------------------------------
  # Adding a key here creates a PagerDuty service, a change-event integration,
  # a router rule, and automation-action bindings.
  technical_services = {
    payment-gateway       = "Processes card auths, captures, and refunds."
    checkout-api          = "Orchestrates the customer checkout flow."
    user-auth             = "Auth, sessions, identity."
    product-catalog       = "Product metadata, pricing, inventory."
    recommendation-engine = "Personalised product recommendations."
    search-service        = "Storefront search and faceting."
    order-service         = "Order intake and fulfilment hand-off."
    notification-service  = "Email, SMS, and push notifications."
  }

  # Business services express what customers/execs see. Each lists the
  # technical services it depends on; the dependency graph is built in
  # services.tf.
  business_services = {
    customer-checkout = {
      description = "Customers can browse, pay, and receive their order."
      supports    = ["payment-gateway", "checkout-api", "user-auth", "order-service", "notification-service"]
    }
    product-discovery = {
      description = "Customers can find and explore products."
      supports    = ["product-catalog", "search-service", "recommendation-engine"]
    }
  }

  # Flatten business→tech edges into a single map for_each can consume.
  service_dependencies = merge([
    for biz_name, biz in local.business_services : {
      for tech_name in biz.supports :
      "${biz_name}__${tech_name}" => { biz = biz_name, tech = tech_name }
    }
  ]...)
}
