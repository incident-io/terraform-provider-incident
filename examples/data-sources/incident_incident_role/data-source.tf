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

# Or look one up by name, which avoids pinning an ID that differs between
# workspaces. This is how you get hold of the roles incident.io creates for you,
# such as Incident Lead.
data "incident_incident_role" "incident_lead" {
  name = "Incident Lead"
}
