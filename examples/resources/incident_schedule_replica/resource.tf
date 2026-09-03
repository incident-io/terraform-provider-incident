# Mirror a schedule into a PagerDuty schedule. The replica_provider_id is the
# PagerDuty schedule ID; replica_fallback_user_id is a PagerDuty user used when
# nobody is on-call in incident.io.
resource "incident_schedule_replica" "platform_pagerduty" {
  schedule_id              = incident_schedule.platform.id
  replica_provider         = "pagerduty"
  replica_provider_id      = "PO8107X"
  replica_fallback_user_id = "PA7AXXN"
  mirror_window_days       = 14

  sources = [{
    rotation_id = "primary"
    layer_id    = "primary"
  }]
}
