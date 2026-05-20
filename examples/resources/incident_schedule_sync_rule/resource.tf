# Create a sync target (Slack user group)
resource "incident_schedule_sync_target" "platform_oncall" {
  add_bot_to_group = true

  new_slack_user_group {
    name        = "Platform On-Call"
    handle      = "platform-oncall"
    description = "Current on-call engineers for the Platform team"
  }
}

# Link a schedule to the sync target
# Only the current on-call engineer(s) will be synced to the user group
resource "incident_schedule_sync_rule" "platform_oncall" {
  schedule_id             = incident_schedule.platform.id
  schedule_sync_target_id = incident_schedule_sync_target.platform_oncall.id
  sync_type               = "on_call"
}

# Alternatively, sync all users in the rotation (not just who's on call)
resource "incident_schedule_sync_rule" "platform_all_users" {
  schedule_id             = incident_schedule.platform.id
  schedule_sync_target_id = incident_schedule_sync_target.platform_team.id
  sync_type               = "all_users"
}
