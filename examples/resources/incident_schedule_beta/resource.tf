# A schedule holds the rotations that decide who is on call. This resource covers
# the schedule itself; rotations are managed separately, so a schedule created
# here has none and nobody is on call until one is added.
resource "incident_schedule_beta" "platform" {
  name     = "Platform on-call"
  timezone = "Europe/London"

  # Optional: teams that own this schedule.
  team_ids = [data.incident_catalog_entry.platform_team.id]

  # Optional: public holidays to show on the schedule.
  holidays_public_config = {
    country_codes = ["GB", "FR"]
  }
}
