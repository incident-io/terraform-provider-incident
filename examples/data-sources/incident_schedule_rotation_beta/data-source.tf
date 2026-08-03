# Look up a rotation by name. Names are unique within a schedule, so this is
# unambiguous.
data "incident_schedule_rotation_beta" "primary" {
  schedule_id = incident_schedule_beta.platform.id
  name        = "Primary"
}

# ...or by ID.
data "incident_schedule_rotation_beta" "primary_by_id" {
  schedule_id = incident_schedule_beta.platform.id
  id          = "01MNO456PQR789STU012VWX"
}

# Useful for pointing an escalation path at one rotation rather than the whole
# schedule, when the rotation itself isn't managed here.
resource "incident_escalation_path" "platform" {
  name = "Platform"

  path = [
    {
      type = "level"
      level = {
        targets = [
          {
            type             = "schedule"
            id               = incident_schedule_beta.platform.id
            urgency          = "high"
            schedule_mode    = "all_users_for_rota"
            selected_rota_id = data.incident_schedule_rotation_beta.primary.id
          }
        ]
      }
    }
  ]
}
