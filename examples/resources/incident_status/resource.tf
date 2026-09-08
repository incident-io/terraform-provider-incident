# Statuses are ordered by rank within their category, lowest first. Give every status
# in a category a rank and the order is whatever the config says, rather than the order
# Terraform happened to create them in.
#
# Leaving gaps is worth doing: a status can then be slotted between two others without
# renumbering them.
resource "incident_status" "investigating" {
  name        = "Investigating"
  description = "We've spotted that something is wrong, but we're not sure why yet."
  category    = "live"
  rank        = 10
}

resource "incident_status" "clean_up" {
  name        = "Clean-up"
  description = "Not yet fully finished, but isn't a live incident anymore."
  category    = "live"
  rank        = 20
}
