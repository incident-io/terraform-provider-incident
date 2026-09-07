# A secret's value is write-only: it's sent to incident.io and never written to state or
# to a plan file. Setting it needs Terraform 1.11 or OpenTofu 1.11 and above.
#
# Terraform can't see the value change, so rotating a secret means changing
# value_wo_version too - conventionally by incrementing it. That's the change Terraform
# can see, and it's what asks incident.io to rotate.
variable "pagerduty_webhook_token" {
  type      = string
  ephemeral = true
}

resource "incident_secret" "pagerduty_webhook_token" {
  name        = "PagerDuty webhook token"
  description = "Auth token for the PagerDuty outgoing webhook"

  value_wo         = var.pagerduty_webhook_token
  value_wo_version = 1
}

# Secrets can be owned by teams, which is what governs who can rotate or delete them.
# An owning team is a catalog entry of your Team catalog type.
variable "billing_api_key" {
  type      = string
  ephemeral = true
}

resource "incident_secret" "billing_api_key" {
  name = "Billing API key"

  value_wo         = var.billing_api_key
  value_wo_version = 1

  owning_team_ids = ["01G0J1EXE7AXZ2C93K61WBPYEH"]
}

# Terraform doesn't have to own the value. Leave both value attributes off an existing
# secret and Terraform manages its name, description and owning teams while something
# else - the dashboard, or a rotation job - looks after the value.
resource "incident_secret" "rotated_elsewhere" {
  name        = "Datadog API key"
  description = "Rotated nightly by our key-rotation job"
}
