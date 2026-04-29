## Most alert sources share the same structure: a name, a source_type, and a template
## that describes how to map the incoming payload to an incident alert.
##
## We'd generally recommend building alert sources in the incident.io web dashboard
## (https://app.incident.io/~/alerts/configuration) and using the 'Export' flow to
## generate your Terraform, rather than hand-crafting the template. The examples below
## cover the structural variants — source types that require extra options blocks or
## have different template rules.

## -----------------------------------------------------------------------------
## Generic webhook source (e.g. cloudwatch, datadog, grafana, sns, …)
## -----------------------------------------------------------------------------
## All standard webhook-based source types follow this pattern. The template
## title and description are JSON-encoded rich-text documents; use the Export
## flow in the dashboard to get the exact values for your source type.

resource "incident_alert_source" "cloudwatch" {
  name        = "CloudWatch Alerts"
  source_type = "cloudwatch"

  template = {
    title = {
      literal = jsonencode({
        content = [{
          content = [{
            attrs = { label = "Payload → Title", missing = false, name = "title" }
            type  = "varSpec"
          }]
          type = "paragraph"
        }]
        type = "doc"
      })
    }

    description = {
      literal = jsonencode({
        content = [{
          content = [{
            attrs = { label = "Payload → Description", missing = false, name = "description" }
            type  = "varSpec"
          }]
          type = "paragraph"
        }]
        type = "doc"
      })
    }

    ## Bind the `team` expression to an Alert Attribute to label Alerts
    attributes = [
      {
        alert_attribute_id = data.incident_alert_attribute.team.id
        binding = {
          value          = { reference = "expressions[\"cloudwatch-team\"]" }
          merge_strategy = "first_wins"
        }
      },
    ]

    ## Parse the `team` value from the SNS endpoint query string
    expressions = [
      {
        label          = "Team"
        reference      = "cloudwatch-team"
        root_reference = "payload"
        operations = [
          {
            operation_type = "parse"
            parse = {
              returns = { array = false, type = "CatalogEntry[\"CatalogEntryID\"]" }
              source  = "$['query_params']['team']"
            }
          }
        ]
      },
    ]
  }
}

data "incident_alert_attribute" "team" {
  name = "Team"
}

resource "aws_sns_topic" "alerts" {
  name = "cloudwatch-alerts"
}

resource "aws_sns_topic_subscription" "incidentio_alert_source" {
  endpoint               = "https://api.incident.io/v2/alert_events/cloudwatch/${incident_alert_source.cloudwatch.id}?team=platform"
  endpoint_auto_confirms = true
  protocol               = "https"
  topic_arn              = aws_sns_topic.alerts.arn
}

## -----------------------------------------------------------------------------
## Heartbeat source
## -----------------------------------------------------------------------------
## Heartbeat sources fire an alert when a ping is NOT received on time. They
## require a heartbeat_options block and must NOT set template title/description
## (the API manages those automatically).

resource "incident_alert_source" "heartbeat" {
  name        = "Nightly Batch Job"
  source_type = "heartbeat"

  heartbeat_options = {
    ## How often a ping is expected
    interval_seconds = 86400
    ## Grace period before the heartbeat is considered late (0 = fail immediately)
    grace_period_seconds = 300
    ## Number of consecutive missed pings before an alert fires
    failure_threshold = 1
  }

  template = {
    ## Title and description are managed automatically for heartbeat sources
    title       = {}
    description = {}
    attributes  = []
    expressions = []
  }
}

## POST to this URL from your job to signal it is healthy
output "heartbeat_ping_url" {
  value = incident_alert_source.heartbeat.heartbeat_options.ping_url
}

## -----------------------------------------------------------------------------
## Jira source
## -----------------------------------------------------------------------------
## Jira sources watch one or more Jira projects for new issues and turn them
## into alerts. They require a jira_options block specifying which projects to
## watch.

resource "incident_alert_source" "jira" {
  name        = "Jira Bug Reports"
  source_type = "jira"

  jira_options = {
    ## IDs of Jira projects (or catalog entry IDs for the 'Jira Project' catalog type)
    project_ids = ["10001", "10002"]
  }

  template = {
    title       = { reference = "payload.fields.summary" }
    description = { reference = "payload.fields.description" }
    attributes  = []
    expressions = []
  }
}

## -----------------------------------------------------------------------------
## HTTP Custom source
## -----------------------------------------------------------------------------
## HTTP Custom sources accept arbitrary webhook payloads and use a JavaScript
## expression to transform them into an alert. They require an
## http_custom_options block with a transform expression and a deduplication
## key path.

resource "incident_alert_source" "http_custom" {
  name        = "Internal Platform Alerts"
  source_type = "http_custom"

  http_custom_options = {
    ## JavaScript expression that returns an object with the alert fields
    transform_expression = <<-JS
      ({
        title:       payload.alert_name,
        description: payload.message,
        status:      payload.status === "resolved" ? "resolved" : "firing",
      })
    JS
    ## JSON path used to deduplicate repeated firings of the same alert
    deduplication_key_path = "$.alert_id"
  }

  template = {
    title       = { reference = "expressions[\"title\"]" }
    description = { reference = "expressions[\"description\"]" }
    attributes  = []
    expressions = []
  }
}
