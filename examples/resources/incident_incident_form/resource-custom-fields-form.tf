# A custom-fields form holds the fields shown in the incident sidebar. It is the
# one lifecycle form with no default. This one has no incident_type_id, so it
# applies to every incident type without a form of its own.
resource "incident_custom_field" "ticket" {
  name        = "Ticket"
  description = "The ticket tracking this incident."
  field_type  = "link"
}

resource "incident_custom_field" "customer_impact" {
  name        = "Customer impact"
  description = "How customers were affected."
  field_type  = "single_select"
}

resource "incident_custom_field_option" "customer_impact_none" {
  custom_field_id = incident_custom_field.customer_impact.id
  value           = "None"
}

resource "incident_incident_form" "custom_fields" {
  form_type = "custom-fields"

  lifecycle_elements = [
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.ticket.id
    },
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.customer_impact.id
      # Required once the incident is resolved, so nobody closes one without it.
      required_if = "check_engine_config"
      required_if_condition_groups = [
        {
          conditions = [
            {
              subject        = "incident.status.category"
              operation      = "one_of"
              param_bindings = [{ array_value = [{ literal = "closed" }] }]
            }
          ]
        }
      ]
      default_value       = { value_literal = incident_custom_field_option.customer_impact_none.id }
      can_select_no_value = true
    },
  ]
}
