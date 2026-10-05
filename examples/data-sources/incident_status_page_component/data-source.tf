# Look up a status page component by ID, such as one created in the dashboard.
data "incident_status_page_component" "api" {
  id = "01FCNDV6P870EA6S7TK1DSYDG1"
}

output "api_component_name" {
  value = data.incident_status_page_component.api.name
}
