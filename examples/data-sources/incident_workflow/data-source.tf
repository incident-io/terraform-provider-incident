# Reference an existing workflow by its ID, to read back its full configuration.
data "incident_workflow" "autoassign_incident_lead" {
  id = "01HY0QGB4M1XKGQGKQD0N9MHKG"
}

# The full definition is available, so you can (for example) reuse the steps of a
# workflow that was built in the dashboard.
output "autoassign_incident_lead_steps" {
  value = data.incident_workflow.autoassign_incident_lead.steps
}
