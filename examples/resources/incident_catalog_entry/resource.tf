locals {
  service_tiers = [
    {
      name        = "tier_1"
      description = "Critical customer-facing services"
      external_id = "service-tier-1"
    },
    {
      name        = "tier_2"
      description = "Either customers or internal user processes are impacted if this service fails"
      external_id = "service-tier-2"
    },
    {
      name        = "tier_3"
      description = "Non-essential services"
      external_id = "service-tier-3"
    },
  ]
}

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
  for_each = { for tier in local.service_tiers :
    tier.name => tier
  }

  catalog_type_id = incident_catalog_type.service_tier.id

  name = each.value.name

  external_id = each.value.external_id

  attribute_values = [
    {
      attribute = incident_catalog_type_attribute.service_tier_description.id,
      value     = each.value.description,
    },
  ]
}
