data "incident_user" "rory" {
  email = "rory.malcolm@incident.io"
}

data "incident_user" "martha" {
  email = "martha@incident.io"
}

data "incident_user" "roryb" {
  email = "rory@incident.io"
}

resource "incident_schedule" "testing_my_provider" {
  name = "Testing Terraform"

  // from tz database
  timezone = "Europe/London"

  rotations = [{
    // A string ID for a schedule rotation
    id                = "01HPFH8T92MPGSQS5C1SPAF4V0"
    name              = "Testing Terraform"
    versions = [
      {
        // The date that the handover for this rotation version began
        handover_start_at = "2024-05-01T12:54:13Z"

        users = [
          data.incident_user.martha.id,
        ]

        layers = [
          {
            // A string ID for a layer, to be shared across versions if required
            id   = "01HPFH8T92MPGSQS5C1SPAF4V0"
            name = "oncall"
          }
        ]

        // handovers are optional
        handovers = [
          {
            interval_type = "daily"
            interval      = 1
          }
        ]
      },
      {
        // The date that a schedule rotation versin came into effect
        effective_from = "2024-05-14T12:54:13Z"
        // Used to adjust the handover interval candence - for example, 'changes every week, from the tuesday of that week'
        handover_start_at = "2024-05-01T12:54:13Z"

        users = [
          data.incident_user.martha.id,
          data.incident_user.rory.id,
        ]

        layers = [
          {
            id   = "01HPFH8T92MPGSQS5C1SPAF4V0"
            name = "oncall"
          }
        ]

        // A list of handover intervals, can be used to construct 'changes every week, then every 3 days, then every week'
        handovers = [
          {
            interval_type = "weekly"
            interval      = 1
          }
        ]
      },
    ]
  }]
}
