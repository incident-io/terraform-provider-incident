# Teams are catalog entries, so an owning team is resolved through the catalog
# rather than by pasting in its ID. The identifier is matched against entry
# names, external IDs and aliases, so the team's name is enough.
data "incident_catalog_type" "team" {
  name = "Team"
}

data "incident_catalog_entry" "platform" {
  catalog_type_id = data.incident_catalog_type.team.id
  identifier      = "Platform"
}

# This is a workflow that automatically assigns the incident lead role to the user who acked an escalation.
resource "incident_workflow" "autoassign_incident_lead" {
  name    = "Auto-assign incident leader"
  trigger = "escalation.acked"
  expressions = [
  ]
  condition_groups = [
    {
      conditions = [
        {
          # "User who acked the escalation"
          subject   = "user"
          operation = "is_set"
          param_bindings = [
          ]
        },
      ]
    },
  ]
  steps = [
    {
      # "Assign incident roles"
      id   = "01HY0QG9WT62CEYJN8JD74MJNR" # This is the ID of the step in the workflow, and must be a ULID
      name = "incident.assign_role"
      param_bindings = [
        {
          value = {
            # "Incident"
            reference = "incident"
          }
        },
        {
          value = {
            # "Incident Lead"
            literal = "01HB0ZG24MPVF28Z5NF18DQT84" # This is the ID of the incident lead role in our workspace
          }
        },
        {
          value = {
            # "User who acked the escalation"
            reference = "user"
          }
        },
      ]
    },
  ]
  once_for = [
    # "Incident"
    "incident",
  ]
  # Teams that own this workflow. It's a set, so more than one team can own a
  # workflow and the order doesn't matter.
  owning_team_ids = [
    data.incident_catalog_entry.platform.id,
  ]
  include_private_incidents = false
  continue_on_step_error    = false
  runs_on_incidents         = "newly_created_and_active"
  runs_on_incident_modes = [
    "standard",
  ]
  state = "draft"
}

# This is a manually triggered workflow that collects information from the user
# via form fields when they run it. Each field's value is available in the
# workflow scope under `workflow_form.<key>` for use by conditions and steps.
#
# The order of the list is the order the fields appear in the form. Either
# form_fields = [] or omitting the attribute clears any existing fields.
resource "incident_workflow" "page_execs" {
  name    = "Page execs"
  trigger = "manual"
  expressions = [
  ]
  condition_groups = [
  ]
  steps = [
    {
      id   = "01HY0QG9WT62CEYJN8JD74MJNR" # This is the ID of the step in the workflow, and must be a ULID
      name = "slack.send_message"
      param_bindings = [
        {
          value = {
            reference = "incident.slack_channel"
          }
        },
        {
          array_value = [
            {
              reference = "workflow_form.reason"
            }
          ]
        },
      ]
    },
  ]
  form_fields = [
    {
      key         = "reason"
      title       = "Reason for paging"
      type        = "Text"
      description = "Why are we paging the execs?"
      array       = false
      required    = true
    },
  ]
  once_for = [
    "incident",
  ]
  include_private_incidents = false
  continue_on_step_error    = false
  runs_on_incidents         = "newly_created_and_active"
  runs_on_incident_modes = [
    "standard",
  ]
  state = "draft"
}
