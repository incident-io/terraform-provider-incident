# Read the organisation's IP allowlist.
data "incident_ip_allowlist" "current" {}

output "allowed_addresses" {
  description = "IP addresses and CIDR prefixes allowed to access this workspace"
  value       = [for item in data.incident_ip_allowlist.current.allowlist : item.value]
}
