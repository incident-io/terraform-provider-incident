# A declare form for one incident type. Elements appear in the order listed. The
# form is reconciled as a whole on every apply, so an element dropped from this
# list is removed from the form.
data "incident_incident_type" "security" {
  name = "Security"
}

data "incident_incident_role" "incident_lead" {
  name = "Incident Lead"
}

resource "incident_custom_field" "affected_system" {
  name        = "Affected system"
  description = "The system this incident affects."
  field_type  = "text"
}

resource "incident_incident_form" "security_declare" {
  form_type        = "declare"
  incident_type_id = data.incident_incident_type.security.id

  lifecycle_elements = [
    # The name element always comes first on a declare form, so list it first.
    { element_type = "name" },
    {
      element_type = "severity"
      required_if  = "always_require"
    },
    {
      element_type     = "incident_role"
      incident_role_id = data.incident_incident_role.incident_lead.id
    },
    {
      element_type = "text"
      description  = "**Security incidents are private by default.** Only add people who need to know."
    },
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.affected_system.id
      placeholder     = "Which system is affected?"
      description     = "Name the service or data store, not the team."
      default_value   = { value_literal = "Unknown" }
    },
    { element_type = "divider" },
    {
      element_type = "summary"
      placeholder  = "What do we know so far?"
    },
  ]
}
