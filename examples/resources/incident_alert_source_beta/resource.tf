# An alert source, without the attributes it populates. Each of those is its own
# incident_alert_source_attribute_beta resource, so editing one attribute doesn't
# mean rewriting the source.
resource "incident_alert_source_beta" "prometheus" {
  name        = "Prometheus"
  source_type = "http"

  # Optional: teams that own this alert source.
  owning_team_ids = [data.incident_catalog_entry.platform_team.id]

  # A literal interpolates the alert's scope with {{ }}, and takes the filters
  # truncate and omit_if_unset.
  title = {
    literal = "{{payload.labels.alertname}} on {{payload.labels.service}}"
  }

  # For content a template can't express — formatting, links, lists — build the
  # document from markdown instead. feature_set must match the field: a title is
  # plain_single_line, a description is rich.
  description = {
    literal = data.incident_rich_text.prometheus_alert.json
  }

  # Expressions this source owns, addressed by name. One can reference another.
  #
  # The payload is opaque JSON: an expression reaches into it with a parse operation,
  # and a condition can only ask whether payload as a whole is set. So mapping a payload
  # value onto something takes two expressions — parse the value out, then branch on it.
  named_expression {
    name       = "severity"
    start_from = "payload"

    operation {
      parse {
        function = "$.labels.severity"
        as       = "String"
      }
    }
  }

  # A branches-only expression starts from the whole scope, so its conditions reference
  # absolute paths — including other expressions' results.
  named_expression {
    name       = "priority_lookup"
    start_from = "."

    operation {
      branches {
        # A priority is an AlertPriority catalog entry, so that is what every branch
        # here has to return.
        as = "CatalogEntry[\"AlertPriority\"]"

        if {
          conditions = [{
            subject   = "expressions[\"severity\"]"
            operation = "one_of"
            params    = [{ values = ["critical", "page"] }]
          }]
          result = { value_literal = data.incident_catalog_entry.urgent_priority.id }
        }
      }
    }

    # What the expression produces when no branch matched. Without it, an unmatched
    # severity resolves to nothing and the alert falls back to your default priority.
    fallback {
      result = { value_literal = data.incident_catalog_entry.low_priority.id }
    }
  }

  # Unlike incident_alert_source, priority is a field here rather than a binding on the
  # built-in Priority alert attribute.
  priority = {
    expression_ref = "priority_lookup"
  }
}

# The priorities themselves are AlertPriority catalog entries, looked up by name.

data "incident_catalog_type" "alert_priority" {
  type_name = "AlertPriority"
}

data "incident_catalog_entry" "urgent_priority" {
  catalog_type_id = data.incident_catalog_type.alert_priority.id
  identifier      = "Urgent"
}

data "incident_catalog_entry" "low_priority" {
  catalog_type_id = data.incident_catalog_type.alert_priority.id
  identifier      = "Low"
}

data "incident_rich_text" "prometheus_alert" {
  feature_set = "rich"
  markdown    = <<-EOT
    Fired by the **Prometheus alertmanager**.

    Runbook: {{payload.annotations.runbook_url}}
  EOT
}

# A heartbeat source writes its own title and description, and needs the interval a
# ping is expected within.
resource "incident_alert_source_beta" "nightly_backup" {
  name        = "Nightly backup"
  source_type = "heartbeat"

  heartbeat_options = {
    interval_seconds = 86400

    # Optional: how many missed intervals before we alert, and how long to wait
    # after each one.
    failure_threshold    = 1
    grace_period_seconds = 3600
  }
}

# A private source's alerts are visible to nobody until you say which teams can
# see them.
resource "incident_alert_source_beta" "security_scanner" {
  name        = "Security scanner"
  source_type = "http"

  is_private = true
  visible_to_teams = {
    values = [data.incident_catalog_entry.security_team.id]
  }
}
