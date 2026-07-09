package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/samber/lo"
)

func TestAccIncidentWorkflowResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and check state
			{
				Config: testAccIncidentWorkflowResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "name", incidentWorkflowDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "condition_groups.0.conditions.0.param_bindings.0.array_value.0.literal", incidentWorkflowDefault().ConditionParam),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.1.array_value.0.literal", incidentWorkflowDefault().StepFollowUpName),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "expressions.0.label", incidentWorkflowDefault().ExpressionLabel),
				),
			},
			// Import
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update name and check new state
			{
				Config: testAccIncidentWorkflowResourceConfig(&workflowTemplateOverrides{
					Name: "My New Name",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "name", "My New Name"),
				),
			},
			// Update conditions and check new state
			{
				Config: testAccIncidentWorkflowResourceConfig(&workflowTemplateOverrides{
					ConditionParam: "closed",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "condition_groups.0.conditions.0.param_bindings.0.array_value.0.literal", "closed"),
				),
			},
			// Update step and check new state
			{
				Config: testAccIncidentWorkflowResourceConfig(&workflowTemplateOverrides{
					StepFollowUpName: "Organise postmortem meeting",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.1.array_value.0.literal", "Organise postmortem meeting"),
				),
			},
			// Update expression and check new state
			{
				Config: testAccIncidentWorkflowResourceConfig(&workflowTemplateOverrides{
					ExpressionLabel: "Active participants count",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "expressions.0.label", "Active participants count"),
				),
			},
			// (Clean-up)
		},
	})
}

type workflowTemplateOverrides struct {
	Name             string
	ConditionParam   string
	StepFollowUpName string
	ExpressionLabel  string
}

var incidentWorkflowTemplate = template.Must(template.New("incident_workflow").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_workflow" "example" {
	name               = {{ quote .Name }}
	trigger            = "incident.updated"
	condition_groups 	 = [
		{
			conditions = [
				{
					subject = "incident.status.category"
					operation = "one_of"
					param_bindings = [
						{
							array_value = [
								{
									literal = {{ quote .ConditionParam }}
								}
							]
						}
					]
				}
			]
		}
	]
	steps = [
		{
			id = "01HXVEA7Y0VWQBJB4F2X8WNRW6"
			name = "incident.create_follow_ups"
			param_bindings = [
				{
					value = {
						reference = "incident"
					}
				},
				{
					array_value = [
						{
							literal = {{ quote .StepFollowUpName }}
						}
					]
				},
				{}
			]
		}
	]
	expressions = [
		{
			label = {{ quote .ExpressionLabel }}
			operations = [
				{
					operation_type = "count"
				}
			]
			reference = "participants_cnt"
			root_reference = "incident.active_participants"
		}
	]
	once_for = ["incident"]
	include_private_incidents = false
	continue_on_step_error = false
	runs_on_incidents = "newly_created"
	runs_on_incident_modes = ["standard"]
	state = "draft"
}
`))

func incidentWorkflowDefault() workflowTemplateOverrides {
	return workflowTemplateOverrides{
		Name:             "My Test Workflow",
		ConditionParam:   "open",
		StepFollowUpName: "Write postmortem",
		ExpressionLabel:  "Count active participants",
	}
}

func testAccIncidentWorkflowResourceConfig(override *workflowTemplateOverrides) string {
	model := incidentWorkflowDefault()

	// Merge any non-zero fields in override into the model.
	if override != nil {
		for idx := 0; idx < reflect.TypeOf(*override).NumField(); idx++ {
			field := reflect.ValueOf(*override).Field(idx)
			if !field.IsZero() {
				reflect.ValueOf(&model).Elem().Field(idx).Set(field)
			}
		}
	}

	var buf bytes.Buffer
	if err := incidentWorkflowTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}

// TestAccIncidentWorkflowResourceOwningTeamIDs checks that owning_team_ids round-trips:
// unset reads back as absent, and teams provisioned in-config are applied and re-read.
// Teams are a catalog type, so the test creates its own Team entries to reference.
func TestAccIncidentWorkflowResourceOwningTeamIDs(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create without owning_team_ids
			{
				Config: testAccIncidentWorkflowResourceConfigOwningTeams(0),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_workflow.example", "owning_team_ids"),
				),
			},
			// Update to add a single owning team
			{
				Config: testAccIncidentWorkflowResourceConfigOwningTeams(1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "owning_team_ids.#", "1"),
					resource.TestCheckResourceAttrPair(
						"incident_workflow.example", "owning_team_ids.0",
						"incident_catalog_entry.owner_team_0", "id"),
				),
			},
			// Import and verify the owning teams survive a round-trip
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update to two owning teams
			{
				Config: testAccIncidentWorkflowResourceConfigOwningTeams(2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "owning_team_ids.#", "2"),
				),
			},
			// Clear owning teams again
			{
				Config: testAccIncidentWorkflowResourceConfigOwningTeams(0),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_workflow.example", "owning_team_ids"),
				),
			},
		},
	})
}

// testAccIncidentWorkflowResourceConfigOwningTeams renders a workflow owning teamCount
// self-provisioned Team catalog entries (0 omits owning_team_ids entirely).
func testAccIncidentWorkflowResourceConfigOwningTeams(teamCount int) string {
	teamIDs := make([]string, teamCount)
	for i := 0; i < teamCount; i++ {
		teamIDs[i] = fmt.Sprintf("incident_catalog_entry.owner_team_%d.id", i)
	}

	return testRunTemplate("incident_workflow_with_owning_teams", `
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

{{ range $i := .TeamIndices }}
resource "incident_catalog_entry" "owner_team_{{ $i }}" {
  catalog_type_id  = data.incident_catalog_type.team.id
  external_id      = "tf-workflow-owning-team-test-{{ $i }}"
  name             = "Terraform Workflow Owning Team Test {{ $i }}"
  attribute_values = []
}
{{ end }}

resource "incident_workflow" "example" {
  name    = "Owning teams workflow"
  trigger = "incident.updated"
  condition_groups = [
    {
      conditions = [
        {
          subject        = "incident.status.category"
          operation      = "one_of"
          param_bindings = [{ array_value = [{ literal = "open" }] }]
        }
      ]
    }
  ]
  steps = [
    {
      id   = "01HXVEA7Y0VWQBJB4F2X8WNRW6"
      name = "incident.create_follow_ups"
      param_bindings = [
        { value = { reference = "incident" } },
        { array_value = [{ literal = "Write postmortem" }] },
        {}
      ]
    }
  ]
  expressions               = []
  once_for                  = ["incident"]
  include_private_incidents = false
  continue_on_step_error    = false
  runs_on_incidents         = "newly_created"
  runs_on_incident_modes    = ["standard"]
  state                     = "draft"
  {{ if .TeamIDs }}owning_team_ids = [{{ range $i, $ref := .TeamIDs }}{{ if $i }}, {{ end }}{{ $ref }}{{ end }}]{{ end }}
}
`, struct {
		TeamTypeName string
		TeamIndices  []int
		TeamIDs      []string
	}{
		TeamTypeName: teamTypeName(),
		TeamIndices:  lo.Range(teamCount),
		TeamIDs:      teamIDs,
	})
}
