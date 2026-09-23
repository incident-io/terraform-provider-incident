data "incident_severity" "critical" {
  name = "Critical"
}

# Announce every critical incident, triage included, in #major-incidents, rendered with
# the major incidents template. Updates are shared in the announcement's thread.
resource "incident_announcement_rule" "critical" {
  name                = "Critical incidents"
  slack_channel_ids   = ["C02AW36C1M5"]
  mode                = "include_triage"
  update_sharing_mode = "thread"
  template_id         = incident_announcement_template.major_incidents.id

  condition_groups = [
    {
      conditions = [
        {
          subject        = "incident.severity"
          operation      = "one_of"
          param_bindings = [{ array_value = [{ literal = data.incident_severity.critical.id }] }]
        }
      ]
    }
  ]
}
