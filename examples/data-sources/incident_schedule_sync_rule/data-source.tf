# Look up a schedule sync rule by schedule ID and rule ID.
data "incident_schedule_sync_rule" "platform_oncall" {
  schedule_id = incident_schedule.platform.id
  id          = "01MNO456PQR789STU012VWX"
}
