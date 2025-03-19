# This is the primary schedule that receives pages in working hours.
resource "incident_schedule" "primary_on_call" {
  name     = "Primary"
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [
      {
        handover_start_at = "2024-05-01T12:00:00Z"
        users             = []
        layers = [
          {
            id   = "primary"
            name = "Primary"
          }
        ]
        handovers = [
          {
            interval_type = "daily"
            interval      = 1
          }
        ]
      },
    ]
  }]
}

# If in working hours, send high-urgency alerts. Otherwise use low-urgency.
resource "incident_escalation_path" "urgent_support" {
  name = "Urgent support"

  path = [
    {
      id   = "start"
      type = "if_else"
      if_else = {
        conditions = [
          {
            operation      = "is_active",
            param_bindings = []
            subject        = "escalation.working_hours[\"UK\"]"
          }
        ]
        then_path = [
          {
            type = "level"
            level = {
              targets = [{
                type    = "schedule"
                id      = incident_schedule.primary_on_call.id
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
                type    = "schedule"
                id      = incident_schedule.primary_on_call.id
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

  # Teams that use this escalation path
  team_ids = ["01FCNDV6P870EA6S7TK1DSYD00", "01FCNDV6P870EA6S7TK1DSYD01"]
}
