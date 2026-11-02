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

# Extend the schedule's catalog entry, which incident.io syncs, with extra aliases
# and a runbook link. Terraform never creates or deletes the entry.
resource "incident_catalog_entry_extension" "primary_on_call" {
  catalog_type_id = data.incident_catalog_type.schedule.id

  # A schedule's catalog entry has the schedule's ID as its external ID.
  identifier = data.incident_schedule.primary_on_call.id

  custom_aliases = ["primary", "pd-primary-on-call"]

  attribute_values = [
    {
      attribute = data.incident_catalog_type_attribute.schedule_runbook.id
      value     = "https://runbooks.example.com/primary-on-call"
    },
  ]
}
