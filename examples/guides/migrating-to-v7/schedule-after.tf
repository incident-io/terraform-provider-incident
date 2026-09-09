# After: the schedule and each of its rotations are separate resources, so a
# change to one rotation leaves the rest of the schedule alone.
#
# Written here with the v6 names, because this is the stage you do while still
# on v6. Stage two drops the _beta suffix from all of them.
resource "incident_schedule_beta" "platform" {
  name     = "Platform on-call"
  timezone = "Europe/London"

  team_ids = [data.incident_catalog_entry.platform_team.id]
}

resource "incident_schedule_rotation_beta" "primary" {
  schedule_id = incident_schedule_beta.platform.id
  name        = "Primary"

  users = [
    data.incident_user.alice.id,
    data.incident_user.bob.id,
  ]

  # handover_start_at is now first_interval_starts_at. It means the same thing:
  # the moment handover intervals are counted from.
  first_interval_starts_at = "2024-01-08T09:00:00Z"

  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]

  # layers are gone. A rotation with two layers is one rotation with
  # concurrent_shifts = 2, which is how many people it puts on call at once.
}

# Claim the schedule that already exists, rather than creating a second one.
import {
  to = incident_schedule_beta.platform
  id = "01ABC123DEF456GHI789JKL"
}

# A rotation is identified by the schedule it belongs to as well as itself.
import {
  to = incident_schedule_rotation_beta.primary
  id = "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX"
}

# Stop managing the old resource without deleting what it points at. Destroying
# it would delete the schedule, its rotations, and everyone's shifts.
removed {
  from = incident_schedule.platform

  lifecycle {
    destroy = false
  }
}
