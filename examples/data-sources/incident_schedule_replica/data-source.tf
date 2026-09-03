# Look up a schedule replica by schedule ID and replica ID.
data "incident_schedule_replica" "platform_pagerduty" {
  schedule_id = incident_schedule.platform.id
  id          = "01MNO456PQR789STU012VWX"
}
