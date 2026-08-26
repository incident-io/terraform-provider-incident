package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
					Name: StableSuffix("My New Name"),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "name", StableSuffix("My New Name")),
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

var incidentWorkflowTemplate = template.Must(template.New("incident_workflow").Funcs(testTemplateFuncs()).Parse(`
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
		Name:             StableSuffix("My Test Workflow"),
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

// TestAccIncidentWorkflowResourcePrivateIncidentScope checks that private_incident_scope
// round-trips and that the deprecated include_private_incidents is derived from it on read.
func TestAccIncidentWorkflowResourcePrivateIncidentScope(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with scope = all; the deprecated bool is derived to true.
			{
				Config: testAccIncidentWorkflowConfigPrivacy(`private_incident_scope = "all"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "private_incident_scope", "all"),
					resource.TestCheckResourceAttr("incident_workflow.example", "include_private_incidents", "true"),
				),
			},
			// Import round-trip.
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update to scope = none; derived bool flips to false.
			{
				Config: testAccIncidentWorkflowConfigPrivacy(`private_incident_scope = "none"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "private_incident_scope", "none"),
					resource.TestCheckResourceAttr("incident_workflow.example", "include_private_incidents", "false"),
				),
			},
			// Legacy path: setting only the deprecated bool still works and derives the scope.
			{
				Config: testAccIncidentWorkflowConfigPrivacy(`include_private_incidents = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "include_private_incidents", "true"),
					resource.TestCheckResourceAttr("incident_workflow.example", "private_incident_scope", "all"),
				),
			},
			// Flipping only the deprecated bool must reach the API even
			// though a scope is now carried in state — decisions read config, not plan.
			{
				Config: testAccIncidentWorkflowConfigPrivacy(`include_private_incidents = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "include_private_incidents", "false"),
					resource.TestCheckResourceAttr("incident_workflow.example", "private_incident_scope", "none"),
				),
			},
			// Both fields set together is accepted as long as they agree (the round-trip case).
			{
				Config: testAccIncidentWorkflowConfigPrivacy(`
  include_private_incidents = true
  private_incident_scope    = "all"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "private_incident_scope", "all"),
					resource.TestCheckResourceAttr("incident_workflow.example", "include_private_incidents", "true"),
				),
			},
		},
	})
}

// TestAccIncidentWorkflowResourcePrivateScopeConflict checks that contradictory bool/scope values
// are rejected at plan time (mirrors the API's 422); agreeing values are covered by
// TestAccIncidentWorkflowResourcePrivateIncidentScope.
func TestAccIncidentWorkflowResourcePrivateScopeConflict(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// none does not touch private incidents, so the bool must be false to agree.
				Config: testAccIncidentWorkflowConfigPrivacy(`
  include_private_incidents = true
  private_incident_scope    = "none"`),
				ExpectError: regexp.MustCompile("disagree"),
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
  external_id      = {{ stableSuffix (printf "tf-workflow-owning-team-test-%d" $i) | quote }}
  name             = {{ stableSuffix (printf "Terraform Workflow Owning Team Test %d" $i) | quote }}
  attribute_values = []
}
{{ end }}

