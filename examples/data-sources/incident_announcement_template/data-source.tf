# Look up an existing announcement template by name, such as the default template.
data "incident_announcement_template" "default" {
  name = "Default"
}

# Or by ID, to reference a template another module manages.
data "incident_announcement_template" "major_incidents" {
  id = "01G0J1EXE7AXZ2C93K61WBPYEH"
}

resource "incident_announcement_rule" "fyi" {
  name                = "FYI"
  slack_channel_ids   = ["C02AW36C1M5"]
  mode                = "live_and_closed"
  update_sharing_mode = "none"
  template_id         = data.incident_announcement_template.default.id

  condition_groups = [
    {
      conditions = [
        {
          subject        = "incident.status.category"
          operation      = "one_of"
          param_bindings = [{ array_value = [{ literal = "live" }] }]
        }
      ]
    }
  ]
}
