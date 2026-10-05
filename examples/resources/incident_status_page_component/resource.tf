# A component is something a status page reports the status of. It belongs to your
# organisation rather than to a page. Each page's structure in the dashboard decides which
# pages show it, so creating one here doesn't put it on a page yet.
resource "incident_status_page_component" "api" {
  name        = "API"
  description = "The public REST API"
}

# The description is optional. Drop the attribute to clear one.
resource "incident_status_page_component" "dashboard" {
  name = "Dashboard"
}

# Terraform claims what it manages, so a component can't be edited in the dashboard. Set
# unlock_in_dashboard to leave it editable there, and ignore the attributes people change.
resource "incident_status_page_component" "edited_by_hand" {
  name                = "Email delivery"
  unlock_in_dashboard = true

  lifecycle {
    ignore_changes = [description]
  }
}
