# Look up a status that incident.io manages for you, by name.
data "incident_status" "triage" {
  name = "Triage"
}

# Create an additional status called "Clean-up". Only the live and learning categories
# can be added to: incident.io manages the rest, so they can't be customised.
resource "incident_status" "clean_up" {
  name        = "Clean-up"
  description = "Not yet fully finished, but isn't a live incident anymore."
  category    = "live"
}

# Reference the status by its ID.
data "incident_status" "clean_up" {
  id = incident_status.clean_up.id
}
