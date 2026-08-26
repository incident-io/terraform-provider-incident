# Look up a status that incident.io manages for you, by name.
data "incident_status" "triage" {
  name = "Triage"
}

# Create an additional closed status called "Clean-up".
resource "incident_status" "clean_up" {
  name        = "Clean-up"
  description = "Not yet fully finished, but isn't a live incident anymore."
  category    = "closed"
}

# Reference the status by its ID.
data "incident_status" "clean_up" {
  id = incident_status.clean_up.id
}
