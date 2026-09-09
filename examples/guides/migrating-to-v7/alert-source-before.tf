# Before: the source and every attribute it populates are declared together,
# under one template.attributes list. Filling in one more attribute means
# rewriting that list, and two people editing different attributes are editing
# the same resource.
resource "incident_alert_source" "prometheus" {
  name        = "Prometheus"
  source_type = "http"

  owning_team_ids = [data.incident_catalog_entry.platform_team.id]

  template = {
    title = {
      literal = "{{payload.labels.alertname}} on {{payload.labels.service}}"
    }

    description = {
      literal = data.incident_rich_text.prometheus_alert.json
    }

    is_private = false

    attributes = [
      {
        alert_attribute_id = incident_alert_attribute.environment.id
        binding = {
          value = {
            literal = "production"
          }
          merge_strategy = "first_wins"
        }
      },
      {
        alert_attribute_id = incident_alert_attribute.regions.id
        binding = {
          array_value = [
            { literal = "eu-west-1" },
            { literal = "eu-west-2" },
          ]
          merge_strategy = "append"
        }
      },
    ]
  }
}
