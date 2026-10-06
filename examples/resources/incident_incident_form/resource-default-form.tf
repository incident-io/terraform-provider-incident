# Every organisation already has a default form of each lifecycle type, which
# can't be created or deleted. To manage one, import it by ID: the ID is in the
# dashboard's URL when editing the form.
import {
  to = incident_incident_form.default_update
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

data "incident_severity" "major" {
  name = "Major"
}

resource "incident_incident_form" "default_update" {
  form_type = "update"
  # No incident_type_id: this is the organisation's default update form.

  lifecycle_elements = [
    # Status is pinned to the top of an update form, whatever order it is listed in.
    { element_type = "status" },
    {
      element_type = "update_message"
      required_if  = "always_require"
      placeholder  = "What's changed since the last update?"
    },
    { element_type = "severity" },
    {
      # Only ask for the next update time once the incident is major or worse.
      element_type = "next_update_in"
      show_if_condition_groups = [
        {
          conditions = [
            {
              subject        = "incident.severity"
              operation      = "one_of"
              param_bindings = [{ array_value = [{ literal = data.incident_severity.major.id }] }]
            }
          ]
        }
      ]
    },
  ]
}
