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

  # Expressions this source owns, addressed by name. Setting a priority from the payload
  # takes two of them.
  #
  # The payload is opaque JSON, so a condition can't reach inside it: `payload` as a whole
  # is all one can see, and a subject of "payload.labels.severity" resolves to nothing.
  # Parse the value out first...
  named_expression {
    name       = "severity_string"
    start_from = "payload"

    operation {
      parse = {
        # JavaScript, evaluated with the payload bound to `$`.
        function = "$.labels.severity"
        as       = "String"
      }
    }
  }

  # ...then map that string onto a priority. Parsing straight into a priority would be
  # shorter, but it resolves by matching the value against a priority's name, so a payload
  # saying "critical" would match no priority called "Urgent" and every alert would land on
  # the fallback. Branching on the value says what you mean.
  named_expression {
    name       = "severity_lookup"
    start_from = "."

    operation {
      branches {
        # A priority is a catalog entry, so the branches return its catalog type. Take the
        # type from attribute_type rather than writing it out.
        as = data.incident_catalog_type.alert_priority.attribute_type

        if {
          conditions = [{
            subject   = "expressions[\"severity_string\"]"
            operation = "one_of"
            params    = [{ values = ["critical", "page"] }]
          }]
          result = { value_literal = data.incident_catalog_entry.urgent_priority.id }
        }
      }
    }

    # What the expression produces when no branch matched.
    fallback {
      result = { value_literal = data.incident_catalog_entry.in_hours_priority.id }
    }
  }

  priority = {
    expression_ref = "severity_lookup"
  }
}

# An alert's priority is a catalog entry, so the priorities themselves are looked up rather
# than declared. Urgent and In-hours are the ones a new account starts with; use whichever
# your organisation has.
data "incident_catalog_type" "alert_priority" {
  type_name = "AlertPriority"
}

data "incident_catalog_entry" "urgent_priority" {
  catalog_type_id = data.incident_catalog_type.alert_priority.id
  identifier      = "Urgent"
}

data "incident_catalog_entry" "in_hours_priority" {
  catalog_type_id = data.incident_catalog_type.alert_priority.id
  identifier      = "In-hours"
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

  # Pause monitoring without deleting the source, for example during maintenance.
  # Set false (or omit, after it's been applied once) to resume.
  disabled = true
}

# A private source's alerts are visible to nobody until you say which teams can
# see them.
resource "incident_alert_source_beta" "security_scanner" {
  name        = "Security scanner"
  source_type = "http"

  # Every source but a heartbeat needs both of these: leave one out and the API writes
  # its own default, which this resource has nowhere to store.
  title = {
    literal = "{{payload.rule}} on {{payload.target}}"
  }
  description = {
    literal = "{{payload.detail}}"
  }

  is_private = true
  visible_to_teams = {
    values = [data.incident_catalog_entry.security_team.id]
  }
}
