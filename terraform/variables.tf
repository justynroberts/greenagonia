# ===========================================================================
# variables.tf — inputs to the Greenagonia PagerDuty stack.
# ---------------------------------------------------------------------------
# These four variables fully parameterise an environment. The CLI supplies
# all of them via -var flags; you can also drop a terraform.tfvars file in
# this directory (see terraform.tfvars.example).
# ===========================================================================

variable "pagerduty_token" {
  type        = string
  description = "PagerDuty REST API token with admin/write scope. Required for the provider."
  sensitive   = true
}

variable "pagerduty_region" {
  type        = string
  description = <<-EOT
    PagerDuty service region:
      "us" — default; uses api.pagerduty.com + events.pagerduty.com
      "eu" — uses api.eu.pagerduty.com + events.eu.pagerduty.com

    Must match the region of the account the token belongs to; the wrong
    region returns 401/Unauthorized at apply time. The CLI exposes this
    as --region.
  EOT
  default     = "us"
  validation {
    condition     = contains(["us", "eu"], var.pagerduty_region)
    error_message = "pagerduty_region must be \"us\" or \"eu\"."
  }
}

variable "environment" {
  type        = string
  description = <<-EOT
    Environment slug used to namespace every resource (e.g. "demo", "qa", "alex").
    The CLI also uses this as the Terraform workspace name, so multiple envs can
    coexist against the same PagerDuty account without resource-name collisions.
  EOT
  default     = "demo"
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{0,30}$", var.environment))
    error_message = "environment must be lowercase alphanumerics + hyphens (max 31 chars)."
  }
}

variable "user_email" {
  type        = string
  description = "Email of the primary on-call user. Must be unique within the PagerDuty account."
  validation {
    condition     = can(regex("^[^@]+@[^@]+\\.[^@]+$", var.user_email))
    error_message = "user_email must look like an email address."
  }
}

variable "user_name" {
  type        = string
  description = "Display name for the primary on-call user."
  default     = "Greenagonia Oncall"
}

variable "secondary_user_email" {
  type        = string
  description = <<-EOT
    Secondary on-call user's email. The CLI auto-derives this by adding
    "+oncall2" before the @ of --email (e.g. you@x.com → you+oncall2@x.com),
    so notifications still hit the same inbox. Set explicitly to override.
  EOT
}

variable "tertiary_user_email" {
  type        = string
  description = <<-EOT
    Tertiary on-call user's email. Default derivation: "+oncall3" before @.
    Same inbox routing as secondary — created as a distinct PD user so the
    weekly rotation has three names to cycle through.
  EOT
}

variable "enable_incident_workflows" {
  type        = bool
  default     = false
  description = <<-EOT
    Whether to create the three Incident Workflows (sev1-response, auto-rollback,
    clear-down). Incident Workflows are a paid feature; on plans without the
    entitlement the PagerDuty API returns 404 and the deploy fails.

    Set to true once you've confirmed your account includes Incident Workflows
    (Business / Digital Operations plans). When false, the Automation Actions in
    automation.tf are still created and still appear on every incident — responders
    trigger them directly from the incident's Actions menu instead of via a
    workflow wrapper.

    The CLI exposes this as `--with-workflows`.
  EOT
}
