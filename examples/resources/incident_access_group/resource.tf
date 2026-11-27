# A custom access group. Add people to it in the dashboard, or map it to a group in your
# identity provider with SCIM: Terraform manages the group, not who is in it.
resource "incident_access_group" "platform" {
  name        = "Platform engineers"
  description = "Engineers who own our on-call and catalog setup"
}

# Give the group privileges with an access grant.
resource "incident_access_grant" "platform" {
  principal = {
    type = "access_group"
    id   = incident_access_group.platform.id
  }
  resource = {
    type = "organization"
  }
  privileges = [
    "catalog_types.create",
    "schedules.create",
  ]
}
