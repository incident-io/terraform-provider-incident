package provider

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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
						"incident_escalation_path.example", "path.0.if_else.then_path.1.type", "delay"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.1.delay.delay_seconds", "120"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.2.type", "repeat"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.2.repeat.repeat_times", "3"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.then_path.2.repeat.to_node", "start"),
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
						"incident_escalation_path.example", "path.0.if_else.else_path.2.type", "delay"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.2.delay.delay_interval_condition", "active"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.2.delay.delay_weekday_interval_config_id", "UK"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.3.type", "repeat"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.3.repeat.repeat_times", "3"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "path.0.if_else.else_path.3.repeat.to_node", "start"),
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
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "repeat_config.repeat_after_seconds", "1800"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.example", "repeat_config.delay_repeat_on_activity", "true"),
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

var escalationPathTemplate = template.Must(template.New("incident_escalation_path").Funcs(testTemplateFuncs()).Parse(`
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
  external_id = {{ stableSuffix "tf-acceptance-test" | quote }}
  name = {{ stableSuffix "Terraform test team" | quote }}
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
            type = "delay"
            delay = {
              delay_seconds = 120
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
          },
          {
            type = "delay"
            delay = {
              delay_interval_condition         = "active"
              delay_weekday_interval_config_id = "UK"
            }
          },
          {
            type = "repeat"
            repeat = {
              repeat_times = 3
              to_node      = "start"
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

  repeat_config = {
    repeat_after_seconds     = 1800
    delay_repeat_on_activity = true
  }

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
		ChannelID:    channelID(false),
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

func TestAccIncidentEscalationPathMaxDepth(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// A path nested to the maximum supported depth (5 if_else levels)
			// applies cleanly.
			{
				Config: testAccIncidentEscalationPathNestedConfig(5),
				Check: resource.TestCheckResourceAttr(
					"incident_escalation_path.example", "name", StableSuffix("Deeply Nested Path Test")),
			},
		},
	})
}

func TestAccIncidentEscalationPathExceedsMaxDepth(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Nesting one level beyond the maximum (6 if_else levels) is rejected
			// at plan time with a clear validation error, rather than slipping
			// through to fail at apply with an opaque API error.
			{
				Config:      testAccIncidentEscalationPathNestedConfig(6),
				ExpectError: regexp.MustCompile(`if_else nodes can be nested at most 5 levels deep`),
			},
		},
	})
}

// testAccIncidentEscalationPathNestedConfig builds an escalation path whose path
// is a single chain of ifElseLevels nested if_else nodes terminating in a level
// node. Used to exercise the maximum supported nesting depth and one level
// beyond it.
func testAccIncidentEscalationPathNestedConfig(ifElseLevels int) string {
	node := `
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
                }`

	// Wrap the leaf in (ifElseLevels-1) if_else nodes; the outermost path node
	// below is the final if_else, giving ifElseLevels in total.
	for i := 0; i < ifElseLevels-1; i++ {
		node = fmt.Sprintf(`
                {
                  type = "if_else"
                  if_else = {
                    conditions = [
                      {
                        operation      = "is_active"
                        param_bindings = []
                        subject        = "escalation.working_hours[\"UK\"]"
                      }
                    ]
                    then_path = [%[1]s
                    ]
                    else_path = []
                  }
                }`, node)
	}

	return fmt.Sprintf(`
resource "incident_schedule" "primary_on_call" {
  name     = %[2]q
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"
    versions = [
      {
        handover_start_at = "2024-05-01T12:00:00Z"
        users             = []
        layers            = [{ id = "primary", name = "Primary" }]
        handovers         = [{ interval_type = "daily", interval = 1 }]
      },
    ]
  }]
  team_ids = []
}

