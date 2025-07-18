# List catalog entries for a specific catalog type
data "incident_catalog_entries" "services" {
  catalog_type_id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

# Example usage: output all entry names
output "service_names" {
  value = [for entry in data.incident_catalog_entries.services.catalog_entries : entry.name]
}

# Example usage: find entries with specific attributes
output "services_with_external_id" {
  value = [
    for entry in data.incident_catalog_entries.services.catalog_entries : {
      name        = entry.name
      external_id = entry.external_id
    }
    if entry.external_id != ""
  ]
}

# Example usage: get all aliases for entries
output "service_aliases" {
  value = {
    for entry in data.incident_catalog_entries.services.catalog_entries :
    entry.name => entry.aliases
    if length(entry.aliases) > 0
  }
}