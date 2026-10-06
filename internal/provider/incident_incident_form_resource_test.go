package provider

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"
)

const incidentFormAddress = "incident_incident_form.test"

// incidentFormTestConfig renders an incident_incident_form config. Elements is HCL for the
// lifecycle_elements list body; the resource IDs it references come from the caller, so the
// same steps can run against literal IDs on the fake API and real resources on an account.
type incidentFormTestConfig struct {
	Prelude        string
	FormType       string
	IncidentTypeID string
	Expressions    string
	Elements       string
	WithDataSource bool
}

func testAccIncidentFormConfig(config incidentFormTestConfig) string {
	// The prelude joins the template source rather than being substituted into it, so
	// template calls inside it, like stableSuffix, are rendered too. The provider is named
	// explicitly for the tests that import before anything else: see testProviderRequirement.
	return testRunTemplate("incident_incident_form", testProviderRequirement()+config.Prelude+`
resource "incident_incident_form" "test" {
  form_type = {{ .FormType | quote }}
{{ if .IncidentTypeID }}  incident_type_id = {{ .IncidentTypeID }}
{{ end }}{{ if .Expressions }}  expressions = {{ .Expressions }}
{{ end }}{{ if .Elements }}  lifecycle_elements = {{ .Elements }}
{{ end }}}
{{ if .WithDataSource }}
data "incident_incident_form" "test" {
  id = incident_incident_form.test.id
}
{{ end }}
`, config)
}

// fakeIncidentFormIDs are the literal IDs the fake API tests reference. The fake doesn't check
// that they exist.
const (
	fakeIncidentTypeID = `"01FAKEINCIDENTTYPE000000000"`
	fakeCustomField1   = `"01FAKECUSTOMFIELD0000000001"`
	fakeCustomField2   = `"01FAKECUSTOMFIELD0000000002"`
)

