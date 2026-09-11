# After: the same escalation path, written as a flat map of named sequences. A
# branch says which sequence to continue down rather than holding its nodes, so
# every sequence sits at the same depth and there is no nesting limit.
#
# The resource keeps its address. incident_escalation_path is the new schema in
# v7, so the path's state carries over, minus the path it used to hold, and the
# refresh reads its nodes back in the new shape.
resource "incident_escalation_path" "urgent_support" {
  name = "Urgent support"

  # Which sequence the escalation begins with. There is no equivalent in the old
  # shape, where the first element of path was the start.
  #
  # The API does not store sequence names, so the refresh after an upgrade names
  # them: the one the path starts with is main, and the sequences a branch leads
  # to are main_then and main_else, after the sequence the branch sits in. These
  # are written to match, so the names are not something the first plan has to
  # reconcile. Rename them afterwards if you would rather they read better -
  # that is an update to your own state, not a change to the path.
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
            then = "main_then"
            else = "main_else"
          }
        }
      ]
    }

    # then_path becomes a sequence of its own, named by the branch above.
    main_then = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "user"
              id      = data.incident_user.on_call.id
              urgency = "high"
            }]
            time_to_ack_seconds = 300

            # Write out ack_mode on every level you mean to keep as it is. This
            # resource defaults it to first where v6 defaulted it to all, and a
            # default applies wherever the configuration is silent - so a level
            # that never set it plans a change from all to first, after which
            # the first person to acknowledge cancels everyone else's
            # escalation on the level.
            ack_mode = "all"
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

    main_else = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "user"
              id      = data.incident_user.on_call.id
              urgency = "low"
            }]
            time_to_ack_seconds = 300
            ack_mode            = "all"
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
