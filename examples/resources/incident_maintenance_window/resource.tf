data "incident_user" "ops_lead" {
  email = "ops-lead@example.com"
}

resource "incident_maintenance_window" "example" {
  name            = "Scheduled Database Migration"
  start_at        = "2026-04-01T02:00:00Z"
  end_at          = "2026-04-01T06:00:00Z"
  lead_id         = data.incident_user.ops_lead.id
  show_in_sidebar = true
  resolve_on_end  = true
  reroute_on_end  = false

  alert_condition_groups {
    conditions {
      subject   = "alert.title"
      operation = "contains"
      param_bindings {
        value {
          literal = "database"
        }
      }
    }
  }
}