resource "incident_workflow" "example" {
  name    = {{ stableSuffix "Owning teams workflow" | quote }}
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

// TestAccIncidentWorkflowResourceFormFields checks that form_fields round-trips on a
// manually triggered workflow: unset reads back as absent, fields are applied and
// re-read (with a server-generated id), and both `[]` and omitting the attribute
// clear them again without leaving a diff behind.
func TestAccIncidentWorkflowResourceFormFields(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create without form_fields: the attribute stays absent.
			{
				Config: testAccIncidentWorkflowConfigFormFields(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_workflow.example", "form_fields"),
				),
			},
			// Add a single form field. id is computed, so we only check it is set.
			{
				Config: testAccIncidentWorkflowConfigFormFields(`
  form_fields = [
    {
      key         = "reason"
      title       = "Reason for paging"
      type        = "Text"
      description = "Why are we paging the execs?"
      array       = false
      required    = true
    },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.#", "1"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.key", "reason"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.title", "Reason for paging"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.type", "Text"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.description", "Why are we paging the execs?"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.array", "false"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.required", "true"),
					resource.TestCheckResourceAttrSet("incident_workflow.example", "form_fields.0.id"),
				),
			},
			// Import and verify the form fields survive a round-trip. Import has no
			// planned value to respect, so it adopts whatever the API reports.
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update the first field and add a second, array-valued one. Dropping the
			// optional description must read back as absent, not as an empty string.
			{
				Config: testAccIncidentWorkflowConfigFormFields(`
  form_fields = [
    {
      key      = "reason"
      title    = "Why are we paging?"
      type     = "Text"
      array    = false
      required = false
    },
    {
      key      = "responders"
      title    = "Who should we page?"
      type     = "User"
      array    = true
      required = true
    },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.#", "2"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.title", "Why are we paging?"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.0.required", "false"),
					resource.TestCheckNoResourceAttr("incident_workflow.example", "form_fields.0.description"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.1.key", "responders"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.1.type", "User"),
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.1.array", "true"),
				),
			},
			// An explicit empty list clears the fields and stays empty in state,
			// rather than collapsing to null and failing as an inconsistent result.
			{
				Config: testAccIncidentWorkflowConfigFormFields(`
  form_fields = []`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.#", "0"),
				),
			},
			// Re-add a field so the next step has something to clear by omission.
			{
				Config: testAccIncidentWorkflowConfigFormFields(`
  form_fields = [
    {
      key      = "reason"
      title    = "Why are we paging?"
      type     = "Text"
      array    = false
      required = false
    },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.#", "1"),
				),
			},
			// Omitting the attribute clears the fields too, and reads back as absent:
			// config is the source of truth, so a workflow that doesn't mention form
			// fields ends up without any.
			{
				Config: testAccIncidentWorkflowConfigFormFields(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_workflow.example", "form_fields"),
				),
			},
		},
	})
}

// TestAccIncidentWorkflowResourceFormFieldIDsFollowKeys checks that a form field's
// computed id follows its key rather than its position in the list. form_fields is
// ordered, so an id correlated by position moves onto the wrong field the moment an
// earlier field is removed — the update would then rewrite the surviving field's
// identity (and type) instead of deleting the one that went away.
func TestAccIncidentWorkflowResourceFormFieldIDsFollowKeys(t *testing.T) {
	idsByKey := map[string]string{}

	captureIDs := func(s *terraform.State) error {
		res, ok := s.RootModule().Resources["incident_workflow.example"]
		if !ok {
			return fmt.Errorf("incident_workflow.example not found in state")
		}

		idsByKey = map[string]string{}
		for i := 0; ; i++ {
			key, ok := res.Primary.Attributes[fmt.Sprintf("form_fields.%d.key", i)]
			if !ok {
				break
			}
			idsByKey[key] = res.Primary.Attributes[fmt.Sprintf("form_fields.%d.id", i)]
		}

		if len(idsByKey) != 2 {
			return fmt.Errorf("expected to capture 2 form field ids, got %d", len(idsByKey))
		}
		for key, id := range idsByKey {
			if id == "" {
				return fmt.Errorf("form field %q has no id in state", key)
			}
		}

		return nil
	}

	// checkOnlyRemainingField asserts the single surviving field is the expected
	// key and still carries the id it was created with.
	checkOnlyRemainingField := func(key string) func(*terraform.State) error {
		return func(s *terraform.State) error {
			want := idsByKey[key]
			if want == "" {
				return fmt.Errorf("no id captured for form field %q", key)
			}

			res, ok := s.RootModule().Resources["incident_workflow.example"]
			if !ok {
				return fmt.Errorf("incident_workflow.example not found in state")
			}
			if got := res.Primary.Attributes["form_fields.0.key"]; got != key {
				return fmt.Errorf("expected the remaining form field to be %q, got %q", key, got)
			}
			if got := res.Primary.Attributes["form_fields.0.id"]; got != want {
				return fmt.Errorf(
					"form field %q id changed from %q to %q: ids must follow the key, not the list position",
					key, want, got)
			}

			return nil
		}
	}

	twoFields := `
  form_fields = [
    {
      key      = "reason"
      title    = "Reason"
      type     = "Text"
      array    = false
      required = false
    },
    {
      key      = "responders"
      title    = "Responders"
      type     = "User"
      array    = true
      required = false
    },
  ]`

	// The first field is gone, so responders moves from index 1 to index 0.
	onlyResponders := `
  form_fields = [
    {
      key      = "responders"
      title    = "Responders"
      type     = "User"
      array    = true
      required = false
    },
  ]`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowConfigFormFields(twoFields),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.#", "2"),
					captureIDs,
				),
			},
			{
				Config: testAccIncidentWorkflowConfigFormFields(onlyResponders),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "form_fields.#", "1"),
					checkOnlyRemainingField("responders"),
				),
			},
		},
	})
}

// testAccIncidentWorkflowConfigFormFields renders a manually triggered workflow with
// the given form_fields attribute lines spliced in (empty string omits them). form
// fields only apply to manual triggers, so this can't reuse the incident.updated
// workflow the other tests share.
func testAccIncidentWorkflowConfigFormFields(formFields string) string {
	return testRunTemplate("incident_workflow_form_fields", `
resource "incident_workflow" "example" {
  name    = "Form fields workflow"
  trigger = "manual"
  condition_groups = []
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
  expressions            = []
  once_for               = ["incident"]
  {{ .FormFields }}
  include_private_incidents = false
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = ["standard"]
  state                  = "draft"
}
`, struct{ FormFields string }{FormFields: formFields})
}

// testAccIncidentWorkflowConfigPrivacy renders a minimal workflow with the given
// privacy attribute lines spliced in (e.g. a private_incident_scope assignment).
func testAccIncidentWorkflowConfigPrivacy(privacy string) string {
	return testRunTemplate("incident_workflow_privacy", `
resource "incident_workflow" "example" {
  name    = {{ stableSuffix "Private scope workflow" | quote }}
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
  expressions            = []
  once_for               = ["incident"]
  {{ .Privacy }}
  continue_on_step_error = false
  runs_on_incidents      = "newly_created"
  runs_on_incident_modes = ["standard"]
  state                  = "draft"
}
`, struct{ Privacy string }{Privacy: privacy})
}