resource "incident_escalation_path" "example" {
  name = %[3]q

  path = [
    {
      id   = "start"
      type = "if_else"
      if_else = {
        conditions = [
          {
            operation      = "is_active"
            param_bindings = []
            subject        = "escalation.working_hours[\"UK\"]"
          }
        ]
        then_path = [%[1]s
        ]
        else_path = []
      }
    }
  ]

  working_hours = [
    {
      id       = "UK"
      name     = "UK"
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

  team_ids = []
}
`, node, StableSuffix("Deep Nesting Test Schedule"), StableSuffix("Deeply Nested Path Test"))
}

func TestAccIncidentEscalationPathSelectedRotaID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentEscalationPathResourceConfigWithSelectedRotaID(
					StableSuffix("EP rota-mode"),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_escalation_path.rota_modes", "path.0.level.targets.0.schedule_mode", "currently_on_call_for_rota"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.rota_modes", "path.0.level.targets.0.selected_rota_id", "primary"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.rota_modes", "path.0.level.targets.1.schedule_mode", "next_on_call"),
					resource.TestCheckNoResourceAttr(
						"incident_escalation_path.rota_modes", "path.0.level.targets.1.selected_rota_id"),
				),
			},
			{
				ResourceName:      "incident_escalation_path.rota_modes",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccIncidentEscalationPathResourceConfigWithSelectedRotaID(name string) string {
	return fmt.Sprintf(`
data "incident_catalog_type" "team" {
  name = %[1]q
}

resource "incident_catalog_entry" "terraform_rota_modes" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = %[4]q
  name               = %[4]q
  attribute_values   = []
  managed_attributes = []
}

resource "incident_schedule" "rota_modes" {
  name     = %[2]q
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"
    versions = [{
      handover_start_at = "2024-05-01T12:00:00Z"
      users             = []
      layers = [{
        id   = "primary"
        name = "Primary"
      }]
      handovers = [{
        interval_type = "daily"
        interval      = 1
      }]
    }]
  }]
  team_ids = [incident_catalog_entry.terraform_rota_modes.id]
}

resource "incident_escalation_path" "rota_modes" {
  name = %[3]q

  path = [
    {
      type = "level"
      level = {
        targets = [
          {
            type             = "schedule"
            id               = incident_schedule.rota_modes.id
            urgency          = "high"
            schedule_mode    = "currently_on_call_for_rota"
            selected_rota_id = "primary"
          },
          {
            type          = "schedule"
            id            = incident_schedule.rota_modes.id
            urgency       = "high"
            schedule_mode = "next_on_call"
          },
        ]
        time_to_ack_seconds = 300
      }
    }
  ]

  team_ids = [incident_catalog_entry.terraform_rota_modes.id]
}
`, teamTypeName(), name, name, StableSuffix("tf-acceptance-test-rota-modes"))
}

func TestAccIncidentEscalationPathRetryConfig(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read a level that pages more than once via retry_config.
			{
				Config: testAccIncidentEscalationPathResourceConfigWithRetryConfig(
					StableSuffix("EP retry-config"),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_escalation_path.retries", "path.0.level.retry_config.attempts", "3"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.retries", "path.0.level.retry_config.interval_seconds", "60"),
				),
			},
			{
				ResourceName:      "incident_escalation_path.retries",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccIncidentEscalationPathResourceConfigWithRetryConfig(name string) string {
	return fmt.Sprintf(`
data "incident_catalog_type" "team" {
  name = %[1]q
}

resource "incident_catalog_entry" "terraform_retries" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = %[4]q
  name               = %[4]q
  attribute_values   = []
  managed_attributes = []
}

resource "incident_schedule" "retries" {
  name     = %[2]q
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"
    versions = [{
      handover_start_at = "2024-05-01T12:00:00Z"
      users             = []
      layers = [{
        id   = "primary"
        name = "Primary"
      }]
      handovers = [{
        interval_type = "daily"
        interval      = 1
      }]
    }]
  }]
  team_ids = [incident_catalog_entry.terraform_retries.id]
}

