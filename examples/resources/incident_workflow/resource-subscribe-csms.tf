# Custom fields and their options both have data sources, so a condition on one
# needs no ULIDs: look the field up by name, and each option by its value.
data "incident_custom_field" "affected_customers" {
  name = "Affected customers"
}

data "incident_custom_field_option" "all_customers" {
  custom_field_id = data.incident_custom_field.affected_customers.id
  value           = "All customers"
}

data "incident_user" "csm_lead" {
  email = "csm-lead@example.com"
}

data "incident_user" "csm_deputy" {
  email = "csm-deputy@example.com"
}

# This is the workflow we run ourselves: subscribe the CSM team to every
# Critical incident, and to Major incidents that affect all customers.
#
# It's the clearest example of how condition groups combine. Conditions within
# a group are ANDed, and the groups are ORed with each other, so this reads as:
#
#   severity is Critical
#     OR (severity is Major AND affected customers is All customers)
#
# Two conditions that must both hold go in one group. Two alternatives, as
# here, need a group each - putting all three conditions in a single group
# would subscribe nobody, since no incident is both Critical and Major.
resource "incident_workflow" "subscribe_csms" {
  name        = "Subscribe CSMs to customer-facing incidents"
  trigger     = "incident.updated"
  expressions = []
  condition_groups = [
    {
      conditions = [
        {
          # "Incident → Severity"
          subject   = "incident.severity"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  # "Critical". Severities are the one value here without a
                  # data source: if you manage yours in Terraform, reference
                  # the resource (incident_severity.critical.id), otherwise
                  # take the ID from the severity's page in the dashboard.
                  literal = "01K675152G91VK4S8YYT7NKPAE"
                },
              ]
            },
          ]
        },
      ]
    },
    {
      conditions = [
        {
          # "Incident → Severity"
          subject   = "incident.severity"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  # "Major"
                  literal = "01KCZFEB7DPVCQQM262JTKN5RH"
                },
              ]
            },
          ]
        },
        {
          # "Incident → Affected customers". Custom fields are addressed in the
          # scope by ID, so interpolate the one the data source found.
          subject   = "incident.custom_field[\"${data.incident_custom_field.affected_customers.id}\"]"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  literal = data.incident_custom_field_option.all_customers.id
                },
              ]
            },
          ]
        },
      ]
    },
  ]
  steps = [
    {
      # "Subscribe a user to the incident"
      id   = "01KDEMWQCS30P3GHAVNG9VGKWS" # This is the ID of the step in the workflow, and must be a ULID
      name = "incident.subscribe_user_to_incident"
      param_bindings = [
        # "Incident"
        {
          value = {
            reference = "incident"
          }
        },
        # "User(s)"
        {
          array_value = [
            {
              literal = data.incident_user.csm_lead.id
            },
            {
              literal = data.incident_user.csm_deputy.id
            },
          ]
        },
      ]
    },
  ]
  # An incident that goes Major then Critical satisfies the conditions twice,
  # and subscribing once per incident keeps that to a single subscription.
  once_for = [
    # "Incident"
    "incident",
  ]
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = [
    "standard",
  ]
  state = "draft"
}
