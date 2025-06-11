# Get an alert source by ID
data "incident_alert_source" "example_by_id" {
  id = "01GW2G3V0S59R238FAHPDS1R66"
}

# Get an alert source by name
data "incident_alert_source" "example_by_name" {
  name = "Production Web Dashboard Alerts"
}

# Output the alert source details
output "alert_source_id" {
  value = data.incident_alert_source.example_by_id.id
}

# Output the alert source name
output "alert_source_name" {
  value = data.incident_alert_source.example_by_id.name
}

# Output the alert source type
output "alert_source_type" {
  value = data.incident_alert_source.example_by_id.source_type
}

# Output the alert source secret token
output "alert_source_secret_token" {
  value     = data.incident_alert_source.example_by_id.secret_token
  sensitive = true
} 