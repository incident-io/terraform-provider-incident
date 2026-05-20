# Create a new Slack user group as a sync target
resource "incident_schedule_sync_target" "platform_oncall" {
  add_bot_to_group = true

  new_slack_user_group {
    name        = "Platform On-Call"
    handle      = "platform-oncall"
    description = "Current on-call engineers for the Platform team"
  }
}

# Or use an existing Slack user group
resource "incident_schedule_sync_target" "existing_group" {
  add_bot_to_group    = true
  slack_user_group_id = "S0123456789"
}
