# After: the source is one resource and each attribute binding is another, with
# its own lifecycle. Filling in one more attribute is an add rather than an edit
# of everything else.
#
# Written here with the v6 names, because this is the stage you do while still
# on v6. Stage two drops the _beta suffix from all of them.
resource "incident_alert_source_beta" "prometheus" {
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

  # template.expressions becomes expression and named_expression blocks, on
  # whichever resource uses them: an expression feeding one attribute belongs on
  # that attribute's resource.
}

# Each entry of template.attributes becomes one of these. The binding's value
# keeps its shape, and merge_strategy comes with it.
resource "incident_alert_source_attribute_beta" "environment" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.environment.id

  # value = { literal = "..." } is still accepted; value_literal is shorthand.
  value_literal  = "production"
  merge_strategy = "first_wins"
}

resource "incident_alert_source_attribute_beta" "regions" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.regions.id

  # An array of fixed values is values; array_value is for a mix of fixed
  # values and references.
  values         = ["eu-west-1", "eu-west-2"]
  merge_strategy = "append"
}

import {
  to = incident_alert_source_beta.prometheus
  id = "01ABC123DEF456GHI789JKL"
}

# An attribute binding is identified by its source and the attribute it binds.
import {
  to = incident_alert_source_attribute_beta.environment
  id = "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX"
}

import {
  to = incident_alert_source_attribute_beta.regions
  id = "01ABC123DEF456GHI789JKL:01STU012VWX345YZA678BCD"
}

removed {
  from = incident_alert_source.prometheus

  lifecycle {
    destroy = false
  }
}
