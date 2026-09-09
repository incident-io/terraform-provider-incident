# After: the same escalation path, written as a flat map of named sequences. A
# branch says which sequence to continue down rather than holding its nodes, so
# every sequence sits at the same depth and there is no nesting limit.
#
# Written here with the v6 name, because this is the stage you do while still on
# v6. Stage two drops the _beta suffix.
resource "incident_escalation_path_beta" "urgent_support" {
  name = "Urgent support"

  # Which sequence the escalation begins with. There is no equivalent in the old
  # shape, where the first element of path was the start.
  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          # Keep an id on the nodes something loops back to, and leave it off the rest.
          id = "start"
          branch = {
            # The raw engine condition becomes one attribute per thing an
            # escalation can be tested on: working hours, or the priority it
            # came in at.
            if = {
              working_hours_active = "UK"
            }
            then = "in_hours"
            else = "out_of_hours"
          }
        }
      ]
    }

    # then_path becomes a sequence of its own, named by the branch above.
    in_hours = {
      nodes = [
        {
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
          # repeat is now loop, which names the node to go back to.
          loop = {
            back_to = "start"
            times   = 3
          }
        }
      ]
    }

    out_of_hours = {
      nodes = [
        {
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

  # working_hours, repeat_config and team_ids are unchanged.
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

# Both resources manage the same escalation path through the same API, so claim
# the existing one rather than creating a second.
import {
  to = incident_escalation_path_beta.urgent_support
  id = "01ABC123DEF456GHI789JKL"
}

removed {
  from = incident_escalation_path.urgent_support

  lifecycle {
    destroy = false
  }
}
