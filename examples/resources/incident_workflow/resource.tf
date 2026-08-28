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

# Rich text fields hold a document, not a string, so build one from markdown
# rather than writing the JSON by hand. Variables written as {{ }} become
# references into the workflow scope when the message is sent - a plain string
# literal is stored as literal text and won't interpolate. The feature set must
# match the field it's going into: a Slack message is "mrkdwn".
data "incident_rich_text" "exec_page" {
  markdown    = "**Exec page for {{ incident.name }}**\n\n{{ form.reason }}\n\nJoin the incident channel: {{ incident.slack_channel }}"
  feature_set = "mrkdwn"
}

# This is a manually triggered workflow that collects information from the user
# via form fields when they run it. Each field's value is available in the
# workflow scope as `form.<key>`, for both conditions and steps to bind
# against - as the message below does with `form.reason`. That's the prefix the
# dashboard shows as "Form → <title>".
#
# The order of the list is the order the fields appear in the form. Either
# form_fields = [] or omitting the attribute clears any existing fields.
#
# A field's `id` is computed, never written: the provider correlates fields
# across applies by `key`, so retitling a field keeps its identity (and any
# in-flight runs), while changing its key replaces it. That's also why the key,
# not the title, is what the scope reference uses.
resource "incident_workflow" "page_execs" {
  name    = "Page execs"
  trigger = "manual"
  expressions = [
  ]
  condition_groups = [
  ]
  steps = [
    {
      # "Send direct message"
      id   = "01KG1326KD7DT1Z785SYVXXP98" # This is the ID of the step in the workflow, and must be a ULID
      name = "slack.send_message"
      param_bindings = [
        # "Recipient(s)", an array parameter: array-typed bindings always use
        # array_value, even when a single reference supplies every element.
        {
          array_value = [
            {
              reference = "form.execs"
            },
          ]
        },
        # "Message"
        {
          value = {
            literal = data.incident_rich_text.exec_page.json
          }
        },
        # "Timezone", left unset so the step falls back to UTC
        {},
      ]
    },
  ]
  form_fields = [
    {
      key         = "reason"
      title       = "Reason for paging"
      type        = "String"
      description = "Why are we paging the execs?"
      array       = false
      required    = true
    },
    {
      key         = "execs"
      title       = "Who should we page?"
      type        = "User"
      description = "The execs to message. Leave the on-call exec out, they're paged already."
      array       = true
      required    = true
    },
  ]
  once_for = [
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
