package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
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

// TestAccIncidentWorkflowResourceOwningTeamFromCatalog covers the pattern the resource
// example uses: teams are catalog entries, so the owning team is resolved through the
// catalog by name rather than by pasting in an ID that differs between workspaces.
func TestAccIncidentWorkflowResourceOwningTeamFromCatalog(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowResourceConfigOwningTeamFromCatalog(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The name lookup resolves to the team we provisioned...
					resource.TestCheckResourceAttrPair(
						"data.incident_catalog_entry.owner_team", "id",
						"incident_catalog_entry.owner_team", "id"),
					// ...and that is what the workflow ends up owned by.
					resource.TestCheckResourceAttr("incident_workflow.example", "owning_team_ids.#", "1"),
					resource.TestCheckResourceAttrPair(
						"incident_workflow.example", "owning_team_ids.0",
						"data.incident_catalog_entry.owner_team", "id"),
				),
			},
			// The owning team survives an import round-trip.
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccIncidentWorkflowResourceConfigOwningTeamFromCatalog renders a workflow owned by a
// team that is looked up by name, mirroring examples/resources/incident_workflow/resource.tf.
//
// The data source needs depends_on: its config is fully known at plan time, so without it
// Terraform would read the entry before the resource above has created it, and the lookup
// would fail with a not-found.
func testAccIncidentWorkflowResourceConfigOwningTeamFromCatalog() string {
	return testRunTemplate("incident_workflow_owning_team_from_catalog", `
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

resource "incident_catalog_entry" "owner_team" {
  catalog_type_id  = data.incident_catalog_type.team.id
  external_id      = {{ quote .TeamExternalID }}
  name             = {{ quote .TeamName }}
  attribute_values = []
}

data "incident_catalog_entry" "owner_team" {
  catalog_type_id = data.incident_catalog_type.team.id
  identifier      = {{ quote .TeamName }}

  depends_on = [incident_catalog_entry.owner_team]
}

resource "incident_workflow" "example" {
  name    = {{ quote .WorkflowName }}
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
  owning_team_ids           = [data.incident_catalog_entry.owner_team.id]
  include_private_incidents = false
  continue_on_step_error    = false
  runs_on_incidents         = "newly_created"
  runs_on_incident_modes    = ["standard"]
  state                     = "draft"
}
`, struct {
		TeamTypeName   string
		TeamName       string
		TeamExternalID string
		WorkflowName   string
	}{
		TeamTypeName:   teamTypeName(),
		TeamName:       StableSuffix("Terraform Workflow Owning Team Lookup"),
		TeamExternalID: StableSuffix("tf-workflow-owning-team-lookup"),
		WorkflowName:   StableSuffix("Owning team from catalog workflow"),
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
  name    = {{ stableSuffix "Form fields workflow" | quote }}
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

// TestAccIncidentWorkflowResourceStatusChangedExample covers the pattern the status
// example in examples/resources/incident_workflow/resource.tf uses: a workflow triggered
// by a status change, conditioning on a status resolved through the incident_status data
// source rather than a pasted ID, and binding a step parameter to a reference the trigger
// puts in scope.
//
// Applying is most of the assertion. Creating a workflow type-checks it server-side
// (Create -> UpdateVersion -> BuildWorkflow -> TypeCheck, in core), which is the only
// thing that checks the trigger exists, that `user-who-changed-the-status` is in its
// scope, and that each binding matches the type and arity of the parameter it fills.
// terraform validate can't see any of that, so an example is only really checked here.
func TestAccIncidentWorkflowResourceStatusChangedExample(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowResourceConfigStatusChanged(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "trigger", "incident.status-changed"),
					// The condition holds the status the data source found, not a literal ID.
					resource.TestCheckResourceAttrPair(
						"incident_workflow.example",
						"condition_groups.0.conditions.0.param_bindings.0.array_value.0.literal",
						"data.incident_status.trigger_status", "id"),
					// One binding per parameter the step declares, including the optional
					// ones left empty: a short list only survives where the step has an
					// Upgrade hook padding that exact length.
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.#", "6"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.2.value.reference",
						"user-who-changed-the-status"),
				),
			},
			// The bindings, empty ones included, survive an import round-trip.
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccIncidentWorkflowResourceConfigStatusChanged mirrors the status example, with one
// difference: the example conditions on Closed, which incident.io manages and so can't be
// created here, and looking up whatever the test account happens to call its closed status
// would make this depend on that account's configuration. The test provisions a status of
// its own instead - live being one of the two categories that are configurable - and looks
// that up by name.
//
// The lookup reads `incident_status.trigger_status.name` rather than repeating the name as
// a literal, so it can't run before the status exists. A data source whose config is fully
// known at plan time is read during plan, which is what the owning-team test needs
// depends_on for.
func testAccIncidentWorkflowResourceConfigStatusChanged() string {
	return testRunTemplate("incident_workflow_status_changed", `
resource "incident_status" "trigger_status" {
  name        = {{ stableSuffix "Mitigated" | quote }}
  description = "The incident is mitigated, and we're writing it up"
  category    = "live"
}

data "incident_status" "trigger_status" {
  name = incident_status.trigger_status.name
}

resource "incident_workflow" "example" {
  name        = {{ stableSuffix "Status changed workflow" | quote }}
  trigger     = "incident.status-changed"
  expressions = []
  condition_groups = [
    {
      conditions = [
        {
          subject   = "incident.status"
          operation = "one_of"
          param_bindings = [
            { array_value = [{ literal = data.incident_status.trigger_status.id }] }
          ]
        }
      ]
    }
  ]
  steps = [
    {
      id   = "01KT8B9QQH67GF9S6SW4A7DNEX"
      name = "incident.create_follow_ups"
      param_bindings = [
        { value = { reference = "incident" } },
        { array_value = [{ literal = "Write the postmortem" }] },
        { value = { reference = "user-who-changed-the-status" } },
        {},
        {},
        {}
      ]
    }
  ]
  once_for               = ["incident"]
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = ["standard"]
  state                  = "draft"
}
`, nil)
}

// TestAccIncidentWorkflowResourceFormFieldsInStepExample covers the pattern the form
// submission example uses: a manual workflow whose steps bind the values its form
// collects.
//
// The tests above cover form_fields as an attribute - ordering, computed ids, clearing -
// but none of them bind a field into a step, which is the half that was wrong in the
// example this PR fixes. Binding is where the types have to line up: an array parameter
// takes array_value even when one reference fills it, a scalar parameter takes value, and
// the reference has to name a field that BuildScope actually put in scope.
//
// The message goes through the rich text data source, so this also covers a parsed
// document reaching a TemplatedText parameter intact.
func TestAccIncidentWorkflowResourceFormFieldsInStepExample(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowResourceConfigFormFieldsInStep(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "form_fields.0.key", "reason"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "form_fields.1.key", "execs"),
					// The recipients are the array-valued form field, bound as an array
					// even though a single reference supplies every element.
					resource.TestCheckResourceAttr(
						"incident_workflow.example",
						"steps.0.param_bindings.0.array_value.0.reference", "form.execs"),
					// The message is the document the data source parsed, not a string.
					resource.TestCheckResourceAttrPair(
						"incident_workflow.example", "steps.0.param_bindings.1.value.literal",
						"data.incident_rich_text.exec_page", "json"),
				),
			},
			// The document survives an import round-trip: rich text is stored as a
			// literal and compared semantically, so this is where re-encoding on read
			// would show up.
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccIncidentWorkflowResourceConfigFormFieldsInStep mirrors the form submission
// example. slack.send_message is only eligible where Slack is the primary comms platform,
// which the integration workspace is.
func testAccIncidentWorkflowResourceConfigFormFieldsInStep() string {
	return testRunTemplate("incident_workflow_form_fields_in_step", `
data "incident_rich_text" "exec_page" {
  markdown    = "**Exec page for {{ "{{ incident.name }}" }}**\n\n{{ "{{ form.reason }}" }}"
  feature_set = "mrkdwn"
}

resource "incident_workflow" "example" {
  name        = {{ stableSuffix "Form fields in step workflow" | quote }}
  trigger     = "manual"
  expressions = []
  condition_groups = []
  steps = [
    {
      id   = "01KG1326KD7DT1Z785SYVXXP98"
      name = "slack.send_message"
      param_bindings = [
        { array_value = [{ reference = "form.execs" }] },
        { value = { literal = data.incident_rich_text.exec_page.json } },
        {}
      ]
    }
  ]
  form_fields = [
    {
      key      = "reason"
      title    = "Reason for paging"
      type     = "String"
      array    = false
      required = true
    },
    {
      key      = "execs"
      title    = "Who should we page?"
      type     = "User"
      array    = true
      required = true
    }
  ]
  once_for               = ["incident"]
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = ["standard"]
  state                  = "draft"
}
`, nil)
}

