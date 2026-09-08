# Look up an existing API key by name, such as one created in the dashboard.
data "incident_api_key" "ci" {
  name = "CI deploy key"
}

# Or by ID, to reference a key another module manages. Names aren't unique, so an ID is
# the way through an ambiguous lookup.
data "incident_api_key" "reporting" {
  id = "01G0J1EXE7AXZ2C93K61WBPYEH"
}

# A lookup can tell you what a key may do, but never how to authenticate as it: the token
# is returned only when incident.io issues one, so there's no token attribute here.
output "ci_key_roles" {
  value = data.incident_api_key.ci.role_names
}

# last_used_at is null for a key nothing has authenticated with, which is how you find the
# ones worth deleting.
output "ci_key_last_used_at" {
  value = data.incident_api_key.ci.last_used_at
}
