# Look up an incident template by name, to reference one the dashboard owns
# rather than managing it here — for example from an alert route.
data "incident_incident_template" "payments" {
  name = "Payments incidents"
}

# Or by ID, for example one copied out of your organisation's settings.
data "incident_incident_template" "default" {
  id = "01FCNDV6P870EA6S7TK1DSYD5H"
}

output "payments_template_id" {
  description = "ID to pass to incident_config.incident_template"
  value       = data.incident_incident_template.payments.id
}
