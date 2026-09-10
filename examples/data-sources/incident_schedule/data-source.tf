# Look up a schedule by name...
data "incident_schedule" "platform" {
  name = "Platform on-call"
}

# ...or by ID, which is what you want if several schedules share a name.
data "incident_schedule" "platform_by_id" {
  id = "01ABC123DEF456GHI789JKL"
}
