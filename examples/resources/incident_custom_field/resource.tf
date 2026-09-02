# Create an Affected Teams multi-select field, required always, shown at all
# opportunities.
resource "incident_custom_field" "affected_teams" {
  name        = "Affected Teams"
  description = "The teams that are affected by this incident."
  field_type  = "multi_select"
}

# Catalog-backed fields can offer a restricted set of options, rather than every
# entry of their catalog type.
resource "incident_catalog_type" "service" {
  name        = "Service"
  description = "All services that we run across our product"
}

resource "incident_catalog_type" "service_tier" {
  name        = "Service Tier"
  description = "Level of importance for each service"
}

resource "incident_catalog_type_attribute" "service_service_tier" {
  catalog_type_id = incident_catalog_type.service.id
  name            = "Tier"
  type            = incident_catalog_type.service_tier.type_name
}

resource "incident_catalog_entry" "tier_one" {
  catalog_type_id  = incident_catalog_type.service_tier.id
  name             = "Tier 1"
  attribute_values = []
}

# Offer only our tier 1 services, on every incident. `fixed_filter` restricts the
# options to entries whose Tier attribute is one of the listed values - unlike
# `filter_by`, which follows another custom field's value on the incident.
resource "incident_custom_field" "affected_tier_one_service" {
  name        = "Affected tier 1 service"
  description = "The tier 1 service that is affected by this incident."
  field_type  = "single_select"

  catalog_type_id = incident_catalog_type.service.id

  fixed_filter = {
    catalog_attribute_id = incident_catalog_type_attribute.service_service_tier.id
    values               = [incident_catalog_entry.tier_one.id]
  }
}
