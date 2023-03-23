resource "incident_catalog_type" "service_tier" {
  name        = "Service Tier"
  description = "Level of importance for each service"
}

resource "incident_catalog_type_attribute" "service_tier_description" {
  catalog_type_id = incident_catalog_type.service_tier.id

  name = "Description"
  type = "Text"
}

resource "incident_catalog_entry" "service_tier" {
  for_each = {
    for name, tier in [
      {
        name        = "tier_1"
        description = "Critical customer-facing services"
      },
      {
        name        = "tier_2"
        description = "Either customers or internal user processes are impacted if this service fails"
      },
      {
        name        = "tier_3"
        description = "Non-essential services"
      },
    ] : tier.name => tier
  }

  catalog_type_id = incident_catalog_type.service_tier.id

  name = each.value.name

  attribute_values = [
    {
      attribute = incident_catalog_type_attribute.service_tier_description.id,
      value     = each.value.description,
    },
  ]
}
