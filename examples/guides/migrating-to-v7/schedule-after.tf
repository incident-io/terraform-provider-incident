# After: the schedule and each of its rotations are separate resources, so a
# change to one rotation leaves the rest of the schedule alone.
#
# The resource keeps its address. incident_schedule is the new schema in v7, so
# the schedule's state carries over as it stands, minus the rotations it used to
# hold. Nothing is created or destroyed, and nobody's shifts move.
resource "incident_schedule" "platform" {
  name     = "Platform on-call"
  timezone = "Europe/London"

  team_ids = [data.incident_catalog_entry.platform_team.id]
}

resource "incident_schedule_rotation" "primary" {
  schedule_id = incident_schedule.platform.id
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

# The rotation is the one thing that needs claiming: it was part of the schedule
# in v6 and is its own resource now, so there is no state under this address to
# carry over. A rotation is identified by its schedule as well as itself, and its
# own ID is the one the old configuration chose - "primary", here - because the
# old resource sent it.
import {
  to = incident_schedule_rotation.primary
  id = "01ABC123DEF456GHI789JKL:primary"
}
