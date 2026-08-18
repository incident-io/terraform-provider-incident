# Build a rich text document from markdown, for a field that stores a document rather
# than a "{{ }}" template. feature_set must match the field it's going into.
data "incident_rich_text" "alert_description" {
  markdown    = "**Alert**: {{payload.summary}}\n\nRunbook: {{payload.annotations.runbook_url}}"
  feature_set = "rich"
}

resource "incident_alert_source_beta" "prometheus" {
  name        = "Prometheus"
  source_type = "http"

  title = {
    literal = "{{payload.labels.alertname}}"
  }
  description = {
    literal = data.incident_rich_text.alert_description.json
  }
}

# A feature set that can't hold some of the markdown's formatting warns at plan time and
# drops the unsupported content, listing what was dropped.
output "dropped_content" {
  value = data.incident_rich_text.alert_description.dropped_content
}
