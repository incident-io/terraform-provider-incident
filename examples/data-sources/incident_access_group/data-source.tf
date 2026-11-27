# Look up a base group by its slug: every organization has owner, admin and standard.
data "incident_access_group" "admin" {
  slug = "admin"
}

# Or look up any group by ID.
data "incident_access_group" "platform" {
  id = "01G0J1EXE7AXZ2C93K61WBPYEH"
}
