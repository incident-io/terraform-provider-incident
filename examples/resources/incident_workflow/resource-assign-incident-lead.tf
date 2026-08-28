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

# Look the incident lead role up by name, rather than pinning its ID into the
# workflow below. The ID differs between workspaces, so a name keeps the config
# portable.
data "incident_incident_role" "incident_lead" {
  name = "Incident Lead"
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
            literal = data.incident_incident_role.incident_lead.id
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
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = [
    "standard",
  ]
  state = "draft"
}
