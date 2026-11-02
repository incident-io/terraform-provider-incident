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
  # Required. Point this at the repo that manages the type to send people there
  # instead of letting them edit it in the dashboard; "" keeps it editable.
  source_repo_url = ""
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

# Extend an entry that incident.io syncs, such as a schedule, by adopting it.
# Terraform finds the entry rather than creating it, writes only the custom
# aliases and the attributes in managed_attributes, and leaves the entry in
# place when destroyed.
data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

data "incident_schedule" "primary_on_call" {
  name = "Primary on-call"
}

# An attribute you added to the Schedule type in the incident.io web UI.
data "incident_catalog_type_attribute" "schedule_runbook" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  name            = "Runbook"
}

resource "incident_catalog_entry" "primary_on_call" {
  catalog_type_id = data.incident_catalog_type.schedule.id

  # The entry's ID, external ID or one of its aliases. For a schedule, the
  # external ID is the schedule's ID.
  existing_entry_identifier = data.incident_schedule.primary_on_call.id

  custom_aliases = ["primary", "pd-primary-on-call"]

  managed_attributes = [data.incident_catalog_type_attribute.schedule_runbook.id]
  attribute_values = [
    {
      attribute = data.incident_catalog_type_attribute.schedule_runbook.id
      value     = "https://runbooks.example.com/primary-on-call"
    },
  ]
}
