resource "incident_access_group" "responders" {
  name = "Incident responders"
}

# An access grant gives privileges to an access group across the whole organization.
resource "incident_access_grant" "responders" {
  principal = {
    type = "access_group"
    id   = incident_access_group.responders.id
  }
  resource = {
    type = "organization"
  }
  privileges = [
    "incidents.manage",
  ]
}

# You can also manage what the admin and standard base groups can do. Destroying one of
# these grants leaves the group's privileges as they are.
data "incident_access_group" "standard" {
  slug = "standard"
}

resource "incident_access_grant" "standard" {
  principal = {
    type = "access_group"
    id   = data.incident_access_group.standard.id
  }
  resource = {
    type = "organization"
  }
  privileges = [
    "incidents.manage",
    "schedule_overrides.manage",
  ]
}