// testIncidentFormLifecycleSteps is the life of a form for an incident type: created with a
// mix of keyed and unkeyed elements and read through the data source, imported, reordered
// with elements added and dropped, written with a pinned element out of place, and emptied.
func testIncidentFormLifecycleSteps(fake *fakeIncidentFormsAPI) []resource.TestStep {
	var formID, customFieldElementID, textElementID string

	return []resource.TestStep{
		{
			Config: testAccIncidentFormConfig(incidentFormTestConfig{
				FormType:       "declare",
				IncidentTypeID: fakeIncidentTypeID,
				WithDataSource: true,
				Elements: `[
    { element_type = "name" },
    {
      element_type    = "custom_field"
      custom_field_id = ` + fakeCustomField1 + `
      required_if     = "always_require"
      placeholder     = "Which team owns this?"
      description     = "The team **paged** for this incident."
      default_value   = { value_literal = "Payments" }
    },
    {
      element_type = "text"
      description  = "Fill in as much as you know."
    },
    { element_type = "divider" },
    {
      element_type = "severity"
      show_if_condition_groups = [
        {
          conditions = [
            {
              subject        = "incident.incident_type"
              operation      = "one_of"
              param_bindings = [{ array_value = [{ literal = ` + fakeIncidentTypeID + ` }] }]
            }
          ]
        }
      ]
    },
  ]`,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(incidentFormAddress, "id"),
				resource.TestCheckResourceAttr(incidentFormAddress, "form_type", "declare"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", "5"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.0.element_type", "name"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.0.required_if", "never_require"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.0.can_select_no_value", "false"),
				resource.TestCheckNoResourceAttr(incidentFormAddress, "lifecycle_elements.0.config"),
				resource.TestCheckNoResourceAttr(incidentFormAddress, "lifecycle_elements.0.show_if_condition_groups"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.element_type", "custom_field"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.required_if", "always_require"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.placeholder", "Which team owns this?"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.description", "The team **paged** for this incident."),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.default_value.value_literal", "Payments"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.2.element_type", "text"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.3.element_type", "divider"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.4.element_type", "severity"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.4.show_if_condition_groups.0.conditions.0.operation", "one_of"),
				resource.TestCheckResourceAttrSet(incidentFormAddress, "lifecycle_elements.1.id"),
				resource.TestCheckResourceAttrSet(incidentFormAddress, "lifecycle_elements.2.id"),
				// The data source reads the same form.
				resource.TestCheckResourceAttrPair("data.incident_incident_form.test", "id", incidentFormAddress, "id"),
				resource.TestCheckResourceAttr("data.incident_incident_form.test", "lifecycle_elements.#", "5"),
				resource.TestCheckResourceAttr("data.incident_incident_form.test", "lifecycle_elements.1.default_value.value.literal", "Payments"),
				resource.TestCheckNoResourceAttr("data.incident_incident_form.test", "lifecycle_elements.1.config"),
				rememberAttr("id", &formID),
				rememberAttr("lifecycle_elements.1.id", &customFieldElementID),
				rememberAttr("lifecycle_elements.2.id", &textElementID),
				func(_ *terraform.State) error {
					if annotations, ok := fake.claimedBy(formID); !ok || annotations["incident.io/terraform/version"] == "" {
						return fmt.Errorf("expected the form to be claimed for Terraform, got %v", annotations)
					}
					return nil
				},
			),
		},
		{
			ResourceName:      incidentFormAddress,
			ImportState:       true,
			ImportStateVerify: true,
			// An import can't know a binding was written with the value_literal shorthand.
			ImportStateVerifyIgnore: []string{"lifecycle_elements.1.default_value"},
		},
		{
			// Reorder, add a custom field and a second text element, drop the divider. The
			// elements that stayed keep their IDs: the custom field by its key, the text by
			// position among the text elements.
			Config: testAccIncidentFormConfig(incidentFormTestConfig{
				FormType:       "declare",
				IncidentTypeID: fakeIncidentTypeID,
				Elements: `[
    { element_type = "name" },
    { element_type = "severity" },
    {
      element_type = "text"
      description  = "Fill in as much as you know."
    },
    {
      element_type    = "custom_field"
      custom_field_id = ` + fakeCustomField1 + `
      description     = "The team **paged** for this incident."
    },
    {
      element_type    = "custom_field"
      custom_field_id = ` + fakeCustomField2 + `
      required_if     = "check_engine_config"
      required_if_condition_groups = [
        {
          conditions = [
            {
              subject        = "incident.severity"
              operation      = "is_set"
              param_bindings = []
            }
          ]
        }
      ]
    },
    {
      element_type = "text"
      description  = "Anything else?"
    },
  ]`,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", "6"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.element_type", "severity"),
				resource.TestCheckNoResourceAttr(incidentFormAddress, "lifecycle_elements.1.show_if_condition_groups"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.2.element_type", "text"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.3.element_type", "custom_field"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.3.required_if", "never_require"),
				resource.TestCheckNoResourceAttr(incidentFormAddress, "lifecycle_elements.3.placeholder"),
				resource.TestCheckNoResourceAttr(incidentFormAddress, "lifecycle_elements.3.default_value"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.4.required_if", "check_engine_config"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.4.required_if_condition_groups.0.conditions.0.operation", "is_set"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.5.description", "Anything else?"),
				func(state *terraform.State) error {
					attrs := state.RootModule().Resources[incidentFormAddress].Primary.Attributes
					if got := attrs["lifecycle_elements.3.id"]; got != customFieldElementID {
						return fmt.Errorf("custom field element changed ID: was %s, now %s", customFieldElementID, got)
					}
					if got := attrs["lifecycle_elements.2.id"]; got != textElementID {
						return fmt.Errorf("text element changed ID: was %s, now %s", textElementID, got)
					}
					return nil
				},
			),
		},
		{
			// A pinned element listed after another: the API saves it first, and state keeps
			// the order the configuration gave, so the apply is consistent.
			Config: testAccIncidentFormConfig(incidentFormTestConfig{
				FormType:       "declare",
				IncidentTypeID: fakeIncidentTypeID,
				Elements: `[
    {
      element_type    = "custom_field"
      custom_field_id = ` + fakeCustomField1 + `
    },
    { element_type = "name" },
  ]`,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", "2"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.0.element_type", "custom_field"),
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.element_type", "name"),
				func(_ *terraform.State) error {
					ids := fake.elementIDs(formID)
					if len(ids) != 2 || ids[1] != customFieldElementID {
						return fmt.Errorf("expected the API to list name first and keep the custom field's ID, got %v", ids)
					}
					return nil
				},
			),
		},
		{
			Config: testAccIncidentFormConfig(incidentFormTestConfig{
				FormType:       "declare",
				IncidentTypeID: fakeIncidentTypeID,
				Expressions:    "[]",
				Elements:       "[]",
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", "0"),
				resource.TestCheckResourceAttr(incidentFormAddress, "expressions.#", "0"),
			),
		},
	}
}

// rememberAttr stores one of the form's attributes for a later step to compare against.
func rememberAttr(attribute string, into *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		res, ok := state.RootModule().Resources[incidentFormAddress]
		if !ok {
			return fmt.Errorf("%s not in state", incidentFormAddress)
		}
		*into = res.Primary.Attributes[attribute]

		return nil
	}
}

func fakeIncidentFormsTestCase(t *testing.T) *fakeIncidentFormsAPI {
	t.Helper()

	fake, url := startFakeIncidentFormsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	return fake
}

// TestIncidentIncidentFormResourceLifecycle runs the lifecycle against a fake API, so it
// needs no account.
func TestIncidentIncidentFormResourceLifecycle(t *testing.T) {
	fake := fakeIncidentFormsTestCase(t)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testIncidentFormLifecycleSteps(fake),
	})
}

// TestIncidentIncidentFormResourceDefaultForm imports a default form, which can't be created,
// and destroys it, which can't archive it: the form leaves state and goes back to the
// dashboard.
func TestIncidentIncidentFormResourceDefaultForm(t *testing.T) {
	fake := fakeIncidentFormsTestCase(t)
	declare := fake.defaultForm("declare")

	config := testAccIncidentFormConfig(incidentFormTestConfig{
		FormType: "declare",
		Elements: `[
    { element_type = "name" },
    { element_type = "severity" },
    { element_type = "summary", placeholder = "What do we know so far?" },
  ]`,
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             config,
				ResourceName:       incidentFormAddress,
				ImportState:        true,
				ImportStateId:      declare.Id,
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 || states[0].Attributes["form_type"] != "declare" || states[0].Attributes["lifecycle_elements.#"] != "3" {
						return fmt.Errorf("unexpected imported state: %v", states)
					}
					return nil
				},
			},
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(incidentFormAddress, "id", declare.Id),
					resource.TestCheckNoResourceAttr(incidentFormAddress, "incident_type_id"),
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.2.placeholder", "What do we know so far?"),
				),
			},
		},
	})

	annotations, ok := fake.claimedBy(declare.Id)
	if !ok {
		t.Fatalf("expected the default form to have been claimed and then handed back")
	}
	if len(annotations) != 0 {
		t.Fatalf("expected destroying the default form to hand it back to the dashboard, but it is still claimed: %v", annotations)
	}
	if fake.defaultForm("declare").Id != declare.Id {
		t.Fatalf("expected the default form to survive a destroy")
	}
}

