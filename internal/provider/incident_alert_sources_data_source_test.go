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
			// Test retrieving all alert sources
			{
				Config: testAccIncidentAlertSourcesDataSourceConfigAll,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.incident_alert_sources.all", "alert_sources.#"),
				),
			},
			// Test filtering by ID
			{
				Config: testAccIncidentAlertSourcesDataSourceConfigByID,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_alert_sources.by_id", "alert_sources.#", "1"),
					resource.TestCheckResourceAttrSet("data.incident_alert_sources.by_id", "alert_sources.0.id"),
					resource.TestCheckResourceAttr("data.incident_alert_sources.by_id", "alert_sources.0.name", "Test Alert Source for ID Filter"),
				),
			},
			// Test filtering by name
			{
				Config: testAccIncidentAlertSourcesDataSourceConfigByName,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_alert_sources.by_name", "alert_sources.#", "1"),
					resource.TestCheckResourceAttr("data.incident_alert_sources.by_name", "alert_sources.0.name", "Test Alert Source for Name Filter"),
				),
			},
			// Test filtering by source type
			{
				Config: testAccIncidentAlertSourcesDataSourceConfigBySourceType,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.incident_alert_sources.by_source_type", "alert_sources.#"),
					// All returned sources should be webhook type
					resource.TestCheckResourceAttr("data.incident_alert_sources.by_source_type", "alert_sources.0.source_type", "webhook"),
				),
			},
		},
	})
}

const testAccIncidentAlertSourcesDataSourceConfigAll = `
# Create test alert sources
resource "incident_alert_source" "test1" {
  name        = "Test Alert Source 1"
  source_type = "webhook"
  
  template {
    title {
      literal = "Test Alert 1"
    }
    description {
      literal = "Test alert description 1"
    }
    attributes = []
    expressions = []
  }
}

resource "incident_alert_source" "test2" {
  name        = "Test Alert Source 2"
  source_type = "webhook"
  
  template {
    title {
      literal = "Test Alert 2"
    }
    description {
      literal = "Test alert description 2"
    }
    attributes = []
    expressions = []
  }
}

# Get all alert sources
data "incident_alert_sources" "all" {
  depends_on = [incident_alert_source.test1, incident_alert_source.test2]
}
`

const testAccIncidentAlertSourcesDataSourceConfigByID = `
# Create test alert source
resource "incident_alert_source" "test" {
  name        = "Test Alert Source for ID Filter"
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

# Get alert source by ID
data "incident_alert_sources" "by_id" {
  id = incident_alert_source.test.id
}
`

const testAccIncidentAlertSourcesDataSourceConfigByName = `
# Create test alert source
resource "incident_alert_source" "test" {
  name        = "Test Alert Source for Name Filter"
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

# Get alert source by name
data "incident_alert_sources" "by_name" {
  name = incident_alert_source.test.name
}
`

const testAccIncidentAlertSourcesDataSourceConfigBySourceType = `
# Create test alert sources with different types
resource "incident_alert_source" "webhook" {
  name        = "Test Webhook Alert Source"
  source_type = "webhook"
  
  template {
    title {
      literal = "Test Webhook Alert"
    }
    description {
      literal = "Test webhook alert description"
    }
    attributes = []
    expressions = []
  }
}

# Get alert sources by source type
data "incident_alert_sources" "by_source_type" {
  source_type = "webhook"
  depends_on = [incident_alert_source.webhook]
}
`
