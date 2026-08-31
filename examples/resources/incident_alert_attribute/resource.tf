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

# An attribute is account-wide, so declare it once. What differs between environments is the
# binding that fills it in, which belongs to an alert source.
resource "incident_alert_attribute" "gcp_service" {
  name  = "GCP service"
  type  = "String"
  array = false
}

# Staging and production each parse the same attribute out of their own source, so both
# environments' alerts are labelled with one attribute that routes can match on.
resource "incident_alert_source_attribute_beta" "gcp_service_staging" {
  alert_source_id    = incident_alert_source_beta.gcp_staging.id
  alert_attribute_id = incident_alert_attribute.gcp_service.id

  value_reference = "payload.resource.labels.service_name"
}

resource "incident_alert_source_attribute_beta" "gcp_service_production" {
  alert_source_id    = incident_alert_source_beta.gcp_production.id
  alert_attribute_id = incident_alert_attribute.gcp_service.id

  value_reference = "payload.resource.labels.service_name"
}

# Where a source lives in a different workspace to the attribute it binds, read the attribute
# by name rather than declaring it again. Two workspaces declaring the same name will both plan
# cleanly, and then the second one to apply will fail.
data "incident_alert_attribute" "existing_gcp_service" {
  name = "GCP service"
}

resource "incident_alert_source_attribute_beta" "gcp_service_other_workspace" {
  alert_source_id    = incident_alert_source_beta.gcp_other.id
  alert_attribute_id = data.incident_alert_attribute.existing_gcp_service.id

  value_reference = "payload.resource.labels.service_name"
}
