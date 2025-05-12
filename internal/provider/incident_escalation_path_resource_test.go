package provider

import (
	"bytes"
	"os"
	"regexp"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
						"incident_escalation_path.example", "path.0.if_else.else_path.0.type", "notify_channel"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.0.notify_channel.targets.0.type", "slack_channel"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.0.notify_channel.targets.0.urgency", "low"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.0.notify_channel.time_to_ack_seconds", "300"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.1.type", "level"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.1.level.targets.0.type", "schedule"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.1.level.targets.0.urgency", "low"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.1.level.time_to_ack_seconds", "300"),
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
					resource.TestCheckResourceAttrPair(
						"incident_escalation_path.example", "team_ids.0", "incident_catalog_entry.terraform", "id"),
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

// TestAccIncidentEscalationPathTeamIDs specifically tests the handling of team_ids
// in empty and nil states
func TestAccIncidentEscalationPathTeamIDs(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Test with empty team_ids (explicitly set to [])
			{
				Config: testAccIncidentEscalationPathResourceWithTeamIDs(
					StableSuffix("Empty TeamIDs Test"),
					"empty", // Use empty team_ids
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "name", StableSuffix("Empty TeamIDs Test")),
					// Verify that team_ids is an empty set but not nil
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "team_ids.#", "0"),
				),
			},
			// Test with team_ids not specified at all (omitted from config)
			{
				Config: testAccIncidentEscalationPathResourceWithTeamIDs(
					StableSuffix("Omitted TeamIDs Test"),
					"omit", // Completely omit team_ids field
				),
				// When omitted, the API should error as we have team settings
				// Annoyingly Terraform returns this with indent, so this is
				// the subset we match on.
				ExpectError: regexp.MustCompile("must set an empty slice or a list of Team"),
			},
		},
	})
}

var escalationPathTemplate = template.Must(template.New("incident_escalation_path").Funcs(sprig.TxtFuncMap()).Parse(`
# This is the _official_ team catalog type
# This means our test will only work in Github, you'll need to point this to your local
# Team type!
# Same as the Slack channel used here.
data "incident_catalog_type" "team" {
  name            = {{ quote .TeamTypeName }}
}

# This is a team catalog entry
resource "incident_catalog_entry" "terraform" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id = "tf-acceptance-test"
  name = "Terraform test team"
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
            type = "notify_channel"
            notify_channel = {
              targets = [{
               type    = "slack_channel"
               id      = {{ quote .ChannelID }}
               urgency  = "low"
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

  # Teams that use this escalation path
  {{- if eq .TeamIDsType "normal" }}
  team_ids = [incident_catalog_entry.terraform.id]
  {{- else if eq .TeamIDsType "empty" }}
  team_ids = []
  {{- end }}
}
`))

func testAccIncidentEscalationPathResourceConfig(name string) string {
	return testAccIncidentEscalationPathResourceWithTeamIDs(name, "normal")
}

func testAccIncidentEscalationPathResourceWithTeamIDs(name string, teamIDsType string) string {
	model := struct {
		ScheduleName string
		PathName     string
		TeamIDsType  string
		ChannelID    string
		TeamTypeName string
	}{
		ScheduleName: name,
		PathName:     name,
		TeamIDsType:  teamIDsType, // "normal", "empty", or "omit"
		ChannelID:    channelID(),
		TeamTypeName: teamTypeName(),
	}

	var buf bytes.Buffer
	if err := escalationPathTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}

func teamTypeName() string {
	if os.Getenv("CI") == "true" {
		// This is a type that exists in our test workspace
		return "Team"
	}
	// Override the team type name for local testing
	if teamType := os.Getenv("TF_TEAM_TYPE_NAME"); teamType != "" {
		return teamType
	}

	return "Team"
}
