data "incident_user" "debriefs_owner" {
  email = "incident-manager@example.com"
}

data "incident_incident_timestamp" "debriefs_closed" {
  name = "Closed at"
}

# A debrief policy: major incidents need a debrief booked in, not just promised.
#
# This one scopes itself with condition_groups rather than applying to every
# incident, so only the incidents that warrant a debrief are held to it.
resource "incident_policy" "debriefs" {
  name        = "Debriefs booked for major incidents"
  description = "Anything at major severity or above needs a debrief in the calendar."

  condition_groups = [
    {
      conditions = [
        {
          subject   = "incident.severity"
          operation = "gte"
          # The ID of the severity to compare against, which is a literal rather
          # than a reference.
          param_bindings = [{ value_literal = "01FCNDV6P870EA6S7TK1DSYD5H" }]
        }
      ]
    }
  ]

  assignment_rules = {
    bindings                       = [{ value_literal = data.incident_user.debriefs_owner.id }]
    reminder_due_date_offset_hours = [-24]
  }

  debrief = {
    # `is_set` takes no parameters, so its bindings list is empty — but the key
    # is still required.
    requirements = [
      {
        conditions = [
          {
            subject        = "debrief.is_scheduled"
            operation      = "is_set"
            param_bindings = []
          }
        ]
      }
    ]

    due_date_config = {
      incident_timestamp_id = data.incident_incident_timestamp.debriefs_closed.id
      days                  = { value_literal = "5" }
      calculation_type      = "weekdays"
    }
  }
}