// TestAccIncidentWorkflowResourceInviteUserExample covers the invite example, including
// the data source lookups it recommends over pasting IDs.
//
// The interesting part is arity and typing: slack.invite_user takes an incident, then two
// optional arrays holding different resource types - users, and catalog entries of the
// managed Slack User Group type. A config that puts a group in the users array, or drops a
// binding for an optional parameter, is only caught when the workflow is created.
//
// The two values the example looks up are specific to an account, so rather than hardcode
// them the test asks the API what this one has, the way channelID does for Slack channels.
func TestAccIncidentWorkflowResourceInviteUserExample(t *testing.T) {
	// The lookups below run before resource.Test, so honour TF_ACC ourselves rather than
	// calling the API during a unit test run, then initialise testClient.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}
	testAccPreCheck(t)

	email := testAccUniqueActiveUserEmail(t)
	groupTypeName, groupIdentifier := testAccSlackUserGroup(t)
	if groupIdentifier == "" {
		t.Logf("no Slack user group in the catalog, leaving the group binding empty")
	}

	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(
			"incident_workflow.example", "steps.0.name", "slack.invite_user"),
		// Three bindings, one per parameter the step declares.
		resource.TestCheckResourceAttr(
			"incident_workflow.example", "steps.0.param_bindings.#", "3"),
		// The user the example invites is the one the data source found by email.
		resource.TestCheckResourceAttrPair(
			"incident_workflow.example", "steps.0.param_bindings.1.array_value.0.literal",
			"data.incident_user.security_lead", "id"),
	}
	if groupIdentifier != "" {
		checks = append(checks,
			// Groups bind as catalog entries, so this is the entry's ID rather than a
			// Slack group ID.
			resource.TestCheckResourceAttrPair(
				"incident_workflow.example", "steps.0.param_bindings.2.array_value.0.literal",
				"data.incident_catalog_entry.security_responders", "id"),
		)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowResourceConfigInviteUser(email, groupTypeName, groupIdentifier),
				Check:  resource.ComposeAggregateTestCheckFunc(checks...),
			},
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccUniqueActiveUserEmail returns the email of an active user in the test account, for
// examples that resolve people with the incident_user data source.
//
// It skips duplicated emails: the data source rejects a lookup matching more than one
// active user, which is a property of the account rather than anything the example does.
func testAccUniqueActiveUserEmail(t *testing.T) string {
	users, err := testClient.UsersV2ListWithResponse(context.Background(), &client.UsersV2ListParams{
		PageSize: lo.ToPtr(int64(250)),
	})
	if err != nil {
		t.Fatalf("listing users: %s", err)
	}
	if users.JSON200 == nil {
		t.Fatalf("listing users: %s", string(users.Body))
	}

	count := map[string]int{}
	for _, user := range users.JSON200.Users {
		if user.IsActive && user.Email != nil {
			count[*user.Email]++
		}
	}
	for _, user := range users.JSON200.Users {
		if user.IsActive && user.Email != nil && *user.Email != "" && count[*user.Email] == 1 {
			return *user.Email
		}
	}

	t.Skip("no active user with a unique email in the test account")
	return ""
}

