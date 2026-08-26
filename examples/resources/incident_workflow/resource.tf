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

# A message body is a rich text document, so build it from markdown here rather
# than pasting the JSON an export produces. Anything the feature set can't hold
# is dropped and listed in dropped_content, which you can output to see what
# went missing.
data "incident_rich_text" "leadership_update" {
  markdown    = <<-EOT
    **{{ incident.name }}** — update from {{ update.author }}

    {{ update.message }}

    Now at {{ update.to_status }}, severity {{ update.to_severity }}. Follow along in {{ incident.slack_channel }}.
  EOT
  feature_set = "mrkdwn"
}

# This workflow mirrors every incident update into a leadership channel.
#
# Three Slack steps send messages, and they take different first parameters:
#
#   slack.post_message     posts to a channel, which is where this one goes
#   slack.send_message     sends a DM to users
#   slack.reply_in_thread  replies to a message already in the scope
#
# On Microsoft Teams the equivalent is ms_teams.post_message, whose channel
# parameter takes a MicrosoftTeamsChatChannel and defaults to
# `incident.ms_teams_channel`. Only the steps for your workspace's primary
# comms platform are available, so a config written for one won't apply to the
# other.
resource "incident_workflow" "share_updates_with_leadership" {
  name        = "Share incident updates with leadership"
  trigger     = "incident.update_shared"
  expressions = []
  condition_groups = [
  ]
  steps = [
    {
      # "Send message to a channel"
      id   = "01K5TSNDBF7KBZY9AFDKYXJM3M" # This is the ID of the step in the workflow, and must be a ULID
      name = "slack.post_message"
      param_bindings = [
        # "Channel". Slack channels have no data source, so this is one of the
        # few places an ID is unavoidable: copy it from the channel's details
        # in Slack. To post in the incident's own channel, bind the
        # `incident.slack_channel` reference instead of an ID.
        {
          value = {
            literal = "C02A1FSNEKC"
          }
        },
        # "Message"
        {
          value = {
            literal = data.incident_rich_text.leadership_update.json
          }
        },
        # "Threaded Message"
        {},
        # "Timezone", used to render any timestamps the message contains
        {
          value = {
            literal = "Europe/London"
          }
        },
      ]
    },
  ]
  # Deliberately empty: every update should post, so there's nothing to
  # deduplicate on. Setting once_for = ["incident"] here would post the first
  # update and silently drop the rest.
  once_for               = []
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = [
    "standard",
  ]
  state = "draft"
}

# Custom fields and their options both have data sources, so a condition on one
# needs no ULIDs: look the field up by name, and each option by its value.
data "incident_custom_field" "affected_customers" {
  name = "Affected customers"
}

data "incident_custom_field_option" "all_customers" {
  custom_field_id = data.incident_custom_field.affected_customers.id
  value           = "All customers"
}

data "incident_user" "csm_lead" {
  email = "csm-lead@example.com"
}

data "incident_user" "csm_deputy" {
  email = "csm-deputy@example.com"
}

# This is the workflow we run ourselves: subscribe the CSM team to every
# Critical incident, and to Major incidents that affect all customers.
#
# It's the clearest example of how condition groups combine. Conditions within
# a group are ANDed, and the groups are ORed with each other, so this reads as:
#
#   severity is Critical
#     OR (severity is Major AND affected customers is All customers)
#
# Two conditions that must both hold go in one group. Two alternatives, as
# here, need a group each - putting all three conditions in a single group
# would subscribe nobody, since no incident is both Critical and Major.
resource "incident_workflow" "subscribe_csms" {
  name        = "Subscribe CSMs to customer-facing incidents"
  trigger     = "incident.updated"
  expressions = []
  condition_groups = [
    {
      conditions = [
        {
          # "Incident → Severity"
          subject   = "incident.severity"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  # "Critical". Severities are the one value here without a
                  # data source: if you manage yours in Terraform, reference
                  # the resource (incident_severity.critical.id), otherwise
                  # take the ID from the severity's page in the dashboard.
                  literal = "01K675152G91VK4S8YYT7NKPAE"
                },
              ]
            },
          ]
        },
      ]
    },
    {
      conditions = [
        {
          # "Incident → Severity"
          subject   = "incident.severity"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  # "Major"
                  literal = "01KCZFEB7DPVCQQM262JTKN5RH"
                },
              ]
            },
          ]
        },
        {
          # "Incident → Affected customers". Custom fields are addressed in the
          # scope by ID, so interpolate the one the data source found.
          subject   = "incident.custom_field[\"${data.incident_custom_field.affected_customers.id}\"]"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  literal = data.incident_custom_field_option.all_customers.id
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
      # "Subscribe a user to the incident"
      id   = "01KDEMWQCS30P3GHAVNG9VGKWS" # This is the ID of the step in the workflow, and must be a ULID
      name = "incident.subscribe_user_to_incident"
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
              literal = data.incident_user.csm_lead.id
            },
            {
              literal = data.incident_user.csm_deputy.id
            },
          ]
        },
      ]
    },
  ]
  # An incident that goes Major then Critical satisfies the conditions twice,
  # and subscribing once per incident keeps that to a single subscription.
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
