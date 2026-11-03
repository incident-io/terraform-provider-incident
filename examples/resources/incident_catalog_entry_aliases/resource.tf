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

resource "incident_catalog_entry_aliases" "primary_on_call" {
  entry_id = data.incident_catalog_entry.primary_on_call.id
  aliases  = ["primary", "pd-primary-on-call"]
}
