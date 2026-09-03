package provider

import (
	"bytes"
	"context"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// TestIncidentEscalationPathDataSourceSchemaMatchesModel guards the seam between the data source
// schema and IncidentEscalationPathResourceModel, the same way the workflow data source's test
// does: Read reuses the resource's buildModel, so an attribute the schema forgets fails every
// escalation path read rather than only the paths using it.
func TestIncidentEscalationPathDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&IncidentEscalationPathDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	// Everything the API can hand back, so a nested attribute missing from the schema shows up
	// as an error rather than an untouched null.
	ep := client.EscalationPathV2{
		Id:      "01M0TAR6EMNDC7BVA45RZDE5KM",
		Name:    "Paged by severity",
		TeamIds: []string{"01G0J1EXE7AXZ2C93K61WBPYEH"},
		Path: []client.EscalationPathNodeV2{
			{
				Id:   "start",
				Type: client.EscalationPathNodeV2TypeIfElse,
				IfElse: &client.EscalationPathNodeIfElseV2{
					Conditions: []client.ConditionV2{
						{
							Subject:   client.ConditionSubjectV2{Reference: "incident.severity"},
							Operation: client.ConditionOperationV2{Value: "one_of"},
							ParamBindings: []client.EngineParamBindingV2{
								{ArrayValue: &[]client.EngineParamBindingValueV2{{Literal: lo.ToPtr("high")}}},
								{Value: &client.EngineParamBindingValueV2{Reference: lo.ToPtr("incident.severity")}},
							},
						},
					},
					ThenPath: []client.EscalationPathNodeV2{{
						Id:   "then-level",
						Type: client.EscalationPathNodeV2TypeLevel,
						Level: &client.EscalationPathNodeLevelV2{
							Targets: []client.EscalationPathTargetV2{{
								Id:      "01G0J1EXE7AXZ2C93K61WBPYEH",
								Type:    client.EscalationPathTargetV2TypeSchedule,
								Urgency: client.EscalationPathTargetV2UrgencyHigh,
							}},
							TimeToAckSeconds: lo.ToPtr(int64(300)),
						},
					}},
					ElsePath: []client.EscalationPathNodeV2{{
						Id:    "else-delay",
						Type:  client.EscalationPathNodeV2TypeDelay,
						Delay: &client.EscalationPathNodeDelayV2{DelaySeconds: lo.ToPtr(int64(120))},
					}},
				},
			},
		},
		WorkingHours: &[]client.WeekdayIntervalConfigV2{{
			Id:       "UK",
			Name:     "UK",
			Timezone: "Europe/London",
			WeekdayIntervals: []client.WeekdayIntervalV2{
				{StartTime: "09:00", EndTime: "17:00", Weekday: client.WeekdayIntervalV2WeekdayMonday},
			},
		}},
		RepeatConfig: &client.EscalationPathRepeatConfigV2{
			RepeatAfterSeconds:    300,
			DelayRepeatOnActivity: true,
		},
	}

	var diags diag.Diagnostics
	model := (&IncidentEscalationPathResource{}).buildModel(ctx, ep, nil, &diags)
	require.False(t, diags.HasError(), "buildModel: %s", diags)

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	assert.False(t, state.Set(ctx, model).HasError(), "setting state from the resource model")
}

func accIncidentEscalationPathDataSource(t *testing.T) {
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

					// Verify simple delay node is returned via data source
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.if_else.then_path.1.type", "delay"),
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.if_else.then_path.1.delay.delay_seconds", "120"),

					// Verify working hours delay node is returned via data source
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.if_else.else_path.2.type", "delay"),
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.if_else.else_path.2.delay.delay_interval_condition", "active"),
					resource.TestCheckResourceAttr(
						"data.incident_escalation_path.by_id", "path.0.if_else.else_path.2.delay.delay_weekday_interval_config_id", "UK"),

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

var escalationPathDataSourceTemplate = template.Must(template.New("incident_escalation_path_data_source").Funcs(testTemplateFuncs()).Parse(`
# This is the _official_ team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# This is a team catalog entry
resource "incident_catalog_entry" "terraform" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id = {{ stableSuffix "tf-acceptance-test-ds" | quote }}
  name = {{ stableSuffix "Terraform test team (data source)" | quote }}
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
