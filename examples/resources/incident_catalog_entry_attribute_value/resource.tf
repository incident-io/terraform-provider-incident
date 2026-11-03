data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

data "incident_schedule" "primary_on_call" {
  name = "Primary on-call"
}

# incident.io syncs a catalog entry for each schedule, with the schedule's ID as its
# external ID.
data "incident_catalog_entry" "primary_on_call" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  identifier      = data.incident_schedule.primary_on_call.id
}

# An attribute you add to the Schedule type yourself.
resource "incident_catalog_type_attribute" "schedule_runbook" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  name            = "Runbook"
  type            = "String"
}

resource "incident_catalog_entry_attribute_value" "primary_on_call_runbook" {
  entry_id     = data.incident_catalog_entry.primary_on_call.id
  attribute_id = incident_catalog_type_attribute.schedule_runbook.id
  value        = "https://runbooks.example.com/primary-on-call"
}
