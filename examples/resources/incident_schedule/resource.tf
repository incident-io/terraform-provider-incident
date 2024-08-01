data "incident_user" "rory" {
  id = "01HPFH8T92MPGSQS5C1SPAF4V0"
}

data "incident_user" "martha" {
  slack_user_id = "U01HJ1J2Z6Z"
}

# This allows lookups by email, slack user ID, or user ID
data "incident_user" "roryb" {
  email = "rory@incident.io"
}

# This is exportable from the incident.io dashboard as a Terraform configuration
resource "incident_schedule" "primary_on_call" {
  name = "Primary On-call"

  # This is a valid value from the tz database
  # https://en.wikipedia.org/wiki/List_of_tz_database_time_zones
  timezone = "Europe/London"

  rotations = [{
    // A string ID for a schedule rotation, user provided
    id   = "testing-terraform"
    name = "Testing Terraform"
    versions = [
      {
        # The date that the handover for this rotation version began
        # Expects an RFC3339 formatted string
        handover_start_at = "2024-05-01T12:54:13Z"

        # Reference the data sources for the users
        users = [
          data.incident_user.martha.id,
        ]

        # The number of concurrent users that can be on-call at the same time for a given
        # rotation.
        layers = [
          {
            // A string ID for a layer, to be shared across versions if required
            id   = "primary"
            name = "Primary"
          }
        ]

        handovers = [
          {
            # Allowed values are 'daily' and 'weekly' and 'hourly'
            interval_type = "daily"
            interval      = 1
          }
        ]
      },
      {
        # The date that a schedule rotation version came into effect
        # Expects an RFC3339 formatted string
        effective_from = "2024-05-14T12:54:13Z"

        # Expects an RFC3339 formatted string
        # Used to adjust the handover interval candence
        # for example, 'changes every week, from the tuesday of that week'
        handover_start_at = "2024-05-01T12:54:13Z"

        # Reference the data sources for the users
        users = [
          data.incident_user.martha.id,
          data.incident_user.rory.id,
        ]

        # The number of concurrent users that can be on-call at the same time for a given
        # rotation.
        layers = [
          {
            // A string ID for a layer, to be shared across versions if required
            id   = "primary"
            name = "Primary"
          }
        ]

        # A list of handover intervals, can be used to construct 'changes every week, then every 3 days, then every week'
        handovers = [
          {
            # Allowed values are 'daily' and 'weekly' and 'hourly'
            interval_type = "weekly"
            interval      = 1
          }
        ]
      },
    ]
  }]

  # If you want to show a country's public holidays on your schedule, use a list of alpha-2 country codes.
  holidays_public_config = {
    country_codes = ["GB", "FR"]
  }
}
