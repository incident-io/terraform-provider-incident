package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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
						"incident_workflow.example", "steps.0.param_bindings.1.value.literal",
						fmt.Sprintf("{\"content\":[{\"content\":[{\"text\":\"%s\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}", incidentWorkflowDefault().StepSlackMessage)),
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
					StepSlackMessage: "Don't forget to write a postmortem!",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.1.value.literal",
						"{\"content\":[{\"content\":[{\"text\":\"Don't forget to write a postmortem!\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"),
				),
			},
			// (Clean-up)
		},
	})
}

type workflowTemplateOverrides struct {
	Name             string
	ConditionParam   string
	StepSlackMessage string
}

var incidentWorkflowTemplate = template.Must(template.New("incident_workflow").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_workflow" "example" {
	name               = {{ quote .Name }}
	trigger            = "incident.updated"
	terraform_repo_url = "https://github.com/incident-io/test"
	condition_groups 	 = [
		{
			conditions = [
				{
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
					subject = "incident.status.category"
				}
			]
		}
	]
	steps = [
		{
			name = "slack.post_message"
			param_bindings = [
				{
					value = {
						reference = "incident.slack_channel"
					}
				},
				{
					value = {
						literal = "{\"content\":[{\"content\":[{\"text\":\"{{ .StepSlackMessage }}\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
					}
				},
				{}
			]
		}
	]
}
`))

func incidentWorkflowDefault() workflowTemplateOverrides {
	return workflowTemplateOverrides{
		Name:             "My Test Workflow",
		ConditionParam:   "open",
		StepSlackMessage: "This is a slack message!",
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
