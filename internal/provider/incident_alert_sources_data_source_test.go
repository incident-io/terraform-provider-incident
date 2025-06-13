package provider

import (
	"fmt"
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
					// Check that we have at least 2 alert sources (our created ones plus any existing ones)
					resource.TestCheckResourceAttrWith("data.incident_alert_sources.test", "alert_sources.#", func(value string) error {
						if value == "0" || value == "1" {
							return fmt.Errorf("expected at least 2 alert sources, got %s", value)
						}
						return nil
					}),
					// Check that our test alert sources exist in the results
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_sources.test", "alert_sources.*", map[string]string{
						"name":        "Test HTTP Alert Source 1",
						"source_type": "http",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_sources.test", "alert_sources.*", map[string]string{
						"name":        "Test HTTP Alert Source 2",
						"source_type": "http",
					}),
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
      literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Title 1\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
    }
    description = {
      literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Description 1\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
    }
    attributes = []
    expressions = []
  }
}

resource "incident_alert_source" "test2" {
  name        = "Test HTTP Alert Source 2"
  source_type = "http"
  template = {
    title = {
      literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Title 2\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
    }
    description = {
      literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Description 2\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
    }
    attributes = []
    expressions = []
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
