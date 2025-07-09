package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentScheduleDataSource(t *testing.T) {
	defaultSchedule := incidentScheduleDefault("ONC-DataSource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentScheduleDataSourceConfig(incidentScheduleDataSourceFixture{
					Name:     defaultSchedule.Name,
					Timezone: defaultSchedule.Timezone,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check resource attributes
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "name", defaultSchedule.Name),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "timezone", defaultSchedule.Timezone),

					// Check data source lookup by ID
					resource.TestCheckResourceAttr(
						"data.incident_schedule.by_id", "name", defaultSchedule.Name),
					resource.TestCheckResourceAttr(
						"data.incident_schedule.by_id", "timezone", defaultSchedule.Timezone),
					resource.TestCheckResourceAttrSet(
						"data.incident_schedule.by_id", "id"),

					// Check data source lookup by name
					resource.TestCheckResourceAttr(
						"data.incident_schedule.by_name", "name", defaultSchedule.Name),
					resource.TestCheckResourceAttr(
						"data.incident_schedule.by_name", "timezone", defaultSchedule.Timezone),
					resource.TestCheckResourceAttrSet(
						"data.incident_schedule.by_name", "id"),

					// Check that both lookups return the same ID
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule.by_id", "id",
						"data.incident_schedule.by_name", "id"),
				),
			},
		},
	})
}

var incidentScheduleDataSourceTemplate = template.Must(template.New("incident_schedule_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_schedule" "example" {
  name     = {{ quote .Name }}
  timezone = {{ quote .Timezone }}
  rotations = [
    {
      id   = "rota-primary"
      name = "Primary Rota"
      versions = [
        {
          handover_start_at = "2024-04-26T16:00:00Z"
          handovers = [
            {
              interval      = 1
              interval_type = "weekly"
            }
          ]
          users = []
          layers = [
            {
              id   = "rota-primary-layer-one"
              name = "Primary Layer One"
            }
          ]
        }
      ]
    }
  ]
  holidays_public_config = {
    country_codes = ["GB", "FR"]
  }
}

data "incident_schedule" "by_id" {
  id = incident_schedule.example.id
}

data "incident_schedule" "by_name" {
  name = incident_schedule.example.name
}
`))

type incidentScheduleDataSourceFixture struct {
	Name     string
	Timezone string
}

func testAccIncidentScheduleDataSourceConfig(payload incidentScheduleDataSourceFixture) string {
	var buf bytes.Buffer
	if err := incidentScheduleDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(err)
	}
	return buf.String()
}
