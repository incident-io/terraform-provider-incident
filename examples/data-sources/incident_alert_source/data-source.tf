# Look up a single alert source by ID.
data "incident_alert_source" "pagerduty" {
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

output "pagerduty_alert_events_url" {
  description = "URL to send HTTP alert events to this source"
  value       = data.incident_alert_source.pagerduty.alert_events_url
}
