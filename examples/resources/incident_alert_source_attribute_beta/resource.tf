# One attribute of an alert source. The source is an incident_alert_source_beta resource,
# so editing an attribute doesn't mean rewriting the source.

# The simplest form: a fixed value.
resource "incident_alert_source_attribute_beta" "environment" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.environment.id

  value_literal = "production"
}

# Several fixed values, for an attribute that takes an array.
resource "incident_alert_source_attribute_beta" "regions" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.regions.id

  # Later values are added to the ones already on the alert, rather than replacing them.
  merge_strategy = "append"

  values = ["eu-west-1", "eu-west-2"]
}

# A value read straight off the incoming payload.
resource "incident_alert_source_attribute_beta" "service_name" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.service_name.id

  value_reference = "payload.labels.service"
}

# Computed by an expression. Declaring the block is what binds its result, so there is no
# value alongside it.
resource "incident_alert_source_attribute_beta" "team" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.team.id

  # Only the first value survives an update, so a re-fired alert doesn't collect owners.
  merge_strategy = "first_wins"

  expression {
    start_from = "payload"

    # An ordered pipeline: each operation feeds the next.
    operation {
      parse = {
        function = file("${path.module}/service_from_payload.js")
        as       = incident_catalog_type.service.attribute_type
      }
    }
    operation { navigate = { to = incident_catalog_type_attribute.service_owner.id } }
    operation { first = {} }

    # What the expression produces when the pipeline found nothing.
    fallback {
      result = { value_literal = incident_catalog_entry.platform_team.id }
    }
  }
}

# Expressions can also be named and referenced. That is how one falls back to another.
#
# A name only has to be unique within this resource: two attributes of one source can each have
# a "severity_lookup".
resource "incident_alert_source_attribute_beta" "severity" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.severity.id

  expression_ref = "severity_lookup"

  named_expression {
    name = "severity_lookup"

    # A branches-only expression starts from the whole scope, so its conditions reference
    # absolute paths.
    start_from = "."

    operation {
      branches {
        as = incident_alert_attribute.severity.type

        if {
          conditions = [{
            subject   = "payload.labels.severity"
            operation = "one_of"
            params    = [{ values = ["critical", "page"] }]
          }]
          result = { value_literal = "high" }
        }

        else_if {
          conditions = [{
            subject   = "payload.labels.severity"
            operation = "one_of"
            params    = [{ values = ["warning"] }]
          }]
          result = { value_literal = "medium" }
        }
      }
    }

    fallback {
      result = { value_literal = "low" }
    }
  }
}
