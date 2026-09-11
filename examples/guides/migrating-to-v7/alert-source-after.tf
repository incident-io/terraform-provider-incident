# After: the source is one resource and each attribute binding is another, with
# its own lifecycle. Filling in one more attribute is an add rather than an edit
# of everything else.
#
# The source keeps its address. incident_alert_source is the new schema in v7,
# so its state carries over, minus the template it used to hold.
resource "incident_alert_source" "prometheus" {
  name        = "Prometheus"
  source_type = "http"

  owning_team_ids = [data.incident_catalog_entry.platform_team.id]

  # title, description and is_private come out of template to the top level.
  # A title takes a {{ }} template; a description that needs formatting, links
  # or lists is built from markdown with data.incident_rich_text.
  title = {
    literal = "{{payload.labels.alertname}} on {{payload.labels.service}}"
  }

  description = {
    literal = data.incident_rich_text.prometheus_alert.json
  }

  is_private = false

  # template.expressions becomes expression and named_expression blocks, and
  # template.visible_to_teams becomes visible_to_teams. An expression feeding a
  # single attribute belongs on that attribute's resource.
}

# Each entry of template.attributes becomes one of these. The binding's value
# keeps its shape, and merge_strategy comes with it. These are the resources
# that need claiming: the bindings lived inside the source in v6, so there is no
# state under these addresses to carry over.
resource "incident_alert_source_attribute" "environment" {
  alert_source_id    = incident_alert_source.prometheus.id
  alert_attribute_id = incident_alert_attribute.environment.id

  # value = { literal = "..." } is still accepted; value_literal is shorthand.
  value_literal  = "production"
  merge_strategy = "first_wins"
}

resource "incident_alert_source_attribute" "regions" {
  alert_source_id    = incident_alert_source.prometheus.id
  alert_attribute_id = incident_alert_attribute.regions.id

  # An array of fixed values is values; array_value is for a mix of fixed
  # values and references.
  values         = ["eu-west-1", "eu-west-2"]
  merge_strategy = "append"
}

# A binding is identified by its source and the attribute it binds.
import {
  to = incident_alert_source_attribute.environment
  id = "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX"
}

import {
  to = incident_alert_source_attribute.regions
  id = "01ABC123DEF456GHI789JKL:01STU012VWX345YZA678BCD"
}
