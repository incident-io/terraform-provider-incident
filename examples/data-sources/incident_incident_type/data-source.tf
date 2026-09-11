# Look up an incident type by name. Incident types are configured in your
# settings rather than created from Terraform, so this is how you get hold of
# one's ID.
data "incident_incident_type" "production_outage" {
  name = "Production Outage"
}

# Or by ID, if you already have one.
data "incident_incident_type" "by_id" {
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

output "production_outage_id" {
  value = data.incident_incident_type.production_outage.id
}
