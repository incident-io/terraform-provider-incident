package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentScheduleReplicasDataSource_Empty(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentScheduleReplicasDataSourceEmptyConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.incident_schedule_replicas.test", "schedule_id"),
					resource.TestCheckResourceAttr("data.incident_schedule_replicas.test", "schedule_replicas.#", "0"),
				),
			},
		},
	})
}

func testAccIncidentScheduleReplicasDataSourceEmptyConfig() string {
	return testRunTemplate("incident_schedule_replicas_empty", `
resource "incident_schedule" "test" {
  name     = {{ stableSuffix "Test Schedule for Replica List" | quote }}
  timezone = "Europe/London"
}

data "incident_schedule_replicas" "test" {
  schedule_id = incident_schedule.test.id
}
`, nil)
}
