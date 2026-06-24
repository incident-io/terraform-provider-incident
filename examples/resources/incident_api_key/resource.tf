# Create an API key with account-level roles only
resource "incident_api_key" "automation" {
  name       = "Automation"
  role_names = ["incident_creator", "incident_editor"]
}

# Create an API key scoped to a specific team, with team-level on-call roles
resource "incident_api_key" "oncall_team" {
  name           = "Security On-Call"
  role_names     = []
  team_ids       = ["01JXXXXXXXXXXXXXXXXXXXXXXX"]
  team_role_names = ["schedules_editor", "on_call_editor"]
}

# Create an API key with the team_memberships_manage role, allowing it to
# control which teams have default access to private incidents via incident types
resource "incident_api_key" "team_access_manager" {
  name       = "Team Access Manager"
  role_names = ["team_memberships_manage"]
}

output "automation_token" {
  value     = incident_api_key.automation.token
  sensitive = true
}
