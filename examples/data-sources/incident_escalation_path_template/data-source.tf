# Look up an escalation path template by name, such as one built in the dashboard.
# Names aren't unique, so prefer `id` when you already have one.
data "incident_escalation_path_template" "team_on_call" {
  name = "Team on-call"
}

# A templated escalation path binds each of the template's params. The params
# are keyed by name, so a config can check what the template asks for.
output "team_on_call_params" {
  value = keys(data.incident_escalation_path_template.team_on_call.params)
}
