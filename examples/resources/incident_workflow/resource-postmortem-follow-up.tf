# Statuses are managed by incident.io, so look one up by name rather than pasting
# the ULID an export gives you: the ID differs between workspaces, the name is
# what you see in the dashboard.
data "incident_status" "closed" {
  name = "Closed"
}

# This workflow runs when someone moves an incident into Closed, and creates the
# postmortem follow-up assigned to whoever closed it.
#
# The trigger's scope carries three references to bind against: "incident",
# "previous-status" (the status the incident moved out of) and
# "user-who-changed-the-status".
#
# Note that building this in the dashboard pre-fills a "status category is
# Active" condition, which would stop this workflow ever running, Closed not
# being an active category. Terraform writes conditions explicitly so there's
# nothing to remove here, but check for that condition if you export a
# status-driven workflow from the dashboard.
resource "incident_workflow" "postmortem_when_closed" {
  name        = "Create the postmortem follow-up when an incident closes"
  trigger     = "incident.status-changed"
  expressions = []
  condition_groups = [
    {
      conditions = [
        {
          # "Incident → Status"
          subject   = "incident.status"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  literal = data.incident_status.closed.id
                },
              ]
            },
          ]
        },
      ]
    },
  ]
  steps = [
    {
      # "Create follow-ups"
      id   = "01KT8B9QQH67GF9S6SW4A7DNEX" # This is the ID of the step in the workflow, and must be a ULID
      name = "incident.create_follow_ups"
      # Bindings are positional: one per parameter the step declares, in order,
      # with `{}` for the optional parameters you aren't setting.
      param_bindings = [
        # "Incident"
        {
          value = {
            reference = "incident"
          }
        },
        # "Title"
        {
          array_value = [
            {
              literal = "Write the postmortem"
            },
          ]
        },
        # "Assignee", the person who closed the incident
        {
          value = {
            reference = "user-who-changed-the-status"
          }
        },
        # "Labels"
        {},
        # "Description"
        {},
        # "Priority"
        {},
      ]
    },
  ]
  # Running once for the incident means reopening and re-closing it won't create
  # a second follow-up: we've already run these steps for this incident.
  once_for = [
    # "Incident"
    "incident",
  ]
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = [
    "standard",
  ]
  state = "draft"
}