// incidentFormLifecycleTypes are the form types every organisation has a default of.
var incidentFormLifecycleTypes = []string{"declare", "accept", "update", "resolve", "retrospective"}

// incidentFormTypes is every form type the resource manages.
var incidentFormTypes = append(append([]string{}, incidentFormLifecycleTypes...), "custom-fields")

// TestIncidentIncidentFormResourceEveryFormType creates a form of each type for an incident
// type. A custom field is the one element every form type accepts, so each form carries one.
func TestIncidentIncidentFormResourceEveryFormType(t *testing.T) {
	for _, formType := range incidentFormTypes {
		t.Run(formType, func(t *testing.T) {
			fakeIncidentFormsTestCase(t)

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testAccIncidentFormConfig(incidentFormTestConfig{
							FormType:       formType,
							IncidentTypeID: fakeIncidentTypeID,
							WithDataSource: true,
							Elements: `[{
    element_type    = "custom_field"
    custom_field_id = ` + fakeCustomField1 + `
  }]`,
						}),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(incidentFormAddress, "form_type", formType),
							resource.TestCheckResourceAttr(incidentFormAddress, "incident_type_id", strings.Trim(fakeIncidentTypeID, `"`)),
							resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", "1"),
							resource.TestCheckResourceAttr("data.incident_incident_form.test", "form_type", formType),
						),
					},
					{
						ResourceName:      incidentFormAddress,
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}

// TestIncidentIncidentFormResourceEveryDefaultForm imports and then destroys the default form
// of each lifecycle type, which hands it back rather than archiving it.
func TestIncidentIncidentFormResourceEveryDefaultForm(t *testing.T) {
	for _, formType := range incidentFormLifecycleTypes {
		t.Run(formType, func(t *testing.T) {
			fake := fakeIncidentFormsTestCase(t)
			form := fake.defaultForm(formType)
			elements := lo.Map(fakeDefaultElements(formType), func(elementType string, _ int) string {
				return fmt.Sprintf("{ element_type = %q }", elementType)
			})
			config := testAccIncidentFormConfig(incidentFormTestConfig{
				FormType: formType,
				Elements: "[" + strings.Join(elements, ", ") + "]",
			})

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:             config,
						ResourceName:       incidentFormAddress,
						ImportState:        true,
						ImportStateId:      form.Id,
						ImportStatePersist: true,
					},
					{
						Config: config,
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(incidentFormAddress, "id", form.Id),
							resource.TestCheckResourceAttr(incidentFormAddress, "form_type", formType),
							resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", fmt.Sprint(len(elements))),
						),
					},
				},
			})

			if annotations, ok := fake.claimedBy(form.Id); !ok || len(annotations) != 0 {
				t.Fatalf("expected the default %s form to be handed back on destroy, got %v", formType, annotations)
			}
			if fake.defaultForm(formType).Id != form.Id {
				t.Fatalf("expected the default %s form to survive a destroy", formType)
			}
		})
	}
}