// testAccSlackUserGroup returns the catalog type's name and one entry identifier for Slack
// user groups, or empty strings when the account has none.
//
// The type is managed by incident.io and synced from Slack, so unlike the catalog entries
// other tests create, this one can't be provisioned here: an account with no user groups
// gets the rest of the coverage and an empty group binding.
func testAccSlackUserGroup(t *testing.T) (string, string) {
	types, err := testClient.CatalogV3ListTypesWithResponse(context.Background())
	if err != nil {
		t.Fatalf("listing catalog types: %s", err)
	}
	if types.JSON200 == nil {
		t.Fatalf("listing catalog types: %s", string(types.Body))
	}

	catalogType, found := lo.Find(types.JSON200.CatalogTypes, func(catalogType client.CatalogTypeV3) bool {
		return catalogType.TypeName == "SlackUserGroup"
	})
	if !found {
		return "", ""
	}

	entries, err := testClient.CatalogV3ListEntriesWithResponse(context.Background(), &client.CatalogV3ListEntriesParams{
		CatalogTypeId: catalogType.Id,
		PageSize:      1,
	})
	if err != nil {
		t.Fatalf("listing catalog entries: %s", err)
	}
	if entries.JSON200 == nil || len(entries.JSON200.CatalogEntries) == 0 {
		return catalogType.Name, ""
	}

	return catalogType.Name, entries.JSON200.CatalogEntries[0].Name
}

