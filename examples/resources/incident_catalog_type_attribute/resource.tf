resource "incident_catalog_type" "service" {
  name        = "Service"
  description = "All services that we run across our product"
  # Required. Point this at the repo that manages the type to send people there
  # instead of letting them edit it in the dashboard; "" keeps it editable.
  source_repo_url = ""
}

resource "incident_catalog_type" "service_tier" {
  name            = "Service Tier"
  description     = "Level of importance for each service"
  source_repo_url = ""
}

resource "incident_catalog_type_attribute" "service_description" {
  catalog_type_id = incident_catalog_type.service.id

  name = "Description"
  type = "Text"
}

resource "incident_catalog_type_attribute" "service_team" {
  catalog_type_id = incident_catalog_type.service.id

  name = "Team"
  type = "Text"
}

resource "incident_catalog_type_attribute" "service_service_tier" {
  catalog_type_id = incident_catalog_type.service.id
  name            = "Tier"
  type            = incident_catalog_type.service_tier.type_name
}

# To create a backlink (i.e. Service tier -> Services)
resource "incident_catalog_type_attribute" "service_tier_services" {
  catalog_type_id    = incident_catalog_type.service_tier.id
  name               = "Services"
  type               = incident_catalog_type.service.type_name
  backlink_attribute = incident_catalog_type_attribute.service_service_tier.id
}
