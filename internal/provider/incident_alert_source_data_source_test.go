package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentAlertSourceDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccIncidentAlertSourceDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.incident_alert_source.test", "id"),
					resource.TestCheckResourceAttrSet("data.incident_alert_source.test", "name"),
					resource.TestCheckResourceAttrSet("data.incident_alert_source.test", "source_type"),
					resource.TestCheckResourceAttrSet("data.incident_alert_source.test", "secret_token"),
				),
			},
		},
	})
}

const testAccIncidentAlertSourceDataSourceConfig = `
resource "incident_alert_source" "test" {
  name        = "Test Alert Source"
  source_type = "webhook"
  
  template {
    title {
      literal = "Test Alert"
    }
    description {
      literal = "Test alert description"
    }
    attributes = []
    expressions = []
  }
}

data "incident_alert_source" "test" {
  id = incident_alert_source.test.id
}
`
