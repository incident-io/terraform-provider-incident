# Look up an alert route by name, such as one built in the dashboard. Names aren't
# unique, so prefer `id` when you already have one.
data "incident_alert_route" "production" {
  name = "Production incidents"
}

# Or by ID, to reference a route another module manages.
data "incident_alert_route" "staging" {
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

# The route is read in the shape `incident_alert_route` uses, so a block can be
# reused as-is: here, the escalation targets of the production route.
output "production_escalation_targets" {
  value = data.incident_alert_route.production.escalation_config.escalation_targets
}
