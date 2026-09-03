## Sets no incident template at all. Both the inline template and the standalone
## incident_template reference are optional, so when neither is set the incidents
## this route creates use the organisation's default incident template.

resource "incident_alert_route" "default_template_alerts" {
  name       = "Default template alerts"
  enabled    = true
  is_private = false

  alert_sources = [
    {
      alert_source_id  = incident_alert_source.http.id
      condition_groups = []
    }
  ]

  condition_groups = []
  expressions      = []

  // grouping_config selects the current configuration format for this route.
  grouping_config = {
    default = {
      enabled        = true
      grouping_keys  = []
      window_seconds = 300
      window_type    = "rolling"
    }
  }

  message_config = {
    destinations = [
      {
        condition_groups = []
        slack_targets = {
          binding = {
            array_value = [
              {
                literal = "C01234567"
              }
            ]
          }
          channel_visibility   = "public"
          group_alerts_summary = true
        }
      }
    ]
  }

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = [
      {
        escalation_paths = {
          array_value = [
            {
              literal = "01JPQNFD3RWAAY2V83QQ80D1ZV"
            }
          ]
        }
      }
    ]
  }

  incident_config = {
    auto_decline_enabled = false
    enabled              = true
    condition_groups     = []

    // Neither template nor incident_template is set, so incidents this route
    // creates use the organisation's default incident template.
  }
}
