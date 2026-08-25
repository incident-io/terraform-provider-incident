# Look up a user to page. Escalation path targets can also be schedules or
# Slack/Teams channels; see the `type` attribute on each target.
data "incident_user" "on_call" {
  email = "on-call@example.com"
}

# The same escalation path as the incident_escalation_path example, written as a
# flat map of sequences: if in working hours, page with high urgency; otherwise
# page with low urgency.
resource "incident_escalation_path_beta" "urgent_support" {
  name = "Urgent support"

  # The sequence the escalation path begins with.
  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          # Named so the loop below can point back here. A loop may only go back to
          # the path's first node or a branch it sits under, so this is the one node
          # in this path a loop is allowed to name.
          id = "start"
          branch = {
            # A branch tests one thing: whether a set of this path's working hours is
            # active, or which priorities the escalation came in at.
            if = {
              working_hours_active = "UK"
            }
            then = "in_hours"
            else = "out_of_hours"
          }
        }
      ]
    }

    in_hours = {
      nodes = [
        {
          # Leave `id` out on nodes nothing loops back to.
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
          delay = {
            delay_seconds = 120
          }
        },
        {
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
