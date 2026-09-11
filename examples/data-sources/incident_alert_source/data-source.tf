# Look up a single alert source by ID.
data "incident_alert_source" "pagerduty" {
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

output "pagerduty_alert_events_url" {
  description = "URL to send HTTP alert events to this source"
  value       = data.incident_alert_source.pagerduty.alert_events_url
}

# Or by name, for a source you did not create here and whose ID you do not have. Names are
# not unique, so a name matching more than one source is an error rather than a guess.
data "incident_alert_source" "cron" {
  name = "Cron heartbeat"
}

output "cron_ping_url" {
  description = "URL the cron job pings to say it ran"
  value       = data.incident_alert_source.cron.heartbeat_options.ping_url
}
