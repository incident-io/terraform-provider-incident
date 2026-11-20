# The organisation default groups every alert that no team's preference groups. Alert
# rules use the first branch whose conditions hold, else the default.
resource "incident_grouping_preference" "organisation" {
  scope = "organisation"

  default = {
    settings = {
      enabled        = true
      window_type    = "rolling"
      window_seconds = 1800
      grouping_keys  = [{ reference = "alert.title" }]
    }
  }

  # Database alerts group in a shorter window.
  branch {
    condition_groups = [{
      conditions = [{
        subject        = "alert.title"
        operation      = "contains"
        param_bindings = [{ value_literal = "database" }]
      }]
    }]

    settings = {
      enabled        = true
      window_type    = "fixed"
      window_seconds = 600
    }
  }
}

# A team's preference groups that team's alerts. Teams are catalog entries of your Team
# type, so look the team up rather than hardcoding its ID.
data "incident_catalog_type" "team" {
  name = "Team"
}

data "incident_catalog_entry" "payments" {
  catalog_type_id = data.incident_catalog_type.team.id
  identifier      = "Payments"
}

resource "incident_grouping_preference" "payments" {
  scope   = "team"
  team_id = data.incident_catalog_entry.payments.id

  # None of the team's alerts group, unless a branch below matches. Disabled settings carry
  # no window or keys.
  default = {
    settings = {
      enabled = false
    }
  }

  branch {
    condition_groups = [{
      conditions = [{
        subject        = "alert.title"
        operation      = "is_set"
        param_bindings = []
      }]
    }]

    settings = {
      enabled        = true
      window_type    = "rolling"
      window_seconds = 900
    }
  }
}
