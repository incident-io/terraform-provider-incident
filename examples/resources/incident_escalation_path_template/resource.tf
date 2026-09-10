# A template pages a team's primary schedule, then their backup, with the schedule and
# backup left as parameters for each team to fill in.
resource "incident_escalation_path_template" "team_oncall" {
  name        = "Team on-call"
  description = "Primary schedule, then a backup responder"

  params = [
    {
      name  = "primary_schedule"
      label = "Primary schedule"
      type  = "CatalogEntry[\"Schedule\"]"
    },
    {
      name  = "backup"
      label = "Backup responder"
      type  = "CatalogEntry[\"User\"]"
    },
  ]

  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "schedule"
              urgency = "high"
              # Bound per templated path, from the param of that name.
              binding = { value_reference = "primary_schedule" }
            }]
            time_to_ack_seconds = 300
          }
        },
        {
          level = {
            targets = [{
              type    = "user"
              urgency = "high"
              binding = { value_reference = "backup" }
            }]
            time_to_ack_seconds = 600
          }
        },
      ]
    }
  }
}

# The paths built from it. Each names the template and binds its params, and has no
# sequences of its own.
resource "incident_schedule_beta" "payments" {
  name     = "Payments primary"
  timezone = "Europe/London"
}

data "incident_user" "payments_lead" {
  email = "payments-lead@example.com"
}

resource "incident_escalation_path_beta" "payments" {
  name        = "Payments on-call"
  template_id = incident_escalation_path_template.team_oncall.id

  param_bindings = {
    primary_schedule = { value_literal = incident_schedule_beta.payments.id }
    backup           = { value_literal = data.incident_user.payments_lead.id }
  }
}
