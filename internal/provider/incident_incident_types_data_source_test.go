package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentIncidentTypesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentIncidentTypesDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check that we have at least 1 incident type
					resource.TestCheckResourceAttrWith("data.incident_incident_types.test", "incident_types.#", func(value string) error {
						if value == "0" {
							return fmt.Errorf("expected at least 1 incident type, got %s", value)
						}
						return nil
					}),
					// Check that incident types have required attributes
					resource.TestCheckResourceAttrSet("data.incident_incident_types.test", "incident_types.0.id"),
					resource.TestCheckResourceAttrSet("data.incident_incident_types.test", "incident_types.0.name"),
				),
			},
		},
	})
}

const testAccIncidentIncidentTypesDataSourceConfig = `
data "incident_incident_types" "test" {}
`
