# A shift conflict policy, which flags users who are on call in two or more
# places at once: two rotations of one schedule, or two different schedules.
# The type has nothing to configure, so its block is empty: it is only there to
# say which type this is.
#
# The API assigns the user the finding is about, so assignment_rules takes
# reminders but no bindings. A finding is due when the conflict starts, so this
# reminds them when it's found and again the day before it starts.
resource "incident_policy" "shift_conflicts" {
  name        = "Nobody on call twice"
  description = "Flag anyone scheduled on call in two places at once."

  condition_groups = []

  assignment_rules = {
    reminder_due_date_offset_hours      = [-24]
    reminder_detected_date_offset_hours = [0]
  }

  shift_conflict = {}
}
