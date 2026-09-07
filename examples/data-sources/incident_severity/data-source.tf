# Look up an existing incident severity by name.
data "incident_severity" "minor" {
  name = "Minor"
}

# Create an incident severity.
resource "incident_severity" "critical" {
  name        = "Critical"
  description = "A critical incident that requires an immediate response."
}

# Reference the incident severity by its ID.
data "incident_severity" "critical" {
  id = incident_severity.critical.id
}
