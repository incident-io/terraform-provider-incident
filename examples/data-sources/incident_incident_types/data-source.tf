# Example usage of the incident_incident_types data source

# Get all incident types
data "incident_incident_types" "all" {}

# Output the incident types
output "incident_types" {
  description = "List of all incident types"
  value       = data.incident_incident_types.all.incident_types
}

output "incident_type_names" {
  description = "Names of all incident types"
  value       = [for it in data.incident_incident_types.all.incident_types : it.name]
}