package provider

import (
	"bytes"
	"fmt"
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

func TestAccIncidentEscalationPathResourceValidateMaxDepth(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentEscalationPathResourceConfigExceedingMaxDepth(),
				// Nesting beyond the maximum supported depth is not caught by
				// schema validation at plan time; the API rejects it at apply
				// because the deepest if_else node cannot carry an if_else payload.
				ExpectError: regexp.MustCompile(`If_else type requires an if_else payload`)},
		},
	})
}

func testAccIncidentEscalationPathResourceConfigExceedingMaxDepth() string {
	return `
# This is the primary schedule that receives pages in working hours.
resource "incident_schedule" "primary_on_call" {
  name = "Deep Nesting Test Schedule"
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

  team_ids = []
}

# Create a path with if_else nodes nested one level beyond the maximum the
# schema supports (6 levels deep), which Terraform must reject during config
# validation because the schema does not define an if_else block at that depth.
resource "incident_escalation_path" "example" {
  name = "Deeply Nested Path Test"

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
                                        }
                                      ]
                                      else_path = []
                                    }
                                  }
                                ],
                                else_path = []
                              }
                            }
                          ],
                          else_path = []
                        }
                      }
                    ],
                    else_path = []
                  }
                }
              ],
              else_path = []
            }
          }
        ],
        else_path = []
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

  team_ids = []
}
`
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
  name = %q
}

resource "incident_catalog_entry" "terraform_rota_modes" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = "tf-acceptance-test-rota-modes"
  name               = "Terraform test team rota modes"
  attribute_values   = []
  managed_attributes = []
}

resource "incident_schedule" "rota_modes" {
  name     = %q
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
  name = %q

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
`, teamTypeName(), name, name)
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
  name = %q
}

resource "incident_catalog_entry" "terraform_unknown_values" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = "tf-acceptance-test-unknown-values"
  name               = "Terraform test team unknown values"
  attribute_values   = []
  managed_attributes = []
}

resource "incident_schedule" "unknown_values" {
  name     = %q
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
  name = %q

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
`, teamTypeName(), name, name)
}
