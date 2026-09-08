# incident.io returns an API key's token when it issues one - on create, and on each
# rotation - and never again. Terraform keeps it in state, so anything that can read your
# state can read the token: treat the state file as the credential it now holds.
resource "incident_api_key" "ci" {
  name     = "CI deploy key"
  comments = "Requested in #ask-infra, used by the deploy pipeline"

  role_names = ["viewer", "catalog_viewer"]
}

# Pass the token on through a sensitive output rather than copying it around. Terraform
# refuses to print an output built from a sensitive attribute unless you mark it too.
output "ci_api_key_token" {
  value     = incident_api_key.ci.token
  sensitive = true
}

# Rotating is asked for by changing token_version - conventionally by incrementing it.
# That's the change Terraform can see, since the token itself is invisible to a plan.
# The previous token keeps working for rotation_grace_period_minutes, giving whatever
# holds it a window to pick up the new one.
resource "incident_api_key" "rotated_quarterly" {
  name       = "Terraform state reader"
  role_names = ["viewer"]

  token_version                 = 2
  rotation_grace_period_minutes = 60
}

# A key can be scoped to particular teams instead of the whole account. team_ids says
# which teams, and team_role_names says what the key may do for them, so the two go
# together: set both, or neither. An account with no account-level roles leaves
# role_names out entirely.
resource "incident_api_key" "platform_team_schedules" {
  name = "Platform team schedule sync"

  team_ids        = ["01G0J1EXE7AXZ2C93K61WBPYEH"]
  team_role_names = ["schedules_editor", "on_call_editor"]
}

# Editing a key's roles never rotates it: the token outlives its permissions. Rotate
# deliberately if narrowing a key's scopes should also stop the old token being accepted.
resource "incident_api_key" "narrowed" {
  name       = "Read-only reporting key"
  role_names = ["viewer"]

  # Bumped at the same time as the roles above were narrowed, so the token that held the
  # wider scopes stops working.
  token_version                 = 3
  rotation_grace_period_minutes = 0
}