// TestIncidentIncidentFormResourceSeveralForms manages several forms at once: the same type
// for two incident types, and two types for one incident type. Each is its own form.
func TestIncidentIncidentFormResourceSeveralForms(t *testing.T) {
	fakeIncidentFormsTestCase(t)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "incident_incident_form" "security_declare" {
  form_type        = "declare"
  incident_type_id = "01FAKEINCIDENTTYPE0SECURITY"
  lifecycle_elements = [
    { element_type = "name" },
    { element_type = "severity", required_if = "always_require" },
  ]
}

resource "incident_incident_form" "payments_declare" {
  form_type        = "declare"
  incident_type_id = "01FAKEINCIDENTTYPE0PAYMENTS"
  lifecycle_elements = [
    { element_type = "name" },
    { element_type = "summary" },
  ]
}

resource "incident_incident_form" "payments_update" {
  form_type        = "update"
  incident_type_id = "01FAKEINCIDENTTYPE0PAYMENTS"
  lifecycle_elements = [
    { element_type = "status" },
    { element_type = "update_message" },
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_incident_form.security_declare", "lifecycle_elements.1.required_if", "always_require"),
					resource.TestCheckResourceAttr("incident_incident_form.payments_declare", "lifecycle_elements.1.element_type", "summary"),
					resource.TestCheckResourceAttr("incident_incident_form.payments_update", "lifecycle_elements.0.element_type", "status"),
					func(state *terraform.State) error {
						ids := map[string]bool{}
						for _, name := range []string{"security_declare", "payments_declare", "payments_update"} {
							ids[state.RootModule().Resources["incident_incident_form."+name].Primary.ID] = true
						}
						if len(ids) != 3 {
							return fmt.Errorf("expected three distinct forms, got IDs %v", ids)
						}
						return nil
					},
				),
			},
			{
				// A second form for a pair that already has one is rejected at plan.
				Config: `
resource "incident_incident_form" "security_declare" {
  form_type        = "declare"
  incident_type_id = "01FAKEINCIDENTTYPE0SECURITY"
  lifecycle_elements = [{ element_type = "name" }]
}

resource "incident_incident_form" "security_declare_again" {
  form_type        = "declare"
  incident_type_id = "01FAKEINCIDENTTYPE0SECURITY"
  lifecycle_elements = [{ element_type = "name" }]
}
`,
				ExpectError: wrapRe("A form of this type already exists for this incident type"),
			},
		},
	})
}

