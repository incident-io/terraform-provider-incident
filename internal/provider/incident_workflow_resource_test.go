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
	// Complex case – looping steps, templated text, multiple expressions, varied once-for
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and check state
			{
				Config: testAccIncidentWorkflowComplexResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.complex_example", "name", "Page CSMs of Tier 1 Affected Customers"),
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

// This workflow finds all the values of the `affected customers` field with tier 1, and pages their
// CSM. It then sends a message to the incident channel explaining why the CSM has been paged, and threads the list of
// affected customers.
var incidentComplexWorkflowTemplate = template.Must(template.New("incident_workflow").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_workflow" "complex_example" {
  name    = "Page CSMs of Tier 1 Affected Customers"
  trigger = "incident.updated"
  expressions = [
	# This expression finds all the CSMs of the affected customers
    {
      id             = "01HXXTX10FE2BPJR2DQAP4PJ3H"
      label          = "CSMs of Tier 1 customers"
      reference      = "f2d47f60"
      root_reference = "incident.custom_field[\"01HXXTNJYBV9B37KTBBR8AM7ZX\"]"
      returns = {
        type  = "CatalogEntry[\"User\"]"
        array = true
      }
      operations = [
        {
          operation_type = "filter"
          returns = {
            type  = "CatalogEntry[\"01HXXTH6QM3626M8Y5E3FV9DNP\"]"
            array = true
          }
          filter = {
            condition_groups = [
              {
                conditions = [
                  {
                    # "Incident → Affected Customers → Tier"
                    subject   = "input.catalog_attribute[\"01HXXTH6RTH5JA69372JEHH13Y\"]"
                    operation = "one_of"
                    param_bindings = [
                      {
                        array_value = [
                          {
                            # "First tertile"
                            literal = "01HSXFR18GEZKGSB15EF729VVF"
                          },
                        ]
                      },
                    ]
                  },
                ]
              },
            ]
          }
        },
        {
          operation_type = "navigate"
          returns = {
            type  = "CatalogEntry[\"User\"]"
            array = true
          }
          navigate = {
            reference       = "catalog_attribute[\"01HXXTH6RTH5JA69372MMAXTPP\"]"
            reference_label = "CSM"
          }
        },
      ]
    },
	# This expression finds all the affected customers, so we can thread the list to our message
    {
      id             = "01HXXTX10G798SJ373GWY9ZWNX"
      label          = "Tier 1 affected customers"
      reference      = "b1a797ec"
      root_reference = "incident.custom_field[\"01HXXTNJYBV9B37KTBBR8AM7ZX\"]"
      returns = {
        type  = "CatalogEntry[\"01HXXTH6QM3626M8Y5E3FV9DNP\"]"
        array = true
      }
      operations = [
        {
          operation_type = "filter"
          returns = {
            type  = "CatalogEntry[\"01HXXTH6QM3626M8Y5E3FV9DNP\"]"
            array = true
          }
          filter = {
            condition_groups = [
              {
                conditions = [
                  {
                    # "Incident → Affected Customers → Tier"
                    subject   = "input.catalog_attribute[\"01HXXTH6RTH5JA69372JEHH13Y\"]"
                    operation = "one_of"
                    param_bindings = [
                      {
                        array_value = [
                          {
                            # "First tertile"
                            literal = "01HSXFR18GEZKGSB15EF729VVF"
                          },
                        ]
                      },
                    ]
                  },
                ]
              },
            ]
          }
        },
      ]
    },
  ]
  # We only want to run this in major and critical incidents
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
          # "Incident → Severity"
          subject   = "incident.severity"
          operation = "one_of"
          param_bindings = [
            {
              array_value = [
                {
                  # "Critical"
                  literal = "01HM6HNBF0FR92MVA2VQ8BGC6G"
                },
                {
                  # "Major"
                  literal = "01HM6HNBF0CYPVR5FEKGVCBMAY"
                },
              ]
            },
          ]
        },
      ]
    },
  ]
  # We've got one loop with two steps: page the CSM, and write the message
  steps = [
    {
      # "Escalate via incident.io"
      id       = "01HXXTX0W5A896CXHZW20SJCV2"
      name     = "escalate"
      # This expression is the affected CSMs
      for_each = "expressions[\"f2d47f60\"]"
      param_bindings = [
        {
          value = {
            # "Incident"
            reference = "incident"
          }
        },
        {
        },
        {
          array_value = [
            {
              # "CSMs of Tier 1 customers"
              reference = "loop_variable"
            },
          ]
        },
      ]
    },
    {
      # "Send message to a channel"
      id       = "01HXXTX0W5A896CXHZW43F08DJ"
      name     = "slack.post_message"
      for_each = "expressions[\"f2d47f60\"]"
      param_bindings = [
        {
          value = {
            # "Incident → Slack Channel"
            reference = "incident.slack_channel"
          }
        },
        {
          value = {
            literal = "{\"content\":[{\"content\":[{\"text\":\"Hi \",\"type\":\"text\"},{\"attrs\":{\"label\":\"Each CSMs of Tier 1 customer → Name\",\"missing\":false,\"name\":\"loop_variable.name\"},\"type\":\"varSpec\"},{\"text\":\", one or more of your tier 1 customers is affected by this incident. Affected customers are in :thread:.\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
          }
        },
        {
          value = {
            literal = "{\"content\":[{\"content\":[{\"attrs\":{\"label\":\"Tier 1 affected customers\",\"missing\":false,\"name\":\"expressions[\\\"b1a797ec\\\"]\"},\"type\":\"varSpec\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
          }
        },
      ]
    },
  ]
  once_for = [
    # "Incident"
    "incident",
    # "CSMs of Tier 1 customers"
    "expressions[\"f2d47f60\"]",
  ]
  include_private_incidents = false
  continue_on_step_error    = true
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
