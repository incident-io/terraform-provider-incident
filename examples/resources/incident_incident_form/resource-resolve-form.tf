# A resolve form for one incident type. It collects what is only known at the
# end: when the fix landed, and feedback on the investigation.
data "incident_incident_type" "production_outage" {
  name = "Production Outage"
}

data "incident_incident_timestamp" "fixed" {
  name = "Fixed"
}

resource "incident_custom_field" "root_cause" {
  name        = "Root cause"
  description = "One line on what went wrong."
  field_type  = "text"
}

resource "incident_incident_form" "production_outage_resolve" {
  form_type        = "resolve"
  incident_type_id = data.incident_incident_type.production_outage.id

  lifecycle_elements = [
    {
      element_type          = "timestamp"
      incident_timestamp_id = data.incident_incident_timestamp.fixed.id
      required_if           = "always_require"
    },
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.root_cause.id
      required_if     = "always_require"
      placeholder     = "What went wrong?"
    },
    {
      element_type = "summary"
      description  = "Update the summary to say how it was resolved."
    },
    {
      # Feedback on the AI investigation, with a comment required alongside the rating.
      element_type = "investigation_feedback"
      config       = { require_comment = true }
    },
  ]
}