// testAccIncidentWorkflowResourceConfigInviteUser mirrors the invite example: the person
// resolved by email, the Slack user group resolved through the catalog, and both bound to
// the arrays slack.invite_user declares.
func testAccIncidentWorkflowResourceConfigInviteUser(email, groupTypeName, groupIdentifier string) string {
	return testRunTemplate("incident_workflow_invite_user", `
data "incident_user" "security_lead" {
  email = {{ quote .Email }}
}

{{ if .GroupIdentifier }}
data "incident_catalog_type" "slack_user_group" {
  name = {{ quote .GroupTypeName }}
}

data "incident_catalog_entry" "security_responders" {
  catalog_type_id = data.incident_catalog_type.slack_user_group.id
  identifier      = {{ quote .GroupIdentifier }}
}
{{ end }}

resource "incident_workflow" "example" {
  name        = {{ stableSuffix "Invite responders workflow" | quote }}
  trigger     = "incident.updated"
  expressions = []
  condition_groups = [
    {
      conditions = [
        {
          subject        = "incident.status.category"
          operation      = "one_of"
          param_bindings = [{ array_value = [{ literal = "active" }] }]
        }
      ]
    }
  ]
  steps = [
    {
      id   = "01KFD47KKJS93YXR3W53MQSFVC"
      name = "slack.invite_user"
      param_bindings = [
        { value = { reference = "incident" } },
        { array_value = [{ literal = data.incident_user.security_lead.id }] },
        {{ if .GroupIdentifier }}{ array_value = [{ literal = data.incident_catalog_entry.security_responders.id }] }{{ else }}{}{{ end }}
      ]
    }
  ]
  once_for               = ["incident"]
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = ["standard"]
  state                  = "draft"
}
`, struct {
		Email           string
		GroupTypeName   string
		GroupIdentifier string
	}{Email: email, GroupTypeName: groupTypeName, GroupIdentifier: groupIdentifier})
}

// TestAccIncidentWorkflowResourceSlackPostMessageExample covers the example that posts to
// a Slack channel.
//
// Three things here are only checked when the workflow is saved. The channel binding is
// validated against Slack itself (StepSlackPostMessage.Validate), so a channel the bot
// can't see fails at apply rather than at run time. The message is a document rather than
// a string, and has to arrive at the TemplatedText parameter intact. And the step declares
// four parameters, two of them optional, so the bindings have to line up positionally.
func TestAccIncidentWorkflowResourceSlackPostMessageExample(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowResourceConfigSlackPostMessage(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "trigger", "incident.update_shared"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.name", "slack.post_message"),
					// Four bindings: channel, message, the threaded message left empty,
					// and the timezone.
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.#", "4"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.0.value.literal",
						channelID(false)),
					// The message is the document the data source parsed.
					resource.TestCheckResourceAttrPair(
						"incident_workflow.example", "steps.0.param_bindings.1.value.literal",
						"data.incident_rich_text.leadership_update", "json"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.param_bindings.3.value.literal",
						"Europe/London"),
					// Empty on purpose: every update should post, so there's nothing to
					// deduplicate on.
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "once_for.#", "0"),
				),
			},
			// The document survives an import round-trip. Rich text is stored as a literal
			// and compared semantically, so a re-encode on read shows up here.
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccIncidentWorkflowResourceConfigSlackPostMessage mirrors the Slack channel example.
// The one difference is the channel: the example hardcodes an illustrative ID, because
// Slack channels have no data source, and the test uses the same channel the alert route
// tests post to, which exists in the integration workspace.
//
// The message keeps every variable the example interpolates, so the references it teaches
// are exercised against the scope incident.update_shared actually builds.
func testAccIncidentWorkflowResourceConfigSlackPostMessage() string {
	return testRunTemplate("incident_workflow_slack_post_message", `
data "incident_rich_text" "leadership_update" {
  markdown    = <<-EOT
    **{{ "{{ incident.name }}" }}** — update from {{ "{{ update.author }}" }}

    {{ "{{ update.message }}" }}

    Now at {{ "{{ update.to_status }}" }}, severity {{ "{{ update.to_severity }}" }}.
  EOT
  feature_set = "mrkdwn"
}

resource "incident_workflow" "example" {
  name             = {{ stableSuffix "Share updates workflow" | quote }}
  trigger          = "incident.update_shared"
  expressions      = []
  condition_groups = []
  steps = [
    {
      id   = "01K5TSNDBF7KBZY9AFDKYXJM3M"
      name = "slack.post_message"
      param_bindings = [
        { value = { literal = {{ quote .ChannelID }} } },
        { value = { literal = data.incident_rich_text.leadership_update.json } },
        {},
        { value = { literal = "Europe/London" } }
      ]
    }
  ]
  once_for               = []
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = ["standard"]
  state                  = "draft"
}
`, struct{ ChannelID string }{ChannelID: channelID(false)})
}