// TestIncidentIncidentFormResourceReplace moves a form to another incident type, which
// replaces it. The new form gets its own element IDs rather than inheriting the old ones,
// which the API would reject as belonging to another form.
func TestIncidentIncidentFormResourceReplace(t *testing.T) {
	fakeIncidentFormsTestCase(t)
	var formID, textID string

	elements := `[
    { element_type = "name" },
    { element_type = "text", description = "Read me first." },
  ]`

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentFormConfig(incidentFormTestConfig{
					FormType:       "declare",
					IncidentTypeID: fakeIncidentTypeID,
					Elements:       elements,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					rememberAttr("id", &formID),
					rememberAttr("lifecycle_elements.1.id", &textID),
				),
			},
			{
				Config: testAccIncidentFormConfig(incidentFormTestConfig{
					FormType:       "declare",
					IncidentTypeID: `"01FAKEINCIDENTTYPE000000002"`,
					Elements:       elements,
				}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(incidentFormAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(incidentFormAddress, "incident_type_id", "01FAKEINCIDENTTYPE000000002"),
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.description", "Read me first."),
					func(state *terraform.State) error {
						attrs := state.RootModule().Resources[incidentFormAddress].Primary.Attributes
						if attrs["id"] == formID {
							return fmt.Errorf("expected a new form, got the old one %s", formID)
						}
						if attrs["lifecycle_elements.1.id"] == textID {
							return fmt.Errorf("expected the new form's text element to have its own ID, got the old one %s", textID)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestIncidentIncidentFormResourceCreateDefaultForm checks a configuration for a default form
// of any lifecycle type fails at plan, naming the form to import.
func TestIncidentIncidentFormResourceCreateDefaultForm(t *testing.T) {
	for _, formType := range incidentFormLifecycleTypes {
		t.Run(formType, func(t *testing.T) {
			fake := fakeIncidentFormsTestCase(t)

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testAccIncidentFormConfig(incidentFormTestConfig{
							FormType: formType,
							Elements: `[{ element_type = "name" }]`,
						}),
						ExpectError: wrapRe("(?s)Default incident form already exists.*terraform import <address> " + fake.defaultForm(formType).Id),
					},
				},
			})
		})
	}
}

// TestIncidentIncidentFormResourceValidateConfig covers the element rules checked without an
// account.
func TestIncidentIncidentFormResourceValidateConfig(t *testing.T) {
	fakeIncidentFormsTestCase(t)

	cases := []struct {
		name     string
		elements string
		wantErr  string
	}{
		{
			name:     "custom field without its ID",
			elements: `[{ element_type = "custom_field" }]`,
			wantErr:  "`custom_field_id` is required when `element_type` is \"custom_field\"",
		},
		{
			name:     "role ID on a severity element",
			elements: `[{ element_type = "severity", incident_role_id = "01FAKE" }]`,
			wantErr:  "`incident_role_id` must not be set when `element_type` is \"severity\"",
		},
		{
			name: "required conditions without check_engine_config",
			elements: `[{
    element_type = "summary"
    required_if  = "always_require"
    required_if_condition_groups = [{ conditions = [] }]
  }]`,
			wantErr: "`required_if_condition_groups` only applies when `required_if` is \"check_engine_config\"",
		},
		{
			name: "required conditions with required_if left out",
			elements: `[{
    element_type = "summary"
    required_if_condition_groups = [{ conditions = [] }]
  }]`,
			wantErr: "`required_if_condition_groups` only applies when `required_if` is \"check_engine_config\"",
		},
		{
			name:     "unknown element type",
			elements: `[{ element_type = "mood" }]`,
			wantErr:  "element_type value must be one of",
		},
		{
			name:     "unknown form type is rejected by the schema",
			elements: `[]`,
			wantErr:  "form_type value must be one of",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			formType := "declare"
			if tc.name == "unknown form type is rejected by the schema" {
				formType = "escalate"
			}

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testAccIncidentFormConfig(incidentFormTestConfig{
							FormType:       formType,
							IncidentTypeID: fakeIncidentTypeID,
							Elements:       tc.elements,
						}),
						PlanOnly:    true,
						ExpectError: wrapRe(regexp.QuoteMeta(tc.wantErr)),
					},
				},
			})
		})
	}
}