resource "incident_escalation_path" "retries" {
  name = %[3]q

  path = [
    {
      type = "level"
      level = {
        targets = [{
          type    = "schedule"
          id      = incident_schedule.retries.id
          urgency = "high"
        }]
        time_to_ack_seconds = 300

        retry_config = {
          attempts         = 3
          interval_seconds = 60
        }
      }
    }
  ]

  team_ids = [incident_catalog_entry.terraform_retries.id]
}
`, teamTypeName(), name, name, StableSuffix("tf-acceptance-test-retries"))
}

func TestAccIncidentEscalationPathSelectedRotaIDValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// schedule_mode requires selected_rota_id but it is missing
			{
				Config:      testAccIncidentEscalationPathResourceConfigMissingSelectedRotaID(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Missing selected_rota_id`),
			},
			// schedule_mode does not allow selected_rota_id but it is set
			{
				Config:      testAccIncidentEscalationPathResourceConfigUnexpectedSelectedRotaID(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Unexpected selected_rota_id`),
			},
		},
	})
}

// TestAccIncidentEscalationPathTargetValidation exercises the plan-time validate call
// itself: a target referencing a user ID that doesn't exist isn't something this provider
// checks locally (unlike the schedule_mode/selected_rota_id pairing above, which is a
// ValidateConfig diagnostic and never reaches the API), so a rejection here only happens
// once ModifyPlan calls the API's validate endpoint. PlanOnly proves it fails during plan,
// before Terraform would otherwise create anything.
func TestAccIncidentEscalationPathTargetValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccIncidentEscalationPathResourceConfigNonexistentUser(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`does not exist`),
			},
		},
	})
}

func testAccIncidentEscalationPathResourceConfigNonexistentUser() string {
	return `
resource "incident_escalation_path" "invalid_target" {
  name = "invalid-target"

  path = [
    {
      type = "level"
      level = {
        targets = [{
          type    = "user"
          id      = "01HKZWAAAAAAAAAAAAAAAAAAA1"
          urgency = "high"
        }]
        time_to_ack_seconds = 300
      }
    }
  ]

  team_ids = []
}`
}

func testAccIncidentEscalationPathResourceConfigMissingSelectedRotaID() string {
	return `
resource "incident_escalation_path" "invalid_missing_rota" {
  name = "invalid-missing-rota"

  path = [
    {
      type = "level"
      level = {
        targets = [{
          type          = "schedule"
          id            = "01HKZWAAAAAAAAAAAAAAAAAAA1"
          urgency       = "high"
          schedule_mode = "currently_on_call_for_rota"
        }]
        time_to_ack_seconds = 300
      }
    }
  ]
}`
}

func testAccIncidentEscalationPathResourceConfigUnexpectedSelectedRotaID() string {
	return `
resource "incident_escalation_path" "invalid_unexpected_rota" {
  name = "invalid-unexpected-rota"

  path = [
    {
      type = "level"
      level = {
        targets = [{
          type             = "schedule"
          id               = "01HKZWAAAAAAAAAAAAAAAAAAA1"
          urgency          = "high"
          schedule_mode    = "currently_on_call"
          selected_rota_id = "primary"
        }]
        time_to_ack_seconds = 300
      }
    }
  ]
}`
}

// TestAccIncidentEscalationPathUnknownValues is a regression test for ONC-11917.
//
// It reproduces the two configurations that previously crashed at plan time
// with "Received unknown value, however the target type cannot handle unknown
// values ... Target Type: []provider.IncidentEscalationPathNode". The crash
// happened because the resource model stored path/targets/working_hours as
// plain Go slices, which cannot represent the unknown values that Terraform
// produces during planning when those values derive from computed attributes
// or are constructed via HCL expressions (locals, indexing, etc.).
//
// Both steps below force the whole `path` (and a nested target id /
// working_hours config id) to be unknown at plan time:
//   - the path is read from a `local` indexed by a variable, mirroring the
//     customer's `local.path_templates[var.path_template]`, and
//   - a target id references the computed id of an incident_schedule resource,
//     so it is "known after apply".
//
// With the types.List-based model these plan cleanly and converge.
func TestAccIncidentEscalationPathUnknownValues(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read: the whole path is built from a local indexed by a
			// variable, and the target id is a computed schedule id (known after
			// apply). Both made the old slice-based model crash at plan time.
			{
				Config: testAccIncidentEscalationPathResourceConfigUnknownValues(
					StableSuffix("EP unknown values"),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_escalation_path.unknown_values", "name", StableSuffix("EP unknown values")),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.unknown_values", "path.0.type", "level"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.unknown_values", "path.0.level.targets.0.type", "schedule"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.unknown_values", "path.0.level.targets.0.urgency", "high"),
					// The target id resolves to the computed schedule id.
					resource.TestCheckResourceAttrPair(
						"incident_escalation_path.unknown_values", "path.0.level.targets.0.id",
						"incident_schedule.unknown_values", "id"),
					resource.TestCheckResourceAttr(
						"incident_escalation_path.unknown_values", "working_hours.0.id", "UK"),
				),
			},
			// Import/refresh: confirm existing state reads back cleanly. A
			// ListNestedAttribute already serialises as a list-of-objects in
			// state, identical to the old []struct encoding, so no schema-version
			// bump or state upgrader is required.
			{
				ResourceName:      "incident_escalation_path.unknown_values",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccIncidentEscalationPathResourceConfigUnknownValues builds a config
// where the escalation path is assembled from a local indexed by a variable,
// and where a target id and a working_hours config id reference values that are
// only known after apply.
func testAccIncidentEscalationPathResourceConfigUnknownValues(name string) string {
	return fmt.Sprintf(`
data "incident_catalog_type" "team" {
  name = %[1]q
}

resource "incident_catalog_entry" "terraform_unknown_values" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = %[4]q
  name               = %[4]q
  attribute_values   = []
  managed_attributes = []
}

resource "incident_schedule" "unknown_values" {
  name     = %[2]q
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"
    versions = [{
      handover_start_at = "2024-05-01T12:00:00Z"
      users             = []
      layers = [{
        id   = "primary"
        name = "Primary"
      }]
      handovers = [{
        interval_type = "daily"
        interval      = 1
      }]
    }]
  }]
  team_ids = [incident_catalog_entry.terraform_unknown_values.id]
}

