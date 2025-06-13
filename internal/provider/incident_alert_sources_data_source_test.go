package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentAlertSourcesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAlertSourcesDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_alert_sources.test", "alert_sources.#", "2"),
					resource.TestCheckResourceAttr("data.incident_alert_sources.test", "alert_sources.0.name", "Test HTTP Alert Source 1"),
					resource.TestCheckResourceAttr("data.incident_alert_sources.test", "alert_sources.1.name", "Test HTTP Alert Source 2"),
				),
			},
		},
	})
}

const testAccIncidentAlertSourcesDataSourceConfig = `
resource "incident_alert_source" "test1" {
  name        = "Test HTTP Alert Source 1"
  source_type = "http"
  template = {
    title = {
      literal = "Test Alert Title 1"
    }
    description = {
      literal = "Test Alert Description 1"
    }
  }
}

resource "incident_alert_source" "test2" {
  name        = "Test HTTP Alert Source 2"
  source_type = "http"
  template = {
    title = {
      literal = "Test Alert Title 2"
    }
    description = {
      literal = "Test Alert Description 2"
    }
  }
}

data "incident_alert_sources" "test" {
  source_type = "http"
  depends_on = [
    incident_alert_source.test1,
    incident_alert_source.test2
  ]
}
`
