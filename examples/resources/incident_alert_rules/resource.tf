# Each team has one set of team rules, which run for the team's alerts. Teams are catalog
# entries of your Team type, so look the team up rather than hardcoding its ID.
data "incident_catalog_type" "team" {
  name = "Team"
}

data "incident_catalog_entry" "platform" {
  catalog_type_id = data.incident_catalog_type.team.id
  identifier      = "Platform"
}

data "incident_escalation_path" "platform" {
  name = "Platform"
}

resource "incident_alert_rules" "platform" {
  kind    = "team"
  team_id = data.incident_catalog_entry.platform.id

  # Every block runs for an alert. The first branch runs when its conditions hold, and a
  # second branch, with no conditions, runs when they don't.
  block {
    id = "database-alerts"

    branch {
      id = "database"
      condition_groups = [{
        conditions = [{
          subject        = "alert.title"
          operation      = "contains"
          param_bindings = [{ value_literal = "database" }]
        }]
      }]

      actions = [
        {
          id       = "page-platform"
          type     = "escalate"
          escalate = { escalation_paths = { values = [data.incident_escalation_path.platform.id] } }
        },
        {
          id              = "declare-incident"
          type            = "create_incident"
          create_incident = {}
        },
      ]
    }

    branch {
      id = "everything-else"
      actions = [{
        id   = "post-to-channel"
        type = "post_slack"
        post_slack = {
          channels = { values = ["C01234567"] }
          mode     = "per_alert"
        }
      }]
    }
  }
}

# An organisation has one set of required rules, which run for every alert, and one of
# default rules, which run when no team's rules ran. Creating either fails once it exists,
# so import an existing set to manage it. Destroying it empties the set and hands it back
# to the dashboard.
import {
  to = incident_alert_rules.required
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

resource "incident_alert_rules" "required" {
  kind = "required"

  block {
    id = "investigate"

    branch {
      id      = "always"
      actions = [{ id = "investigate", type = "investigate" }]
    }
  }
}
