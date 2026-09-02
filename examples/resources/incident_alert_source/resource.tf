## Create a basic Alert Source that receives from an SNS Topic in AWS

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
          ## Controls how the attribute value is handled when alert fires multiple times
          merge_strategy = "first_wins"
        }
      },

      ## An alert's priority is set here, as a binding on the built-in `Priority` Alert
      ## Attribute: this resource has no `priority` field of its own. An expression that
      ## returns an AlertPriority and is bound to nothing has no effect, so this entry is
      ## what makes the `cloudwatch-priority` expression below do anything at all.
      ##
      ## A priority binding takes no `merge_strategy`: an alert's priority is always
      ## overwritten when the alert fires again.
      {
        alert_attribute_id = data.incident_alert_attribute.priority.id
        binding = {
          value = {
            reference = "expressions[\"cloudwatch-priority\"]"
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

      ## Mapping the payload's severity onto an AlertPriority takes two expressions.
      ##
      ## The payload is opaque JSON: an expression reaches into it with a `parse`
      ## operation, and a condition can only ask whether `payload` as a whole is set. So
      ## `subject = "payload.severity"` resolves to nothing — pull the value out first.
      ##
      ## Step one: parse the severity out of the payload as a plain string.
      {
        label = "Severity"
        operations = [
          {
            operation_type = "parse"
            parse = {
              returns = {
                array = false
                type  = "String"
              }
              source = "$['severity']"
            }
        }]
        reference      = "cloudwatch-severity"
        root_reference = "payload"
      },

      ## Step two: map that string onto a priority.
      ##
      ## Parsing the severity straight into a CatalogEntry["AlertPriority"] instead would
      ## be shorter, but it resolves by matching the value against a priority's name, alias
      ## or external ID exactly. A payload saying "CRITICAL" matches no priority called
      ## "Urgent", so it resolves to nothing and every alert lands on the else_branch.
      ## Branching on the value says what you mean.
      {
        label = "Priority"
        operations = [
          {
            operation_type = "branches"
            branches = {
              returns = {
                array = false
                type  = "CatalogEntry[\"AlertPriority\"]"
              }
              branches = [
                {
                  condition_groups = [
                    {
                      conditions = [
                        {
                          subject   = "expressions[\"cloudwatch-severity\"]"
                          operation = "one_of"
                          param_bindings = [
                            { values = ["CRITICAL", "critical"] },
                          ]
                        },
                      ]
                    },
                  ]
                  result = { value_literal = data.incident_catalog_entry.urgent_priority.id }
                },
              ]
            }
        }]
        reference = "cloudwatch-priority"
        ## A branches operation reads the whole scope, so root_reference must be "."
        root_reference = "."
        ## What the priority is when no branch matched
        else_branch = {
          result = { value_literal = data.incident_catalog_entry.low_priority.id }
        }
      },
    ]
  }
}

## The `team` Alert Attribute we've configured to label Alerts and route alerts to schedules

data "incident_alert_attribute" "team" {
  name = "Team"
}

## `Priority` is a built-in Alert Attribute that every account already has, so it's looked
## up rather than declared: `incident_alert_attribute` rejects the name.

data "incident_alert_attribute" "priority" {
  name = "Priority"
}

## The priorities themselves are Catalog Entries, looked up by name

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
