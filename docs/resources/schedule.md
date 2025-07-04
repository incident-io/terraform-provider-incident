---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "incident_schedule Resource - terraform-provider-incident"
subcategory: ""
description: |-
  View and manage schedules.
  Manage your full schedule of on-call rotations, including the users and rotation configuration.
  We'd generally recommend building schedules in our web dashboard https://app.incident.io/~/on-call/schedules, and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing schedule and copy the resulting Terraform without persisting it.
---

# incident_schedule (Resource)

View and manage schedules.
Manage your full schedule of on-call rotations, including the users and rotation configuration.


We'd generally recommend building schedules in our [web dashboard](https://app.incident.io/~/on-call/schedules), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing schedule and copy the resulting Terraform without persisting it.

## Example Usage

```terraform
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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `name` (String) Human readable name synced from external provider
- `rotations` (Attributes Set) (see [below for nested schema](#nestedatt--rotations))
- `timezone` (String)

### Optional

- `holidays_public_config` (Attributes) (see [below for nested schema](#nestedatt--holidays_public_config))
- `team_ids` (Set of String) IDs of teams that own this schedule

### Read-Only

- `id` (String) Unique internal ID of the schedule

<a id="nestedatt--rotations"></a>
### Nested Schema for `rotations`

Required:

- `id` (String) Unique internal ID of the rotation
- `name` (String) Human readable name synced from external provider
- `versions` (Attributes Set) (see [below for nested schema](#nestedatt--rotations--versions))

<a id="nestedatt--rotations--versions"></a>
### Nested Schema for `rotations.versions`

Required:

- `handover_start_at` (String) Defines the next moment we'll trigger a handover
- `handovers` (Attributes List) Defines the handover intervals for this rota, in order they should apply (see [below for nested schema](#nestedatt--rotations--versions--handovers))
- `layers` (Attributes List) Controls how many people are on-call concurrently (see [below for nested schema](#nestedatt--rotations--versions--layers))
- `users` (List of String) The incident.io ID of a user

Optional:

- `effective_from` (String) When this rotation config will be effective from
- `working_intervals` (Attributes List) Optional restrictions that define when to schedule people for this rota (see [below for nested schema](#nestedatt--rotations--versions--working_intervals))

<a id="nestedatt--rotations--versions--handovers"></a>
### Nested Schema for `rotations.versions.handovers`

Required:

- `interval` (Number)
- `interval_type` (String) How often a handover occurs. Possible values are: `hourly`, `daily`, `weekly`.


<a id="nestedatt--rotations--versions--layers"></a>
### Nested Schema for `rotations.versions.layers`

Required:

- `id` (String)
- `name` (String)


<a id="nestedatt--rotations--versions--working_intervals"></a>
### Nested Schema for `rotations.versions.working_intervals`

Required:

- `end_time` (String)
- `start_time` (String)
- `weekday` (String)




<a id="nestedatt--holidays_public_config"></a>
### Nested Schema for `holidays_public_config`

Required:

- `country_codes` (List of String) ISO 3166-1 alpha-2 country codes for the countries that this schedule is configured to view holidays for

## Import

Import is supported using the following syntax:

```shell
#!/bin/bash

# Import a schedule using its ID
# Replace the ID with a real ID from your incident.io organization
terraform import incident_schedule.example 01ABC123DEF456GHI789JKL
```
