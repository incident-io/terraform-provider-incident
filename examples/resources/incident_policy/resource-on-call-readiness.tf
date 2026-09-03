# An on-call readiness policy, which checks that responders have a notification
# method that reaches them quickly enough.
#
# It takes no assignment_rules: this type always assigns the user the finding is
# about, and the API picks that assignee itself.
resource "incident_policy" "responders_can_be_reached" {
  name        = "Responders carry a phone"
  description = "Anyone on call needs a notification method that reaches them quickly."

  # Empty, so the policy applies to everyone.
  condition_groups = []

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
