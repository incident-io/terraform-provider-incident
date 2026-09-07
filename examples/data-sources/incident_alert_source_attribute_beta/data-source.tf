# Look up one attribute binding on an alert source.
data "incident_alert_source_attribute_beta" "environment" {
  alert_source_id    = "01FCNDV6P870EA6S7TK1DSYDG0"
  alert_attribute_id = "01GW2G3V0S59R238FAHPDS1R66"
}

output "environment_merge_strategy" {
  description = "How this attribute is merged when an alert is updated"
  value       = data.incident_alert_source_attribute_beta.environment.merge_strategy
}
