# Look up a status page by name, as it was created in the dashboard.
data "incident_status_page" "public" {
  name = "Our public status page"
}

# Or by ID, taken from the dashboard URL or the ListStatusPages endpoint.
data "incident_status_page" "customer" {
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

output "public_status_page_url" {
  value = data.incident_status_page.public.public_url
}
