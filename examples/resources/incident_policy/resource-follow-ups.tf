# Who to chase when a follow-up is overdue.
data "incident_user" "followups_owner" {
  email = "engineering-manager@example.com"
}

data "incident_incident_timestamp" "followups_closed" {
  name = "Closed at"
}

# A follow-up policy: the follow-ups left behind by an incident have to be dealt
# with, rather than sitting open indefinitely.
resource "incident_policy" "follow_ups" {
  name        = "Follow-ups actioned within 30 days"
  description = "Follow-ups from an incident shouldn't be left open once it closes."

  # Empty, so the policy applies to every incident.
  condition_groups = []

  # Offsets are in whole days, so use multiples of 24 hours: a nudge the day
  # before, and a chase the day after.
  #
  # The two cadences add recurring reminders on top of those one-offs, repeating
  # until the follow-up is dealt with. Which field holds the cadence is what says
  # before or after, the way the sign of an offset above does.
  assignment_rules = {
    bindings                       = [{ value_literal = data.incident_user.followups_owner.id }]
    reminder_due_date_offset_hours = [-24, 24]

    # Chase weekly in the run-up to the due date, then daily once it's overdue.
    reminder_cadence_before = { interval = "weekly" }
    reminder_cadence_after  = { interval = "daily" }
  }

  follow_up = {
    # A follow-up is compliant once it's out of the open state — completed, or
    # deliberately dropped.
    requirements = [
      {
        conditions = [
          {
            subject        = "follow_up.status"
            operation      = "not_one_of"
            param_bindings = [{ values = ["open"] }]
          }
        ]
      }
    ]

    # Required for follow-up, debrief and post-mortem policies: these types are
    # the ones that carry a due date, and the API rejects an update without one.
    due_date_config = {
      incident_timestamp_id = data.incident_incident_timestamp.followups_closed.id
      days                  = { value_literal = "30" }
      calculation_type      = "seven_days"
    }
  }
}