# Mirrors the customer's local.path_templates[var.path_template]: the path is
# selected from a map keyed by a variable, so the resource only learns the
# concrete path during apply. The nested target id and the working_hours config
# id reference the computed schedule id, making them "known after apply".
variable "path_template" {
  type    = string
  default = "default"
}

locals {
  path_templates = {
    default = [
      {
        type = "level"
        level = {
          targets = [{
            type    = "schedule"
            id      = incident_schedule.unknown_values.id
            urgency = "high"
          }]
          time_to_ack_seconds = 300
        }
      },
    ]
  }
}

resource "incident_escalation_path" "unknown_values" {
  name = %[3]q

  path = local.path_templates[var.path_template]

  working_hours = [
    {
      id       = "UK"
      name     = "UK"
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

  team_ids = [incident_catalog_entry.terraform_unknown_values.id]
}
`, teamTypeName(), name, name, StableSuffix("tf-acceptance-test-unknown-values"))
}

// TestAccIncidentEscalationPathReassignment covers the two things the model round-trip
// tests can't: that a second plan over an unchanged config is a no-op, and that an
// imported path matches the config it was written from. Both turn on the read, which is
// where a reassignment node used to come back with an empty block and take the next apply
// down with it.
//
// The node needs feature-escalation-path-reassignment enabled for the organisation the API
// key belongs to. That's a server-side flag with no provider-side equivalent, so like the
// other tests for org-gated features this skips unless it's been asked for explicitly.
func TestAccIncidentEscalationPathReassignment(t *testing.T) {
	if os.Getenv("TF_ACC_ESCALATION_PATH_REASSIGNMENT") == "" {
		t.Skip("TF_ACC_ESCALATION_PATH_REASSIGNMENT is not set: skipping test that needs the escalation path reassignment feature enabled")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a path whose last node hands the escalation to another path, and read
			// it back.
			{
				Config: testAccIncidentEscalationPathResourceConfigWithReassignment(
					StableSuffix("EP reassignment"),
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_escalation_path.reassigning", "path.1.type", "escalation_path"),
					resource.TestCheckResourceAttrPair(
						"incident_escalation_path.reassigning", "path.1.escalation_path.escalation_path_id",
						"incident_escalation_path.fallback", "id"),
				),
				// The drift check: a node that reads back empty plans a change here, which
				// is what the old behaviour did on every apply after the first.
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "incident_escalation_path.reassigning",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccIncidentEscalationPathResourceConfigWithReassignment(name string) string {
	return fmt.Sprintf(`
data "incident_catalog_type" "team" {
  name = %[1]q
}

resource "incident_catalog_entry" "terraform_reassignment" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = %[4]q
  name               = %[4]q
  attribute_values   = []
  managed_attributes = []
}

resource "incident_schedule" "reassignment" {
  name     = %[2]q
  timezone = "Europe/London"
  rotations = [{
    id   = "primary"
    name = "Primary"
    versions = [{
      handover_start_at = "2024-05-01T12:00:00Z"
      users             = []
      layers = [{
        id   = "primary"
        name = "Primary"
      }]
      handovers = [{
        interval_type = "daily"
        interval      = 1
      }]
    }]
  }]
  team_ids = [incident_catalog_entry.terraform_reassignment.id]
}

# The path the reassignment hands over to. It continues from this path's first node.
resource "incident_escalation_path" "fallback" {
  name = "%[3]s fallback"

  path = [
    {
      type = "level"
      level = {
        targets = [{
          type    = "schedule"
          id      = incident_schedule.reassignment.id
          urgency = "high"
        }]
        time_to_ack_seconds = 300
      }
    }
  ]

  team_ids = [incident_catalog_entry.terraform_reassignment.id]
}

resource "incident_escalation_path" "reassigning" {
  name = %[3]q

  path = [
    {
      type = "level"
      level = {
        targets = [{
          type    = "schedule"
          id      = incident_schedule.reassignment.id
          urgency = "low"
        }]
        time_to_ack_seconds = 300
      }
    },
    {
      type = "escalation_path"
      escalation_path = {
        escalation_path_id = incident_escalation_path.fallback.id
      }
    }
  ]

  team_ids = [incident_catalog_entry.terraform_reassignment.id]
}
`, teamTypeName(), name, name, StableSuffix("tf-acceptance-test-reassignment"))
}
