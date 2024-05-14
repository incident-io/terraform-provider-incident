package provider

import (
	"bytes"
	"reflect"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentWorkflowResource(t *testing.T) {
	// Simple case
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and check state
			{
				Config: testAccIncidentWorkflowSimpleResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "name", incidentWorkflowDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "condition_groups.0.conditions.0.param_bindings.0.array_value.0.literal", incidentWorkflowDefault().ConditionParam),
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "steps.0.param_bindings.1.array_value.0.literal", incidentWorkflowDefault().StepFollowUpName),
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "expressions.0.label", incidentWorkflowDefault().ExpressionLabel),
				),
			},
			// Import
			{
				ResourceName:      "incident_workflow.simple_example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update name and check new state
			{
				Config: testAccIncidentWorkflowSimpleResourceConfig(&workflowTemplateOverrides{
					Name: "My New Name",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "name", "My New Name"),
				),
			},
			// Update conditions and check new state
			{
				Config: testAccIncidentWorkflowSimpleResourceConfig(&workflowTemplateOverrides{
					ConditionParam: "closed",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "condition_groups.0.conditions.0.param_bindings.0.array_value.0.literal", "closed"),
				),
			},
			// Update step and check new state
			{
				Config: testAccIncidentWorkflowSimpleResourceConfig(&workflowTemplateOverrides{
					StepFollowUpName: "Organise postmortem meeting",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "steps.0.param_bindings.1.array_value.0.literal", "Organise postmortem meeting"),
				),
			},
			// Update expression and check new state
			{
				Config: testAccIncidentWorkflowSimpleResourceConfig(&workflowTemplateOverrides{
					ExpressionLabel: "Active participants count",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.simple_example", "expressions.0.label", "Active participants count"),
				),
			},
			// (Clean-up)
		},
	})
	// Complex case - looping steps, templated text, multiple expressions, varied once-for
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and check state
			{
				Config: testAccIncidentWorkflowComplexResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.complex_example", "name", "A Complicated Workflow"),
				),
			},
			// Import
			{
				ResourceName:      "incident_workflow.complex_example",
				ImportState:       true,
				ImportStateVerify: true,
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

var incidentSimpleWorkflowTemplate = template.Must(template.New("incident_workflow").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_workflow" "simple_example" {
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
			id = "01HXVEB7E3Z1Q1Z7QYDZ8ABDWM"
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

var incidentComplexWorkflowTemplate = template.Must(template.New("incident_workflow").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_workflow" "complex_example" {
	name    = "A Complicated Workflow"
	trigger = "slack.message_reaction_added"
	expressions = [
	  {
		id             = "01HXW1GSKTZSM2Y2RXSY1B51V4"
		label          = "Message sender"
		reference      = "bbb16c82"
		root_reference = "slack_message"
		returns = {
		  type  = "User"
		  array = false
		}
		else_branch = {
		  result = {
			value = {
			  # "Not Benji"
			  literal = "01HM6QTYKB156EVB6D7S81DP6A"
			}
		  }
		}
		operations = [
		  {
			operation_type = "navigate"
			returns = {
			  type  = "User"
			  array = false
			}
			navigate = {
			  reference       = "sender"
			  reference_label = "Sender"
			}
		  },
		]
	  },
	  {
		id             = "01HXW1GSKTZSM2Y2RXSZNFB0Z6"
		label          = "Team members"
		reference      = "965ccb60"
		root_reference = "incident.role[\"01HM6HNBF08FSEZWW4CMA8HJ9E\"].catalog_attribute[\"01HWT283SV9BAEYF63PHAE0R2D\"]"
		returns = {
		  type  = "CatalogEntry[\"User\"]"
		  array = true
		}
		else_branch = {
		  result = {
			array_value = [
			  {
				# "Not Benji"
				literal = "01HM6QTYKB156EVB6D7S81DP6A"
			  },
			]
		  }
		}
		operations = [
		  {
			operation_type = "navigate"
			returns = {
			  type  = "CatalogEntry[\"User\"]"
			  array = true
			}
			navigate = {
			  reference       = "catalog_attribute[\"01HSXG0GYNWP8MECBBKASYSYTS\"]"
			  reference_label = "Members"
			}
		  },
		]
	  },
	]
	condition_groups = [
	  {
		conditions = [
		  {
			# "Incident → Status → Category"
			subject   = "incident.status.category"
			operation = "one_of"
			param_bindings = [
			  {
				array_value = [
				  {
					# "Active"
					literal = "active"
				  },
				]
			  },
			]
		  },
		  {
			# "Message sender"
			subject   = "expressions[\"bbb16c82\"]"
			operation = "one_of"
			param_bindings = [
			  {
				array_value = [
				  {
					# "Benjamin Sidi"
					literal = "01HM6HNCS457WEV1GQC4JSDR0G"
				  },
				]
			  },
			]
		  },
		]
	  },
	]
	steps = [
	  {
		# "Send message to a channel"
		id       = "01HXS0K2769E020ZM5B2WR3Y4A"
		name     = "slack.post_message"
		for_each = "incident.active_participants"
		param_bindings = [
		  {
			value = {
			  # "Incident → Slack Channel"
			  reference = "incident.slack_channel"
			}
		  },
		  {
			value = {
			  # "Messaging Each Incident → Active Participant → Name...."
			  literal = "{\"content\":[{\"content\":[{\"text\":\"Messaging \",\"type\":\"text\"},{\"attrs\":{\"label\":\"Each Incident → Active Participant → Name\",\"missing\":false,\"name\":\"loop_variable.name\"},\"type\":\"varSpec\"},{\"text\":\"....\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
			}
		  },
		  {
			value = {
			  # "Emoji reaction"
			  literal = "{\"content\":[{\"content\":[{\"attrs\":{\"label\":\"Emoji reaction\",\"missing\":false,\"name\":\"reaction\"},\"type\":\"varSpec\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
			}
		  },
		]
	  },
	  {
		# "Send direct message"
		id       = "01HXS0K2769E020ZM5B5WGVKCM"
		name     = "slack.send_message"
		for_each = "incident.active_participants"
		param_bindings = [
		  {
			array_value = [
			  {
				# "Incident → Active Participants"
				reference = "loop_variable"
			  },
			]
		  },
		  {
			value = {
			  # "Suh Emoji reaction"
			  literal = "{\"content\":[{\"content\":[{\"text\":\"Suh \",\"type\":\"text\"},{\"attrs\":{\"label\":\"Emoji reaction\",\"missing\":false,\"name\":\"reaction\"},\"type\":\"varSpec\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
			}
		  },
		]
	  },
	  {
		# "Pin message to timeline"
		id   = "01HXS0K2769E020ZM5B6DR19S8"
		name = "slack.pin_message"
		param_bindings = [
		  {
			value = {
			  # "Message"
			  reference = "slack_message"
			}
		  },
		  {
			value = {
			  # "Incident"
			  reference = "incident"
			}
		  },
		]
	  },
	  {
		# "Invite user or group to the incident's channel"
		id   = "01HXS0K2769E020ZM5B6NC83HH"
		name = "slack.invite_user"
		param_bindings = [
		  {
			value = {
			  # "Incident"
			  reference = "incident"
			}
		  },
		  {
			array_value = [
			  {
				# "Team members"
				reference = "expressions[\"965ccb60\"]"
			  },
			]
		  },
		  {
		  },
		]
	  },
	]
	once_for = [
	  # "Message"
	  "slack_message",
	  # "Emoji reaction"
	  "reaction",
	]
	include_private_incidents = false
	continue_on_step_error    = false
	runs_on_incidents         = "newly_created_and_active"
	runs_on_incident_modes = [
	  "standard",
	]
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

func testAccIncidentWorkflowSimpleResourceConfig(override *workflowTemplateOverrides) string {
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
	if err := incidentSimpleWorkflowTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccIncidentWorkflowComplexResourceConfig(override *workflowTemplateOverrides) string {
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
	if err := incidentComplexWorkflowTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
