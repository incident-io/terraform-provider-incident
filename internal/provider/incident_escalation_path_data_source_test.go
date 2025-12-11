package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentEscalationPathDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentEscalationPathDataSourceConfig(
					StableSuffix("EP DataSource Test"),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check resource attributes
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "name", StableSuffix("EP DataSource Test")),

					// Check data source lookup by ID returns the same values
					resource.TestCheckResourceAttrPair(
						"data.incident_escalation_path.by_id", "id",
						"incident_escalation_path.example", "id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_escalation_path.by_id", "name",
						"incident_escalation_path.example", "name"),

					// Check data source lookup by name returns the same values
					resource.TestCheckResourceAttrPair(
						"data.incident_escalation_path.by_name", "id",
						"incident_escalation_path.example", "id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_escalation_path.by_name", "name",
						"incident_escalation_path.example", "name"),

					// Check that both lookups return the same ID
					resource.TestCheckResourceAttrPair(
						"data.incident_escalation_path.by_id", "id",
						"data.incident_escalation_path.by_name", "id"),

					// Verify nested path attributes are returned
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.id", "start"),
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.type", "if_else"),
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.if_else.conditions.0.operation", "is_active"),

					// Verify working hours are returned
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "working_hours.0.id", "UK"),
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "working_hours.0.name", "UK"),
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "working_hours.0.timezone", "Europe/London"),
				),
			},
		},
	})
}

var escalationPathDataSourceTemplate = template.Must(template.New("incident_escalation_path_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
# This is the _official_ team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# This is a team catalog entry
resource "incident_catalog_entry" "terraform" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id = "tf-acceptance-test-ds"
  name = "Terraform test team (data source)"
  attribute_values = []
  managed_attributes = []
}

# This is the primary schedule that receives pages in working hours.
resource "incident_schedule" "primary_on_call" {
  name = {{ quote .ScheduleName }}
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [
      {
        handover_start_at = "2024-05-01T12:00:00Z"
        users = []
        layers = [
          {
            id   = "primary"
            name = "Primary"
          }
        ]
        handovers = [
          {
            interval_type = "daily"
            interval      = 1
          }
        ]
      },
    ]
  }]

  # Teams that use this schedule
  team_ids = [incident_catalog_entry.terraform.id]
}

# Escalation path resource
resource "incident_escalation_path" "example" {
  name = {{ quote .PathName }}

  path = [
    {
      id = "start"
      type = "if_else"
      if_else = {
        conditions = [
          {
            operation = "is_active",
            param_bindings = []
            subject = "escalation.working_hours[\"UK\"]"
          }
        ]
        then_path = [
          {
            type = "level"
            level = {
              targets = [{
                type    = "schedule"
                id      = incident_schedule.primary_on_call.id
                urgency = "high"
              }]
              time_to_ack_seconds = 300
            }
          },
          {
            type = "repeat"
            repeat = {
              repeat_times = 3
              to_node = "start"
            }
          }
        ]
        else_path = [
          {
            type = "notify_channel"
            notify_channel = {
              targets = [{
                type    = "slack_channel"
                id      = {{ quote .ChannelID }}
                urgency = "low"
              }]
              time_to_ack_seconds = 300
            }
          },
          {
            type = "level"
            level = {
              targets = [{
                type    = "schedule"
                id      = incident_schedule.primary_on_call.id
                urgency = "low"
              }]
              time_to_ack_seconds = 300
            }
          }
        ]
      }
    }
  ]

  working_hours = [
    {
      id = "UK"
      name = "UK"
      timezone = "Europe/London"
      weekday_intervals = [
        {
          weekday    = "monday"
          start_time = "09:00"
          end_time   = "17:00"
        }
      ]
    }
  ]

  team_ids = [incident_catalog_entry.terraform.id]
}

# Data source to look up the escalation path by ID
data "incident_escalation_path" "by_id" {
  id = incident_escalation_path.example.id
}

# Data source to look up the escalation path by name
data "incident_escalation_path" "by_name" {
  name = incident_escalation_path.example.name
}
`))

func testAccIncidentEscalationPathDataSourceConfig(name string) string {
	model := struct {
		ScheduleName string
		PathName     string
		ChannelID    string
		TeamTypeName string
	}{
		ScheduleName: name + " Schedule",
		PathName:     name,
		ChannelID:    channelID(false),
		TeamTypeName: teamTypeName(),
	}

	var buf bytes.Buffer
	if err := escalationPathDataSourceTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
