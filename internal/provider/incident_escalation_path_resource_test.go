package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentEscalationPathResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentEscalationPathResourceConfig(
					StableSuffix("Terraform EP tests"),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "name", StableSuffix("Terraform EP tests")),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.id", "start"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.type", "if_else"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.conditions.0.operation", "is_active"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.0.type", "level"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.0.level.targets.0.type", "schedule"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.0.level.targets.0.urgency", "high"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.0.level.time_to_ack_seconds", "300"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.1.type", "repeat"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.1.repeat.repeat_times", "3"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.1.repeat.to_node", "start"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.0.type", "level"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.0.level.targets.0.type", "schedule"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.0.level.targets.0.urgency", "low"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.0.level.time_to_ack_seconds", "300"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "working_hours.0.id", "UK"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "working_hours.0.name", "UK"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "working_hours.0.timezone", "Europe/London"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "working_hours.0.weekday_intervals.0.weekday", "monday"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "working_hours.0.weekday_intervals.0.start_time", "09:00"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "working_hours.0.weekday_intervals.0.end_time", "17:00"),
				),
			},
			// Import
			{
				ResourceName:      "incident_escalation_path.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

var escalationPathTemplate = template.Must(template.New("incident_escalation_path").Funcs(sprig.TxtFuncMap()).Parse(`
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
}

# If in working hours, send high-urgency alerts. Otherwise use low-urgency.
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
                urgency  = "high"
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
            type = "level"
            level = {
              targets = [{
                type    = "schedule"
                id      = incident_schedule.primary_on_call.id
                urgency  = "low"
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
}
`))

func testAccIncidentEscalationPathResourceConfig(name string) string {
	model := struct {
		ScheduleName string
		PathName     string
	}{
		ScheduleName: name,
		PathName:     name,
	}

	var buf bytes.Buffer
	if err := escalationPathTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