// TestAccIncidentWorkflowResourceConditionGroupsExample covers the CSM subscription
// example: two condition groups, one of them holding two conditions, and a condition whose
// subject addresses a custom field by the ID a data source found.
//
// The grouping is the thing worth protecting. Conditions AND within a group and groups OR
// with each other, so writing the same three conditions as one group would match nothing -
// and nothing would fail, because a workflow that never matches is still a valid workflow.
// A round-trip assertion on the shape is the only thing that keeps it honest.
func TestAccIncidentWorkflowResourceConditionGroupsExample(t *testing.T) {
	// The lookup below runs before resource.Test, so honour TF_ACC ourselves rather than
	// calling the API during a unit test run, then initialise testClient.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}
	testAccPreCheck(t)

	email := testAccUniqueActiveUserEmail(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowResourceConfigConditionGroups(email),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Two groups, ORed, the second holding the two ANDed conditions.
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "condition_groups.#", "2"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "condition_groups.0.conditions.#", "1"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "condition_groups.1.conditions.#", "2"),
					// The custom field condition matches the option the data source
					// resolved, and addresses the field by the ID it found.
					resource.TestCheckResourceAttrPair(
						"incident_workflow.example",
						"condition_groups.1.conditions.1.param_bindings.0.array_value.0.literal",
						"data.incident_custom_field_option.all_customers", "id"),
					resource.TestCheckResourceAttrPair(
						"incident_workflow.example",
						"steps.0.param_bindings.1.array_value.0.literal",
						"data.incident_user.csm_lead", "id"),
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "steps.0.name",
						"incident.subscribe_user_to_incident"),
				),
			},
			// Both groups survive an import round-trip, in the order they were written.
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccIncidentWorkflowResourceConfigConditionGroups mirrors the CSM subscription
// example, with the account-specific values resolved rather than hardcoded.
//
// The example's custom field, its option and the person it subscribes all come from data
// sources, as they do here - the field and option created by this config first, and the
// user found by email. It subscribes one person where the example lists two, an array of
// one binding the same way.
//
// Severities are the exception: they have no data source, so the example pastes IDs and
// says to reference an incident_severity resource if you manage yours in Terraform. That
// is what the test does, which makes it a check on the escape hatch the example points at.
func testAccIncidentWorkflowResourceConfigConditionGroups(email string) string {
	return testRunTemplate("incident_workflow_condition_groups", `
resource "incident_severity" "critical" {
  name        = {{ stableSuffix "Critical" | quote }}
  description = "Everything is on fire"
}

resource "incident_severity" "major" {
  name        = {{ stableSuffix "Major" | quote }}
  description = "Something important is broken"
}

resource "incident_custom_field" "affected_customers" {
  name        = {{ stableSuffix "Affected customers" | quote }}
  description = "Who is affected by this incident?"
  field_type  = "single_select"
}

resource "incident_custom_field_option" "all_customers" {
  custom_field_id = incident_custom_field.affected_customers.id
  value           = "All customers"
}

data "incident_custom_field" "affected_customers" {
  name = incident_custom_field.affected_customers.name
}

data "incident_custom_field_option" "all_customers" {
  custom_field_id = data.incident_custom_field.affected_customers.id
  value           = incident_custom_field_option.all_customers.value
}

data "incident_user" "csm_lead" {
  email = {{ quote .Email }}
}

resource "incident_workflow" "example" {
  name        = {{ stableSuffix "Condition groups workflow" | quote }}
  trigger     = "incident.updated"
  expressions = []
  condition_groups = [
    {
      conditions = [
        {
          subject   = "incident.severity"
          operation = "one_of"
          param_bindings = [
            { array_value = [{ literal = incident_severity.critical.id }] }
          ]
        }
      ]
    },
    {
      conditions = [
        {
          subject   = "incident.severity"
          operation = "one_of"
          param_bindings = [
            { array_value = [{ literal = incident_severity.major.id }] }
          ]
        },
        {
          subject   = "incident.custom_field[\"${data.incident_custom_field.affected_customers.id}\"]"
          operation = "one_of"
          param_bindings = [
            { array_value = [{ literal = data.incident_custom_field_option.all_customers.id }] }
          ]
        }
      ]
    }
  ]
  steps = [
    {
      id   = "01KDEMWQCS30P3GHAVNG9VGKWS"
      name = "incident.subscribe_user_to_incident"
      param_bindings = [
        { value = { reference = "incident" } },
        { array_value = [{ literal = data.incident_user.csm_lead.id }] }
      ]
    }
  ]
  once_for               = ["incident"]
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = ["standard"]
  state                  = "draft"
}
`, struct{ Email string }{Email: email})
}
