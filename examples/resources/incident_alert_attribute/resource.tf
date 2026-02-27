# Create an alert attribute that points at a single Github user in the catalog
resource "incident_alert_attribute" "github_user" {
  name     = "Github user"
  type     = "CatalogEntry[\"Github User\"]"
  array    = false
  required = true
}

# Create an optional alert attribute for severity information
resource "incident_alert_attribute" "severity" {
  name     = "Severity"
  type     = "String"
  array    = false
  required = false
  emoji    = "warning"
}
