# Look up an existing announcement rule by name, such as one created in the dashboard.
data "incident_announcement_rule" "critical" {
  name = "Critical incidents"
}

# Or by ID, to reference a rule another module manages.
data "incident_announcement_rule" "payments" {
  id = "01G0J1EXE7AXZ2C93K61WBPYEH"
}

output "critical_slack_channel_ids" {
  value = data.incident_announcement_rule.critical.slack_channel_ids
}
