# Create a communications lead that is not required.
resource "incident_incident_role" "comms" {
  name         = "Communications Lead"
  description  = "Responsible for communications on behalf of the response team."
  instructions = "Manage internal and external communications on behalf of the response team."
  shortform    = "comms"
}

# Reference the incident role by its ID.
data "incident_incident_role" "comms" {
  id = incident_incident_role.comms.id
}
