resource "incident_alert_route" "service_alerts" {
  name       = "Testing Alert Routes"
  enabled    = true
  is_private = false

  // This references the ID of an alert source, condition groups are
  // used to specify the conditions under which the alert route will be triggered on
  // an alert source-basis
  alert_sources = [
    {
      alert_source_id = incident_alert_source.http.id
      condition_groups = [
        {
          conditions = [
            {
              subject        = "alert.title"
              operation      = "is_set"
              param_bindings = []
            }
          ]
        }
      ]
    }
  ]

  // The condition groups are used to specify the conditions under which the alert route will be triggered
  // on a whole-route basis
  condition_groups = [
    {
      conditions = [
        {
          subject        = "alert.title"
          operation      = "is_set"
          param_bindings = []
        }
      ]
    }
  ]

  expressions = []

  // Setting grouping_config opts this alert route into the v3 schema. Grouping
  // controls how alerts are combined together; in the v3 schema it is configured
  // here rather than on incident_config.
  grouping_config = {
    default = {
      enabled = true
      // grouping_keys is an array of { reference = "alert.title" } which specifies
      // the keys used to group alerts together
      grouping_keys  = []
      window_seconds = 300
      // window_type is one of "rolling" (extends as new alerts attach) or "fixed"
      window_type = "rolling"
    }
  }

  // Used to configure which Slack channels or Microsoft Teams teams should
  // be notified when an alert is received, and the (optional) template applied
  // to those alert messages.
  message_config = {
    destinations = [
      {
        // Define conditions under which this destination notification should occur
        condition_groups = [
          {
            conditions = [
              {
                subject   = "alert.title"
                operation = "contains"
                param_bindings = [
                  {
                    value = {
                      literal = "critical"
                    }
                  }
                ]
              }
            ]
          }
        ]

        // Configure Slack channel notifications - set either slack_targets OR ms_teams_targets
        slack_targets = {
          // Define channels to notify, either with literal channel IDs or dynamic references
          binding = {
            array_value = [
              {
                literal = "C01234567" // Slack channel ID
              }
            ]
          }
          channel_visibility = "public"
        }
      }
    ]

    // Optionally customize how alert messages appear in your communications platform.
    // You can express the template ID using a catalog entry data source, e.g.
    // data.incident_catalog_entry.my_message_template.id
    template = {
      value = {
        literal = "01KHJTJR4FZJJZQ6G02EBFJAAY"
      }
    }
  }

  // Used to configure which escalation paths and/or users who should be notified when an alert is received
  // and the conditions under which they should be notified.
  // auto_cancel_escalations is used to specify whether the escalation should be automatically cancelled
  // when the alert that triggered the escalation is resolved
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = [
      {
        escalation_paths = {
          array_value = [
            {
              reference = "alert.attributes.escalation_path"
            },
          ]
        }
      },
      {
        escalation_paths = {
          array_value = [
            {
              literal = "01JPQNFD3RWAAY2V83QQ80D1ZV"
            }
          ]
        }
      },
      {
        users = {
          array_value = [
            {
              literal = "01GX3C1TK13RQSEGP59XZ3MYP0"
            }
          ]
        }
      }
    ]

    // Optionally control whether (and how) escalations fire again when a
    // subsequent alert joins an existing group.
    when_alert_joins_group = {
      mode                 = "on_each_new_alert"
      grace_period_seconds = 60
    }
  }

  // Used to configure the incident creation settings for the alert route
  // auto_decline_enabled is used to specify whether triage incidents should be automatically declined
  // when the alert that triggered the incident is resolved
  // enabled is used to specify whether or not incidents should be created
  // condition_groups is used to specify the conditions under which the incident should be created
  incident_config = {
    auto_decline_enabled = false
    enabled              = true
    condition_groups     = []

    // The incident template configures how created incidents are populated. In
    // the v3 schema the template is nested under incident_config.
    template = {
      // custom_fields is used to specify the custom fields that should be set on the incident
      // when it is created, the merge_strategy is used to specify how the custom field should be modified
      // when a new alert is received for the incident
      custom_fields = [
        {
          custom_field_id = incident_custom_field.type_field.id
          merge_strategy  = "first-wins"
          binding = {
            value = {
              literal = "Test incident"
            }
          }
        }
      ]

      name = {
        autogenerated = true
        value = {
          literal = jsonencode({
            content = [
              {
                content = [
                  {
                    attrs = {
                      label   = "Alert → Title"
                      missing = false
                      name    = "alert.title"
                    }
                    type = "varSpec"
                  }
                ]
                type = "paragraph"
              }
            ]
            type = "doc"
          })
        }
      }
      summary = {
        autogenerated = true
        value = {
          literal = jsonencode({
            content = [
              {
                content = [
                  {
                    attrs = {
                      label   = "Alert → Description"
                      missing = false
                      name    = "alert.description"
                    }
                    type = "varSpec"
                  }
                ]
                type = "paragraph"
              }
            ]
            type = "doc"
          })
        }
      }
      start_in_triage = {
        value = {
          literal = "true"
        }
      }
      severity = {
        merge_strategy = "first-wins"
      }
    }
  }
}
