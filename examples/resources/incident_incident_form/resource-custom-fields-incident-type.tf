# A custom-fields form for one incident type, overriding the organisation-wide
# one: security incidents show a different set of fields in the sidebar.
data "incident_incident_type" "security" {
  name = "Security"
}

resource "incident_custom_field" "data_exposed" {
  name        = "Data exposed"
  description = "Whether customer data was exposed."
  field_type  = "bool"
}

resource "incident_custom_field" "cve" {
  name        = "CVE"
  description = "The CVE this incident relates to, if any."
  field_type  = "text"
}

resource "incident_incident_form" "security_custom_fields" {
  form_type        = "custom-fields"
  incident_type_id = data.incident_incident_type.security.id

  lifecycle_elements = [
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.data_exposed.id
      required_if     = "always_require"
    },
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.cve.id
      placeholder     = "CVE-2026-..."
    },
  ]
}
