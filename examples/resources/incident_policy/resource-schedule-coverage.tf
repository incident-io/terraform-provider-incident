data "incident_user" "schedule_owner" {
  email = "on-call-manager@example.com"
}

# A schedule policy, which finds gaps in on-call coverage.
#
# Unlike follow-up, debrief and post-mortem policies this one takes no
# due_date_config: a coverage gap is a finding the moment it's spotted, so
# there is no due date to count from. It's the one type that instead supports
# reminders measured from when the gap was detected.
resource "incident_policy" "schedule_coverage" {
  name        = "On-call schedules have no gaps"
  description = "Every rotation should have someone on call at all times."

  # Empty, so the policy applies to every schedule.
  condition_groups = []

  assignment_rules = {
    bindings = [{ value_literal = data.incident_user.schedule_owner.id }]

    # No due date to measure from, so this stays empty and the reminders below
    # do the work instead.
    reminder_due_date_offset_hours = []

    # Whole days from when the gap was found: straight away, then again after
    # two days if it's still there.
    reminder_detected_date_offset_hours = [0, 48]
  }

  schedule = {
    # Check that cover is contiguous, with no uncovered stretches.
    requirement_type = "contiguous"

    # Judge each rotation separately rather than the schedule as a whole, so one
    # rotation covering for another doesn't hide a gap. Defaults to "schedule".
    evaluation_level = "rotation"
  }
}
