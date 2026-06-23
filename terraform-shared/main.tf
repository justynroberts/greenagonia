# ===========================================================================
# main.tf — provider, version pins, and the shared catalogue.
# ---------------------------------------------------------------------------
# SHARED (multi-admin) Greenagonia environment. The multi-user counterpart
# of ../single-user/terraform.
#
# Model:
#   - 5 GENERIC PERSONA USERS (Sarah SRE, Dan Developer, Matt Manager,
#     Pablo Platform, Sam Security) — created once, members of every team,
#     present in every schedule rotation.
#   - PER ADMIN (var.admins, keyed by initials e.g. "JR"):
#       1 user · 1 team (JR-SRE-TEAM) · 10 services (the single-user
#       catalogue: 8 technical + 2 business) · 3 schedules · 1 escalation
#       policy · 1 event orchestration with its own routing key.
#   - SHARED: 3 automation actions bound to every technical service,
#     3 incident workflows (opt-in), 1 catch-all service.
#
# One file per RESOURCE TYPE — to change something, open the file named
# after the thing you're changing:
#   users.tf                 pagerduty_user            (personas + admins)
#   teams.tf                 pagerduty_team + memberships
#   schedules.tf             pagerduty_schedule        (primary/secondary/tertiary)
#   escalation_policies.tf   pagerduty_escalation_policy
#   services.tf              pagerduty_service         (technical + catch-all)
#   business_services.tf     pagerduty_business_service + dependencies
#   orchestration.tf         pagerduty_event_orchestration + routers
#   automation.tf            pagerduty_automation_actions_* (actions + bindings)
#   workflows.tf             pagerduty_incident_workflow + triggers (incl. Slack)
#   variables.tf / outputs.tf / main.tf (provider + the catalogue locals)
#
# STATE: local, in this directory (terraform.tfstate, gitignored). One
# person applies at a time. If multiple admins ever run applies, migrate to
# a remote backend (HCP Terraform free tier gives state + locking) — see
# README.md.
#
# Safety: every per-admin resource is for_each-keyed on the admin's
# initials. Adding/removing an admin is one tfvars entry; plans only touch
# that admin's resources, never a neighbour's.
# ===========================================================================

terraform {
  required_version = ">= 1.5"
  required_providers {
    pagerduty = {
      source  = "PagerDuty/pagerduty"
      version = ">= 3.0"
    }
    slack = {
      source  = "pablovarela/slack"
      version = "~> 1.0"
    }
  }
}

provider "pagerduty" {
  token          = var.pagerduty_token
  user_token     = var.pagerduty_user_token
  service_region = var.pagerduty_region
}

# Token defaults to "" — provider only makes API calls when enable_slack = true
# and slack resources exist. Set slack_bot_token in secrets.auto.tfvars.json.
provider "slack" {
  token = var.slack_bot_token
}

locals {
  # ---- The 5 generic personas ---------------------------------------------
  # Fictional emails on greenagonia.io — set dressing; pages to them go
  # nowhere, which is the point. Order matters: it's the rotation order.
  personas = {
    sarah-sre = {
      name  = "Sarah SRE"
      email = "sarah.sre@greenagonia.io"
    }
    dan-developer = {
      name  = "Dan Developer"
      email = "dan.developer@greenagonia.io"
    }
    matt-manager = {
      name  = "Matt Manager"
      email = "matt.manager@greenagonia.io"
    }
    pablo-platform = {
      name  = "Pablo Platform"
      email = "pablo.platform@greenagonia.io"
    }
    sam-security = {
      name  = "Sam Security"
      email = "sam.security@greenagonia.io"
    }
  }
  persona_order = keys(local.personas) # deterministic rotation order

  # ---- The service catalogue — identical to single-user -------------------
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

  # ---- Cross products for for_each ----------------------------------------
  # admin × technical service → "JR/payment-gateway"
  admin_tech = {
    for pair in setproduct(keys(var.admins), keys(local.technical_services)) :
    "${pair[0]}/${pair[1]}" => { admin = pair[0], service = pair[1] }
  }

  # admin × business service
  admin_biz = {
    for pair in setproduct(keys(var.admins), keys(local.business_services)) :
    "${pair[0]}/${pair[1]}" => { admin = pair[0], service = pair[1] }
  }

}

# admin × (business → tech) dependency edges need a nested flatten — kept
# in a separate locals block for readability.
locals {
  admin_dependency_edges = {
    for edge in flatten([
      for admin in keys(var.admins) : [
        for biz_name, biz in local.business_services : [
          for tech_name in biz.supports : {
            key     = "${admin}/${biz_name}/${tech_name}"
            admin   = admin
            biz     = biz_name
            tech    = tech_name
          }
        ]
      ]
    ]) : edge.key => edge
  }
}
