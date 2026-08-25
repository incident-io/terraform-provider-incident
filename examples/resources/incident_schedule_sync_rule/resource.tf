# Create a sync target (Slack user group)
resource "incident_schedule_sync_target" "platform_oncall" {
  add_bot_to_group = true

  new_slack_user_group = {
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

# Sync the people on the next upcoming shift, rather than whoever is on call now
resource "incident_schedule_sync_rule" "platform_next_oncall" {
  schedule_id             = incident_schedule.platform.id
  schedule_sync_target_id = incident_schedule_sync_target.platform_next_oncall.id
  sync_type               = "next_on_call"
}

# Scope a rule to a single rotation. Without rotation_id a rule covers every
# rotation on the schedule; set it to sync only one. To feed a group from
# several rotations, create one rule per rotation pointing at the same target.
resource "incident_schedule_sync_rule" "platform_eu_oncall" {
  schedule_id             = incident_schedule.platform.id
  schedule_sync_target_id = incident_schedule_sync_target.platform_eu_oncall.id
  sync_type               = "on_call"
  rotation_id             = "eu"
}

# Keep a manager in the Slack user group regardless of who is on call.
# Omit permanent_member_user_ids to leave existing members unchanged on update;
# set it to [] to clear them.
resource "incident_schedule_sync_rule" "platform_oncall_with_manager" {
  schedule_id             = incident_schedule.platform.id
  schedule_sync_target_id = incident_schedule_sync_target.platform_oncall.id
  sync_type               = "on_call"

  permanent_member_user_ids = [
    data.incident_user.platform_manager.id,
  ]
}
