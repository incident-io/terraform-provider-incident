# Import a schedule sync target using its ID
# Replace the ID with a real ID from your incident.io organization
#
# The Slack user group of an imported target already exists, so the imported
# configuration should point at it with slack_user_group_id (the group's Slack
# ID, which is in state after the import). new_slack_user_group asks us to
# create a group, so leaving it set plans a replacement of the target
import {
  to = incident_schedule_sync_target.example
  id = "01ABC123DEF456GHI789JKL"
}
