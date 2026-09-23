# A template pages a team's primary schedule, then their backup, with the schedule and
# backup left as parameters for each team to fill in.
resource "incident_escalation_path_template" "team_oncall" {
  name        = "Team on-call"
  description = "Primary schedule, then a backup responder"

  # Keyed by the parameter's name, which is what a path binds it under.
  params = {
    primary_schedule = {
      label = "Primary schedule"
      type  = "CatalogEntry[\"Schedule\"]"
    }
    backup = {
      label = "Backup responder"
      type  = "CatalogEntry[\"User\"]"
    }
  }

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
resource "incident_schedule" "payments" {
  name     = "Payments primary"
  timezone = "Europe/London"
}

data "incident_user" "payments_lead" {
  email = "payments-lead@example.com"
}

resource "incident_escalation_path" "payments" {
  name        = "Payments on-call"
  kind        = "templated"
  template_id = incident_escalation_path_template.team_oncall.id

  param_bindings = {
    primary_schedule = { value_literal = incident_schedule.payments.id }
    backup           = { value_literal = data.incident_user.payments_lead.id }
  }
}

# To page one rotation of a bound schedule, bind the target to an expression that
# navigates the schedule's rotations and keeps the one with that name.
resource "incident_escalation_path_template" "primary_rotation" {
  name = "Primary rotation"

  params = {
    schedule = {
      label = "Schedule"
      type  = "CatalogEntry[\"Schedule\"]"
    }
  }

  expressions = [
    {
      label          = "Primary rotation"
      reference      = "primary_rotation"
      root_reference = "schedule"
      operations = [
        {
          operation_type = "navigate"
          navigate       = { reference = "rotations" }
        },
        {
          operation_type = "filter"
          filter = {
            condition_groups = [{
              conditions = [{
                subject        = "input.name"
                operation      = "equals"
                param_bindings = [{ value = { literal = "Primary" } }]
              }]
            }]
          }
        },
      ]
    },
  ]

  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          level = {
            targets = [{
              type          = "schedule"
              urgency       = "high"
              schedule_mode = "currently_on_call_for_rota"
              binding       = { expression_ref = "primary_rotation" }
            }]
            time_to_ack_seconds = 300
          }
        },
      ]
    }
  }
}
