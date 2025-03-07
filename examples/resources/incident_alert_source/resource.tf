## Create a basic Alert Source that recieves from an SNS Topic in AWS

resource "incident_alert_source" "cloudwatch" {
  name        = "CloudWatch Alerts"
  source_type = "cloudwatch"
  template = {
    title = {
      literal = jsonencode({
        content = [
          {
            content = [
              {
                attrs = {
                  label   = "Payload → Title"
                  missing = false
                  name    = "title"
                }
                type = "varSpec"
              },
            ]
            type = "paragraph"
          },
        ]
        type = "doc"
      })
    }

    description = {
      literal = jsonencode({
        content = [
          {
            content = [
              {
                attrs = {
                  label   = "Payload → Description"
                  missing = false
                  name    = "description"
                }
                type = "varSpec"
              },
            ]
            type = "paragraph"
          },
        ]
        type = "doc"
      })
    }

    ## Bind the `team` expression to an Alert Attribute we can use to label our Alerts
    attributes = [
      {
        alert_attribute_id = data.incident_alert_attribute.team.id
        binding = {
          value = {
            ## Bind the expression below to this attribute for this Source
            reference = "expressions[\"cloudwatch-team\"]"
          }
        }
      },
    ]

    ## Query the `team` value from the endpoint referenced in the SNS Topic Subscription
    expressions = [
      {
        label = "Team"
        operations = [
          {
            operation_type = "parse"
            parse = {
              returns = {
                array = false
                ## This'll bind to some Catalog Entry Type
                type = "CatalogEntry[\"CatalogEntryID\"]"
              }
              source = "$['query_params']['team']"
            }
        }]
        reference      = "cloudwatch-team"
        root_reference = "payload"
      },
    ]
  }
}

## The `team` Alert Attribute we've configured to label Alerts and route alerts to schedules

data "incident_alert_attribute" "squad" {
  name = "Team"
}

## AWS Resources

resource "aws_sns_topic" "alerts" {
  name = "cloudwatch-alerts"
}

## SNS Topic Subscription that routes to the incident.io Alert Source created above

resource "aws_sns_topic_subscription" "incidentio_alert_source" {
  endpoint               = "https://api.incident.io/v2/alert_events/cloudwatch/${incident_alert_source.cloudwatch.id}?team=platform"
  endpoint_auto_confirms = true
  protocol               = "https"
  topic_arn              = aws_sns_topic.alerts.arn
}
