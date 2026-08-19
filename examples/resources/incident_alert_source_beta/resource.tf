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

  # An expression this source owns, addressed by name.
  named_expression {
    name = "severity_lookup"

    # A branches-only expression starts from the whole scope, so its conditions
    # reference absolute paths.
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
      }
    }

    # What the expression produces when no branch matched.
    fallback {
      result = { value_literal = "low" }
    }
  }

  priority = {
    expression_ref = "severity_lookup"
  }
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
