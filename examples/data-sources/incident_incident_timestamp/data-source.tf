# Look up one of the timestamps incident.io sets on every incident, by name.
# This avoids pinning an ID that differs between workspaces.
data "incident_incident_timestamp" "reported" {
  name = "Reported"
}

# Or by ID, for example one you've copied out of your organisation's settings.
data "incident_incident_timestamp" "impact_started" {
  id = "01FCNDV6P870EA6S7TK1DSYD5H"
}

output "reported_timestamp_id" {
  description = "ID of the Reported timestamp, to reference from a workflow"
  value       = data.incident_incident_timestamp.reported.id
}
