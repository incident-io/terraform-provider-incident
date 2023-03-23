resource "incident_catalog_type" "service" {
  name        = "Service"
  description = "All services that we run across our product"
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
