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
