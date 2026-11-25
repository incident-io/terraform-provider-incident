# List the privileges an access grant can give.
data "incident_access_privileges" "all" {}

output "privilege_names" {
  value = [for privilege in data.incident_access_privileges.all.privileges : privilege.name]
}

# Deprecated privileges still work, but are left out unless you ask for them.
data "incident_access_privileges" "including_deprecated" {
  include_deprecated = true
}
