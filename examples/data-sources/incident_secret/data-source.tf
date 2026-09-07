# Look up an existing secret by name, such as one created in the dashboard.
data "incident_secret" "pagerduty_webhook_token" {
  name = "PagerDuty webhook token"
}

# Or by ID, to reference a secret another module manages.
data "incident_secret" "billing_api_key" {
  id = "01G0J1EXE7AXZ2C93K61WBPYEH"
}

# A secret's value is never returned by the API, so a lookup can only tell you about it:
# which version is current, and the last four characters of that version's value.
output "pagerduty_token_version" {
  value = data.incident_secret.pagerduty_webhook_token.version
}
