# Look up a schedule sync target by ID.
data "incident_schedule_sync_target" "by_id" {
  id = "01ABC123DEF456GHI789JKL"
}

# ...or by the Slack user group it keeps in sync. The group's Slack-assigned
# ID starts with 'S'; this is not the @-handle.
data "incident_schedule_sync_target" "by_slack_group" {
  slack_user_group_id = "S06MNNU5BMK"
}