// TestIncidentIncidentFormResourceInvalidAtPlan checks the API's own validation runs at plan
// time: this ID names a different element, which the API rejects.
func TestIncidentIncidentFormResourceInvalidAtPlan(t *testing.T) {
	fakeIncidentFormsTestCase(t)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentFormConfig(incidentFormTestConfig{
					FormType:       "declare",
					IncidentTypeID: fakeIncidentTypeID,
					Elements: `[
    { element_type = "name" },
    { element_type = "name" },
  ]`,
				}),
				PlanOnly:    true,
				ExpectError: wrapRe("(?s)Invalid incident form configuration.*This element appears more than once on the form"),
			},
		},
	})
}

// TestIncidentIncidentFormDataSource reads a seeded default form by ID.
func TestIncidentIncidentFormDataSource(t *testing.T) {
	fake := fakeIncidentFormsTestCase(t)
	update := fake.defaultForm("update")

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "incident_incident_form" "update" {
  id = %q
}
`, update.Id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_incident_form.update", "form_type", "update"),
					resource.TestCheckNoResourceAttr("data.incident_incident_form.update", "incident_type_id"),
					resource.TestCheckResourceAttr("data.incident_incident_form.update", "lifecycle_elements.#", "3"),
					resource.TestCheckResourceAttr("data.incident_incident_form.update", "lifecycle_elements.0.element_type", "status"),
					resource.TestCheckResourceAttr("data.incident_incident_form.update", "lifecycle_elements.1.element_type", "update_message"),
					resource.TestCheckResourceAttr("data.incident_incident_form.update", "lifecycle_elements.1.required_if", "never_require"),
				),
			},
			{
				Config: `
data "incident_incident_form" "missing" {
  id = "01FAKEMISSING"
}
`,
				ExpectError: wrapRe("Unable to read incident form"),
			},
		},
	})
}

// TestAccIncidentIncidentFormResource runs against a real account. Every organisation has a
// default form of each lifecycle type already, so the form a test can own outright is one
// for an incident type, which the API creates and archives like any other. It needs
// TF_ACC_INCIDENT_TYPE_NAME to name an incident type with no customised forms, and each CI
// leg needs a type of its own, since a type has one form of each kind and the legs run at
// the same time. It skips without one.
func TestAccIncidentIncidentFormResource(t *testing.T) {
	incidentTypeName := os.Getenv("TF_ACC_INCIDENT_TYPE_NAME")
	if incidentTypeName == "" {
		t.Skip("No TF_ACC_INCIDENT_TYPE_NAME environment variable set, skipping")
	}

	prelude := fmt.Sprintf(`
data "incident_incident_type" "test" {
  name = %q
}
`, incidentTypeName) + `
resource "incident_custom_field" "team" {
  name        = {{ stableSuffix "Form team" | quote }}
  description = "The team that owns the affected service."
  field_type  = "text"
}

resource "incident_custom_field" "ticket" {
  name        = {{ stableSuffix "Form ticket" | quote }}
  description = "The ticket tracking this incident."
  field_type  = "text"
}
`
	var teamElementID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentFormConfig(incidentFormTestConfig{
					Prelude:        prelude,
					FormType:       "custom-fields",
					IncidentTypeID: "data.incident_incident_type.test.id",
					WithDataSource: true,
					Elements: `[
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.team.id
      required_if     = "always_require"
      placeholder     = "Which team owns this?"
    },
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.ticket.id
    },
  ]`,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(incidentFormAddress, "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr(incidentFormAddress, "form_type", "custom-fields"),
					resource.TestCheckResourceAttrPair(incidentFormAddress, "incident_type_id", "data.incident_incident_type.test", "id"),
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", "2"),
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.0.required_if", "always_require"),
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.0.placeholder", "Which team owns this?"),
					resource.TestCheckResourceAttrPair(incidentFormAddress, "lifecycle_elements.0.custom_field_id", "incident_custom_field.team", "id"),
					resource.TestCheckResourceAttrPair("data.incident_incident_form.test", "id", incidentFormAddress, "id"),
					resource.TestCheckResourceAttr("data.incident_incident_form.test", "lifecycle_elements.#", "2"),
					rememberAttr("lifecycle_elements.0.id", &teamElementID),
				),
			},
			{
				ResourceName:      incidentFormAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Swap the order and describe the team field: it keeps its ID, matched on its
				// custom field.
				Config: testAccIncidentFormConfig(incidentFormTestConfig{
					Prelude:        prelude,
					FormType:       "custom-fields",
					IncidentTypeID: "data.incident_incident_type.test.id",
					Elements: `[
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.ticket.id
    },
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.team.id
      description     = "The team **paged** for this incident."
    },
  ]`,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.#", "2"),
					resource.TestCheckResourceAttrPair(incidentFormAddress, "lifecycle_elements.0.custom_field_id", "incident_custom_field.ticket", "id"),
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.required_if", "never_require"),
					resource.TestCheckResourceAttr(incidentFormAddress, "lifecycle_elements.1.description", "The team **paged** for this incident."),
					func(state *terraform.State) error {
						got := state.RootModule().Resources[incidentFormAddress].Primary.Attributes["lifecycle_elements.1.id"]
						if got != teamElementID {
							return fmt.Errorf("team element changed ID: was %s, now %s", teamElementID, got)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccIncidentIncidentFormResourceEveryFormType creates a form of each type for one
// incident type on a real account. It needs TF_ACC_INCIDENT_TYPE_NAME to name an incident
// type that has no customised forms yet, since the API refuses a second form for a pair, so
// it skips without one.
func TestAccIncidentIncidentFormResourceEveryFormType(t *testing.T) {
	incidentTypeName := os.Getenv("TF_ACC_INCIDENT_TYPE_NAME")
	if incidentTypeName == "" {
		t.Skip("No TF_ACC_INCIDENT_TYPE_NAME environment variable set, skipping")
	}

	forms := lo.Map(incidentFormTypes, func(formType string, _ int) string {
		return fmt.Sprintf(`
resource "incident_incident_form" %q {
  form_type        = %q
  incident_type_id = data.incident_incident_type.test.id
  lifecycle_elements = [
    {
      element_type    = "custom_field"
      custom_field_id = incident_custom_field.team.id
    },
  ]
}
`, strings.ReplaceAll(formType, "-", "_"), formType)
	})

	config := testRunTemplate("incident_incident_form_every_type", `
data "incident_incident_type" "test" {
  name = {{ .IncidentTypeName | quote }}
}

resource "incident_custom_field" "team" {
  name        = {{ stableSuffix "Form type team" | quote }}
  description = "The team that owns the affected service."
  field_type  = "text"
}
`+strings.Join(forms, ""), map[string]string{"IncidentTypeName": incidentTypeName})

	checks := lo.Map(incidentFormTypes, func(formType string, _ int) resource.TestCheckFunc {
		address := "incident_incident_form." + strings.ReplaceAll(formType, "-", "_")

		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(address, "form_type", formType),
			resource.TestCheckResourceAttrPair(address, "incident_type_id", "data.incident_incident_type.test", "id"),
			resource.TestCheckResourceAttr(address, "lifecycle_elements.#", "1"),
		)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.ComposeAggregateTestCheckFunc(checks...),
			},
		},
	})
}
