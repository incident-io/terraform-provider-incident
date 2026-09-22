# Look up an existing pay config by name, such as one created in the dashboard and
# published in a report.
data "incident_pay_config" "platform" {
  name = "Platform on-call"
}

# Or by ID, to reference a config another module manages.
data "incident_pay_config" "flat_daily" {
  id = "01G0J1EXE7AXZ2C93K61WBPYEH"
}

output "platform_base_rate_cents" {
  value = data.incident_pay_config.platform.base_rate_cents
}
