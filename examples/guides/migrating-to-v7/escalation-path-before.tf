# Before: a branch holds the nodes that follow it, nested under then_path and
# else_path. Terraform schemas cannot recurse indefinitely, so this shape stops
# at five levels of branching.
resource "incident_escalation_path" "urgent_support" {
  name = "Urgent support"

  path = [
    {
      id   = "start"
      type = "if_else"
      if_else = {
        conditions = [
          {
            operation      = "is_active"
            param_bindings = []
            subject        = "escalation.working_hours[\"UK\"]"
          }
        ]
        then_path = [
          {
            type = "level"
            level = {
              targets = [{
                type    = "user"
                id      = data.incident_user.on_call.id
                urgency = "high"
              }]
              time_to_ack_seconds = 300
            }
          },
          {
            type = "repeat"
            repeat = {
              repeat_times = 3
              to_node      = "start"
            }
          }
        ]
        else_path = [
          {
            type = "level"
            level = {
              targets = [{
                type    = "user"
                id      = data.incident_user.on_call.id
                urgency = "low"
              }]
              time_to_ack_seconds = 300
            }
          }
        ]
      }
    }
  ]

  working_hours = [
    {
      id       = "UK"
      name     = "UK"
      timezone = "Europe/London"
      weekday_intervals = [
        {
          weekday    = "monday"
          start_time = "09:00"
          end_time   = "17:00"
        }
      ]
    }
  ]
}
