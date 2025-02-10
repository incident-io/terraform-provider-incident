# Create an alert attribute that points at a single Github user in the catalog
resource "incident_alert_attribute" "github_user" {
  name  = "Github user"
  type  = "CatalogEntry[\"Github User\"]"
  array = false
}
