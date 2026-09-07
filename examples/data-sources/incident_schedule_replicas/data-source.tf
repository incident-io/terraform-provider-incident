# List the replicas that mirror this schedule into an external provider.
data "incident_schedule_replicas" "platform" {
  schedule_id = incident_schedule.platform.id
}
