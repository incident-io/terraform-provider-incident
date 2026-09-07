# Look up an escalation path by name. Names aren't unique across organisations,
# so prefer `id` when you already have one.
data "incident_escalation_path_beta" "urgent_support" {
  name = "Urgent support"
}

output "urgent_support_start" {
  description = "The sequence the path begins with"
  value       = data.incident_escalation_path_beta.urgent_support.start
}
