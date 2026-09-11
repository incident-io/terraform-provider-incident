# Before: one resource holds the schedule and every rotation on it, with each
# rotation's line-up under versions.
resource "incident_schedule" "platform" {
  name     = "Platform on-call"
  timezone = "Europe/London"

  team_ids = [data.incident_catalog_entry.platform_team.id]

  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [
      {
        handover_start_at = "2024-01-08T09:00:00Z"
        users = [
          data.incident_user.alice.id,
          data.incident_user.bob.id,
        ]
        layers = [{
          id   = "primary"
          name = "Primary"
        }]
        handovers = [{
          interval      = 1
          interval_type = "weekly"
        }]
      },
    ]
  }]
}
