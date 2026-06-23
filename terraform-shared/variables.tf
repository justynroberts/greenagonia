# ===========================================================================
# variables.tf — inputs to the shared Greenagonia stack.
# ===========================================================================

variable "pagerduty_token" {
  type        = string
  description = "PagerDuty REST API token with admin/write scope. Required for the provider."
  sensitive   = true
}

variable "pagerduty_user_token" {
  type        = string
  default     = ""
  sensitive   = true
  description = "PagerDuty user-level REST API token. Required for pagerduty_slack_connection. Generate at: My Profile → User Settings → Create API User Token."
}

variable "pagerduty_region" {
  type        = string
  description = <<-EOT
    PagerDuty service region:
      "us" — default; uses api.pagerduty.com + events.pagerduty.com
      "eu" — uses api.eu.pagerduty.com + events.eu.pagerduty.com

    Must match the region of the account the token belongs to; the wrong
    region returns 401/Unauthorized at apply time.
  EOT
  default     = "us"
  validation {
    condition     = contains(["us", "eu"], var.pagerduty_region)
    error_message = "pagerduty_region must be \"us\" or \"eu\"."
  }
}

variable "admins" {
  type = map(object({
    name        = string
    email       = string
    exists      = optional(bool, false)
    slack_email = optional(string, "")
    skip_slack  = optional(bool, false)
  }))
  description = <<-EOT
    One entry per admin, keyed by their initials (used in every resource
    name: team JR-SRE-TEAM, services JR-payment-gateway, …):

      admins = {
        JR = { name = "Justyn Roberts", email = "justyn@example.com", exists = true  }
        AB = { name = "Alice Bell",     email = "alice@example.com"  }
        NT = { name = "Nick T",         email = "nt@pd.com", slack_email = "nick@personal.com" }
      }

    exists      = true  — user already has a PD account; looked up by email.
    exists      = false — user will be created as a new PD user (default).
    slack_email = override the email used to look up this admin in Slack (when
                  their Slack email differs from their PagerDuty email).
    skip_slack  = true — exclude this admin from Slack channel creation entirely
                  (use when they don't have an account in the workspace yet).
  EOT
  validation {
    condition     = length(var.admins) > 0
    error_message = "admins must contain at least one entry."
  }
  validation {
    condition = alltrue([
      for initials, a in var.admins :
      can(regex("^[A-Z]{2,4}$", initials)) && can(regex("^[^@]+@[^@]+\\.[^@]+$", a.email))
    ])
    error_message = "Keys must be 2-4 uppercase initials (e.g. \"JR\", \"JRO\"); emails must look like email addresses."
  }
}

variable "slack_bot_token" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Slack bot token (xoxb-…). Required when enable_slack = true. Set in secrets.auto.tfvars.json. Bot scopes needed: channels:manage, users:read, users:read.email."
}

variable "enable_slack" {
  type        = bool
  default     = false
  description = <<-EOT
    Create a per-admin Incident Workflow that opens a dedicated Slack channel
    (named after their team) for every high-urgency incident on their
    services. Requires: the PagerDuty Slack V2 integration installed and
    connected to the workspace, Incident Workflows entitlement, and
    slack_workspace_id set.
  EOT
}

variable "slack_workspace_id" {
  type        = string
  default     = ""
  description = "Slack workspace (team) ID the PagerDuty Slack integration is connected to, e.g. \"T01ABCDEF23\". Required when enable_slack = true."
  validation {
    condition     = var.slack_workspace_id == "" || can(regex("^T[A-Z0-9]+$", var.slack_workspace_id))
    error_message = "slack_workspace_id should look like a Slack team ID (starts with T)."
  }
}

variable "slack_create_channel_action_id" {
  type        = string
  default     = "pagerduty.com:incident-workflows-slack:create-incident-channel:1"
  description = <<-EOT
    Action ID (including version) of the "Create Incident Dedicated Channel"
    workflow action. Action catalogues are account-specific — confirm yours:

      curl -s -H "Authorization: Token token=$PD_TOKEN" \
        "https://api.pagerduty.com/incident_workflows/actions" \
        | jq -r '.actions[] | select(.name | test("slack"; "i")) | .id'
  EOT
}

variable "schedule_time_zone" {
  type        = string
  description = "IANA time zone for all schedules — defines what 'Mon-Fri 9-5' means for the admins' working-hours layer."
  default     = "Europe/London"
}

variable "site_url" {
  type        = string
  description = <<-EOT
    Base URL where the shared storefront (../shared-usage/shared-site) is
    hosted. Used only to build the ready-to-share per-admin links in
    outputs (site_url?pdkey=<their routing key>).
  EOT
  default     = "http://localhost:8080"
}

variable "persona_email_base" {
  type        = string
  default     = ""
  description = <<-EOT
    When empty (default), persona emails are <slug>@greenagonia.io.
    Set to a real address you control (e.g. "you@pagerduty.com") when the
    account restricts email domains — personas then get plus-addressed emails:
    you+sarah-sre@pagerduty.com, you+dan-developer@pagerduty.com, etc.
  EOT
}

variable "enable_automation_actions" {
  type        = bool
  default     = false
  description = "Create Automation Actions and bind them to every service. Requires the Process Automation entitlement; returns 402 without it."
}

variable "enable_incident_workflows" {
  type        = bool
  default     = false
  description = <<-EOT
    Whether to create the three shared Incident Workflows (SEV-1 Response,
    Auto-Rollback, Clear Down). Paid feature — on plans without the
    entitlement the API returns 404 and the deploy fails. The Automation
    Actions are created regardless and appear on every incident's Actions
    menu.
  EOT
}
