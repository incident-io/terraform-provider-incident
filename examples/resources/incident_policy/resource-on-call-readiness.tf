# An on-call readiness policy, which checks that responders have a notification
# method that reaches them quickly enough.
#
# Its assignment_rules take reminders but no bindings: this type always assigns
# the user the finding is about, and the API picks that assignee itself.
resource "incident_policy" "responders_can_be_reached" {
  name        = "Responders carry a phone"
  description = "Anyone on call needs a notification method that reaches them quickly."

  # Empty, so the policy applies to everyone.
  condition_groups = []

  assignment_rules = {
    # A finding is due as soon as it's found, so reminders count from then: one
    # a day later, then daily until it's fixed.
    reminder_due_date_offset_hours = [24]
    reminder_cadence_after         = { interval = "daily" }
  }

  on_call_readiness = {
    high_urgency = [
      {
        method_types      = ["phone", "sms"]
        max_delay_seconds = 300
      }
    ]
    low_urgency = [
      {
        method_types      = ["email"]
        max_delay_seconds = 900
      }
    ]
  }
}
