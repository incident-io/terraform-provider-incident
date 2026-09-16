# A team's grouping preference applies to that team's alerts on every alert route they
# match, ahead of each route's own grouping. Teams are catalog entries of your Team type,
# so look the team up rather than hardcoding its ID.
data "incident_catalog_type" "team" {
  name = "Team"
}

data "incident_catalog_entry" "platform" {
  catalog_type_id = data.incident_catalog_type.team.id
  identifier      = "Platform"
}

# Group the Platform team's alerts by title and service, in a rolling 30 minute window.
# The settings take the same field names as an alert route's grouping_config.
resource "incident_team_grouping_preference" "platform" {
  team_id = data.incident_catalog_entry.platform.id

  default = {
    settings = {
      enabled        = true
      window_type    = "rolling"
      window_seconds = 1800

      grouping_keys = [
        { reference = "alert.title" },
        { reference = "alert.attributes.01H5EXAMPLESERVICEATTRIBUTE" },
      ]
    }
  }
}

# A preference can also switch grouping off for a team, so none of its alerts group
# however the routes they match are configured. A disabled preference carries no window
# or keys.
data "incident_catalog_entry" "payments" {
  catalog_type_id = data.incident_catalog_type.team.id
  identifier      = "Payments"
}

resource "incident_team_grouping_preference" "payments" {
  team_id = data.incident_catalog_entry.payments.id

  default = {
    settings = {
      enabled = false
    }
  }
}
