
terraform {
  required_providers {
    incident = {
      source  = "incident-io/incident"
      version = "3.0.0"
    }
  }
}

provider "incident" {}

resource "incident_catalog_type" "service_tier" {
  name        = "Service Tier"
  description = "Level of importance for each service"
}

# This is the primary schedule that receives pages in working hours.
resource "incident_schedule" "primary_on_call" {
  name     = "Primary"
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [
      {
        handover_start_at = "2024-05-01T12:00:00Z"
        users             = []
        layers = [
          {
            id   = "primary"
            name = "Primary"
          }
        ]
        handovers = [
          {
            interval_type = "daily"
            interval      = 1
          }
        ]
      },
    ]
  }]
}

# If in working hours, send high-urgency alerts. Otherwise use low-urgency.
resource "incident_escalation_path" "urgent_support" {
  name = "Urgent support v2"

  path = [
    {
        type = "level"
        level = {
            targets = [
                {
                    id = "01J17E4QKFGZ9Z5G6T5VPAB69C"
                    type = "user"
                    urgency = "high"
                    schedule_mode = ""
                }
            ],
            time_to_ack_seconds = 300
        }
    },
    {
        type = "repeat"
        repeat = {
            repeat_times = 3
            to_node      = "start"
        }
    }
  ]

  working_hours = [
    {
      id       = "UK"
      name     = "UK"
      timezone = "Europe/London"
      weekday_intervals = [
        {
          weekday    = "monday"
          start_time = "09:00"
          end_time   = "17:00"
        }
      ]
    }
  ]
}