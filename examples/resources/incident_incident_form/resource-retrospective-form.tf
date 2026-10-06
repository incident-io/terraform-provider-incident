# A retrospective form for one incident type: the form for logging an incident
# after the fact. It has elements no other form has, for the channel to create
# and the post-incident flow to start.
data "incident_incident_type" "near_miss" {
  name = "Near miss"
}

data "incident_incident_timestamp" "impact_started" {
  name = "Impact started"
}

data "incident_incident_timestamp" "impact_ended" {
  name = "Impact ended"
}

resource "incident_incident_form" "near_miss_retrospective" {
  form_type        = "retrospective"
  incident_type_id = data.incident_incident_type.near_miss.id

  lifecycle_elements = [
    # Name and incident type are pinned to the top of a retrospective form, in that order.
    { element_type = "name" },
    { element_type = "incident_type" },
    { element_type = "severity" },
    {
      element_type          = "timestamp"
      incident_timestamp_id = data.incident_incident_timestamp.impact_started.id
      required_if           = "always_require"
    },
    {
      element_type          = "timestamp"
      incident_timestamp_id = data.incident_incident_timestamp.impact_ended.id
      required_if           = "always_require"
    },
    { element_type = "summary" },
    { element_type = "divider" },
    { element_type = "slack_channel" },
    { element_type = "announce_retro_incident" },
    {
      element_type = "enter_post_incident_flow"
      description  = "Near misses skip the post-incident flow unless something needs following up."
    },
  ]
}
