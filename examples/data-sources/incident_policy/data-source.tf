# Look up a policy by name, to reference one the dashboard owns rather than
# managing it here.
data "incident_policy" "postmortems" {
  name = "Post-mortems within 5 working days"
}

# Or by ID, for example one copied out of your organisation's settings.
data "incident_policy" "followups" {
  id = "01FCNDV6P870EA6S7TK1DSYD5H"
}

output "postmortem_policy_type" {
  description = "Which of the six policy types this is"
  value       = data.incident_policy.postmortems.policy_type
}
