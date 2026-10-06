# The page itself is created in the dashboard. Its components can be managed here.
data "incident_status_page" "public" {
  name = "Our public status page"
}

resource "incident_status_page_component" "website" {
  name = "Website"
}

resource "incident_status_page_component" "api" {
  name = "API"
}

resource "incident_status_page_component" "database" {
  name = "Database"
}

# The structure is the page's layout: items appear in the order listed, each one either a
# single component or a named group of components. Display settings are optional; one left
# out keeps whatever the dashboard has.
resource "incident_status_page_structure" "public" {
  status_page_id = data.incident_status_page.public.id

  display_uptime_mode = "chart_and_percentage"

  items = [
    { component_id = incident_status_page_component.website.id },
    {
      group = {
        name        = "Backend"
        description = "The services behind the website"
        components = [
          { component_id = incident_status_page_component.api.id },
          {
            component_id   = incident_status_page_component.database.id
            display_uptime = false
          },
        ]
      }
    },
  ]
}
