# Example 1: Get all alert sources
data "incident_alert_sources" "all" {}

# Example 2: Filter by specific ID
data "incident_alert_sources" "by_id" {
  id = "01GW2G3V0S59R238FAHPDS1R66"
}

# Example 3: Filter by name
data "incident_alert_sources" "by_name" {
  name = "Production Web Dashboard Alerts"
}

# Example 4: Filter by source type
data "incident_alert_sources" "webhooks_only" {
  source_type = "webhook"
}

# Example 5: Filter by multiple criteria (name and source_type)
data "incident_alert_sources" "specific_webhook" {
  name        = "Production Alerts"
  source_type = "webhook"
}

# Output examples
output "all_alert_source_names" {
  description = "Names of all alert sources"
  value       = [for source in data.incident_alert_sources.all.alert_sources : source.name]
}

output "all_alert_source_ids" {
  description = "IDs of all alert sources"
  value       = [for source in data.incident_alert_sources.all.alert_sources : source.id]
}

output "webhook_alert_sources_count" {
  description = "Number of webhook alert sources"
  value       = length(data.incident_alert_sources.webhooks_only.alert_sources)
}

output "specific_alert_source_details" {
  description = "Details of the alert source found by ID"
  value = length(data.incident_alert_sources.by_id.alert_sources) > 0 ? {
    id           = data.incident_alert_sources.by_id.alert_sources[0].id
    name         = data.incident_alert_sources.by_id.alert_sources[0].name
    source_type  = data.incident_alert_sources.by_id.alert_sources[0].source_type
    secret_token = data.incident_alert_sources.by_id.alert_sources[0].secret_token
  } : null
}

# Advanced usage: Create local values for further processing
locals {
  # Group alert sources by type
  alert_sources_by_type = {
    for source in data.incident_alert_sources.all.alert_sources :
    source.source_type => source...
  }

  # Get only webhook sources with specific naming pattern
  production_webhooks = [
    for source in data.incident_alert_sources.webhooks_only.alert_sources :
    source if can(regex("^Production", source.name))
  ]

  # Create a map of alert source names to IDs
  alert_source_name_to_id = {
    for source in data.incident_alert_sources.all.alert_sources :
    source.name => source.id
  }
}

# Output the processed data
output "alert_sources_by_type" {
  description = "Alert sources grouped by type"
  value       = local.alert_sources_by_type
}

output "production_webhook_count" {
  description = "Number of production webhook alert sources"
  value       = length(local.production_webhooks)
}

output "alert_source_lookup" {
  description = "Map of alert source names to IDs for easy lookup"
  value       = local.alert_source_name_to_id
} 