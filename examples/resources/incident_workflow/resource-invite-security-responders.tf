# Users have a data source, so a config doesn't need the ULIDs an export writes
# for them: look people up by the email address they use with incident.io.
data "incident_user" "security_lead" {
  email = "security-lead@example.com"
}

# Slack user groups reach the engine through the catalog, as entries of the
# managed "Slack User Group" type. The identifier matches an entry's name,
# external ID or aliases, so the group's handle is enough.
data "incident_catalog_type" "slack_user_group" {
  name = "Slack User Group"
}

data "incident_catalog_entry" "security_responders" {
  catalog_type_id = data.incident_catalog_type.slack_user_group.id
  identifier      = "security-responders"
}

# This workflow pulls the security responders into an incident's channel as soon
# as the incident becomes active.
#
# Four steps sound alike but do different things, so pick deliberately:
#
#   slack.invite_user                   invites users and Slack user groups to
#                                       the incident's channel (this one)
#   incident.add_member                 grants access to a private incident, and
#                                       does nothing on a public one
#   incident.assign_role                gives someone an incident role, such as
#                                       Incident Lead
#   incident.subscribe_user_to_incident subscribes someone to updates without
#                                       making them a participant
#
# Narrow this with conditions on whatever marks the incidents you care about -
# a custom field, the incident's type, or its severity.
resource "incident_workflow" "invite_security_responders" {
  name        = "Invite security responders"
  trigger     = "incident.updated"
  expressions = []
  condition_groups = [
    {
      conditions = [
        {
          # "Incident → Status → Category"
          subject   = "incident.status.category"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  # "Active"
                  literal = "active"
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
      # "Invite user or group to the incident's channel"
      id   = "01KFD47KKJS93YXR3W53MQSFVC" # This is the ID of the step in the workflow, and must be a ULID
      name = "slack.invite_user"
      param_bindings = [
        # "Incident"
        {
          value = {
            reference = "incident"
          }
        },
        # "User(s)"
        {
          array_value = [
            {
              literal = data.incident_user.security_lead.id
            },
          ]
        },
        # "Slack User Group(s)"
        {
          array_value = [
            {
              literal = data.incident_catalog_entry.security_responders.id
            },
          ]
        },
      ]
    },
  ]
  # incident.updated fires on every change to an incident, so run once per
  # incident: without this, everyone would be re-invited on each update.
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
