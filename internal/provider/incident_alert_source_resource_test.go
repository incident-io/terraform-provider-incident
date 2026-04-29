package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccAlertSourceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccAlertSourceResourceConfig("test-source", "datadog"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "datadog"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "id"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "secret_token"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing
			{
				Config: testAccAlertSourceResourceConfig("updated-source", "datadog"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "updated-source"),
				),
			},
			// Test full configuration with template
			{
				Config: testAccAlertSourceResourceConfigFull(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "full-test-source"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "template.title.literal"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "template.description.literal"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "template.attributes.#", "1"),
					resource.TestCheckResourceAttrPair("incident_alert_source.test", "template.attributes.0.alert_attribute_id", "incident_alert_attribute.test", "id"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "template.attributes.0.binding.value.reference", `expressions["severity_expr"]`),
				),
			},
		},
	})
}

func testRunTemplate(tmplName, source string, args any) string {
	tmpl := template.Must(template.New(tmplName).Funcs(sprig.TxtFuncMap()).Parse(source))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, args)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func testAccAlertSourceResourceConfig(name string, sourceType string) string {
	return testRunTemplate("incident_alert_source", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = {{ quote .SourceType }}

  template = {
    expressions = [],
    title = {
      literal = {{ quote .Title }}
    },
    description = {
      literal = {{ quote .Description }}
    },
    attributes = []
  }
}
`, struct {
		Name, SourceType, Title, Description string
	}{
		Name:        name,
		SourceType:  sourceType,
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

func testAccAlertSourceResourceConfigWithJira(name string, projectIDs []string) string {
	return testRunTemplate("incident_alert_source_jira", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "jira"

  template = {
    expressions = [],
    title = {
      literal = {{ quote .Title }}
    },
    description = {
      literal = {{ quote .Description }}
    },
    attributes = []
  }

  jira_options = {
    project_ids = [
      {{ range .ProjectIDs }}
      {{ quote . }},
      {{ end }}
    ]
  }
}
`, struct {
		Name        string
		Title       string
		Description string
		ProjectIDs  []string
	}{
		Name:        name,
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
		ProjectIDs:  projectIDs,
	})
}

func testAccAlertSourceResourceConfigFull() string {
	return testRunTemplate("incident_alert_source_full", `
resource "incident_alert_attribute" "test" {
  name = "test-attribute"
  type = "String"
  array = false
}

resource "incident_alert_source" "test" {
  name        = "full-test-source"
  source_type = "datadog"

  template = {
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = [{
      alert_attribute_id = incident_alert_attribute.test.id
      binding = {
        value = {
          reference = "expressions[\"severity_expr\"]"
        }
      }
    }]

    expressions = [{
      label = "Severity"
      reference = "severity_expr"
      root_reference = "payload"
      operations = [{
        operation_type = "parse"
        parse = {
          source = "$.metadata.severity"
          returns = {
            type  = "String"
            array = false
          }
        }
      }]
    }]
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

// TestAccAlertSourceResource_Heartbeat checks that heartbeat_options work.
func TestAccAlertSourceResource_Heartbeat(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceResourceConfigWithHeartbeat("heartbeat-source", 60),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "heartbeat-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "heartbeat"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "heartbeat_options.interval_seconds", "60"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "heartbeat_options.failure_threshold"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "heartbeat_options.grace_period_seconds"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "heartbeat_options.ping_url"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccAlertSourceResourceConfigWithHeartbeat(name string, intervalSeconds int) string {
	return testRunTemplate("incident_alert_source_heartbeat", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "heartbeat"

  template = {
    expressions = [],
    title       = {},
    description = {},
    attributes  = []
  }

  heartbeat_options = {
    interval_seconds = {{ .IntervalSeconds }}
  }
}
`, struct {
		Name            string
		IntervalSeconds int
	}{
		Name:            name,
		IntervalSeconds: intervalSeconds,
	})
}

// TestAccAlertSourceResource_Jira checks that the jira_options work.
//
// NOTE: this only runs if TF_ACC_JIRA is in your environment, since it requires
// the Jira integration to be installed in the target account.
func TestAccAlertSourceResource_Jira(t *testing.T) {
	if os.Getenv("TF_ACC_JIRA") == "" {
		t.Skip("TF_ACC_JIRA is not set: skipping Jira-specific test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{

			// Add a Jira one
			{
				// This is the default project in our dev account
				Config: testAccAlertSourceResourceConfigWithJira("jira-source", []string{"46a0db2b-17d4-48c1-961e-563d87797b5c/10000"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "jira-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "jira_options.project_ids.#", "1"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "jira_options.project_ids.0", "46a0db2b-17d4-48c1-961e-563d87797b5c/10000"),
				),
			},
		},
	})
}

// TestAccAlertSourceResource_HTTPCustom checks that http_custom_options work.
func TestAccAlertSourceResource_HTTPCustom(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceResourceConfigWithHTTPCustom("http-custom-source"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "http-custom-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "http_custom"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "http_custom_options.transform_expression"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "http_custom_options.deduplication_key_path", "$.alert_id"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "alert_events_url"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update name
			{
				Config: testAccAlertSourceResourceConfigWithHTTPCustom("http-custom-source-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "http-custom-source-updated"),
				),
			},
		},
	})
}

func testAccAlertSourceResourceConfigWithHTTPCustom(name string) string {
	return testRunTemplate("incident_alert_source_http_custom", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "http_custom"

  http_custom_options = {
    transform_expression   = "({ title: payload.alert_name, description: payload.message, status: payload.status === 'resolved' ? 'resolved' : 'firing' })"
    deduplication_key_path = "$.alert_id"
  }

  template = {
    expressions = []
    title       = { reference = "payload.alert_name" }
    description = { reference = "payload.message" }
    attributes  = []
  }
}
`, struct{ Name string }{Name: name})
}

// TestAccAlertSourceResource_ValidationErrors checks that we return helpful
// validation errors when possible.
func TestAccAlertSourceResource_ValidationErrors(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Test invalid source_type value
				Config: `
resource "incident_alert_source" "test" {
  name        = "test-source"
  source_type = "not-a-real-source"

  template = {
    expressions = []
    title       = { literal = "test" }
    description = { literal = "test" }
    attributes  = []
  }
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`"not-a-real-source" is not a valid value`),
			},
			{
				// Test
				Config: testRunTemplate("incident_alert_source_invalid", `
resource "incident_alert_source" "test" {
  name = "Not Jira, but with Jira options"
  source_type = "datadog"

  template = {
    expressions = [],
    title = {
      literal = {{ quote .Title }}
    },
    description = {
      literal = {{ quote .Description }}
    },
    attributes = []
  }

  jira_options = {
    project_ids = ["my-project"]
  }
}
`, struct{ Title, Description string }{
					Title:       testAlertSourceTitle,
					Description: testAlertSourceDescription,
				}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("jira_options can only be set when source_type is jira"),
			},
			{
				// Test heartbeat_options with wrong source_type
				Config: testRunTemplate("incident_alert_source_invalid_heartbeat", `
resource "incident_alert_source" "test" {
  name = "Not Heartbeat, but with heartbeat options"
  source_type = "datadog"

  template = {
    expressions = [],
    title = {
      literal = {{ quote .Title }}
    },
    description = {
      literal = {{ quote .Description }}
    },
    attributes = []
  }

  heartbeat_options = {
    interval_seconds = 60
  }
}
`, struct{ Title, Description string }{
					Title:       testAlertSourceTitle,
					Description: testAlertSourceDescription,
				}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("heartbeat_options can only be set when source_type is heartbeat"),
			},
			{
				// Test missing required template fields
				Config: testRunTemplate("incident_alert_source_invalid", `
resource "incident_alert_source" "test" {
  name        = "test-source"
  source_type = "datadog"
  template = {
    # Missing required title
    description = {
      literal = {{ quote .Description }}
    }
  }
}
`, struct{ Description string }{Description: testAlertSourceDescription}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("required"),
			},
			{
				// Test visible_to_teams without is_private=true
				Config:      testAccAlertSourceResourceConfigVisibleToTeamsWithoutPrivate(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("visible_to_teams can only be set when is_private is true"),
			},
			{
				// Test is_private=true without visible_to_teams
				Config:      testAccAlertSourceResourceConfigPrivateWithoutTeams(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("visible_to_teams must be set when is_private is true"),
			},
			{
				// Test branches operation with invalid root_reference
				Config:      testAccAlertSourceResourceConfigInvalidBranches(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("Invalid root_reference for branches operation"),
			},
		},
	})
}

func testAccAlertSourceResourceConfigInvalidBranches() string {
	return fmt.Sprintf(`
resource "incident_alert_source" "invalid_branches" {
  name        = "invalid-branches-test"
  source_type = "http"

  template = {
    title = {
      literal = %[1]q
    }
    description = {
      literal = %[2]q
    }
    attributes = []

    # Invalid: branches operation with non-"." root_reference
    expressions = [
      {
        label          = "Test Expression"
        reference      = "test-expr"
        root_reference = "payload.some_field"
        operations = [
          {
            operation_type = "branches"
            branches = {
              returns = {
                type  = "Text"
                array = false
              }
              branches = [
                {
                  condition_groups = [
                    {
                      conditions = [
                        {
                          subject   = "payload.title"
                          operation = "is_set"
                          param_bindings = []
                        }
                      ]
                    }
                  ]
                  result = {
                    value = {
                      literal = "some-value"
                    }
                  }
                }
              ]
            }
          }
        ]
      }
    ]
  }
}
`, testAlertSourceTitle, testAlertSourceDescription)
}

const (
	testAlertSourceTitle       = `{"content":[{"content":[{"attrs":{"label":"Payload → Title","missing":false,"name":"title"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`
	testAlertSourceDescription = `{"content":[{"content":[{"attrs":{"label":"Payload → Description","missing":false,"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`
)

func TestAccAlertSourceResource_DynamicAttributes(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Test dynamic attributes
			{
				Config: testAccAlertSourceResourceConfigDynamicAttributes(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.dynamic_alert_source", "name", "tf-dynamic-alert-source"),
					resource.TestCheckResourceAttr("incident_alert_source.dynamic_alert_source", "source_type", "http"),
					// Verify we have 2 attributes
					resource.TestCheckResourceAttr("incident_alert_source.dynamic_alert_source", "template.attributes.#", "2"),
				),
			},
		},
	})
}

func testAccAlertSourceResourceConfigDynamicAttributes() string {
	return testRunTemplate("incident_alert_source_dynamic_attributes", `
# Create alert attributes directly
resource "incident_alert_attribute" "team" {
  name  = "team-tf-attr"
  type  = "String"
  array = false
}

resource "incident_alert_attribute" "feature" {
  name  = "feature-tf-attr"
  type  = "String"
  array = false
}

locals {
	with_conds = true
}

# Use those attributes in an alert source
resource "incident_alert_source" "dynamic_alert_source" {
  name        = "tf-dynamic-alert-source"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }

    # Use a simple attribute list without dynamic references
    attributes = local.with_conds ? [
      {
        alert_attribute_id = incident_alert_attribute.team.id
        binding = {
          value = {
            literal = "team-value"
          }
        }
      },
      {
        alert_attribute_id = incident_alert_attribute.feature.id
        binding = {
          value = {
            literal = "feature-value"
          }
        }
      }
    ] : []
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

func testAccAlertSourceResourceConfigVisibleToTeamsWithoutPrivate() string {
	return testRunTemplate("incident_alert_source_visible_to_teams_without_private", `
resource "incident_alert_source" "test" {
  name        = "test-source-invalid"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
    visible_to_teams = {
      array_value = [{ literal = "some-team-id" }]
    }
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

func testAccAlertSourceResourceConfigPrivateWithoutTeams() string {
	return testRunTemplate("incident_alert_source_private_without_teams", `
resource "incident_alert_source" "test" {
  name        = "test-source-invalid"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
    is_private = true
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

// TestAccAlertSourceResource_Private checks that privacy settings work correctly.
func TestAccAlertSourceResource_Private(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a private alert source
			{
				Config: testAccAlertSourceResourceConfigPrivate(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "private-alert-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "http"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "template.is_private", "true"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "template.visible_to_teams.array_value.0.literal"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccAlertSourceResourceConfigPrivate() string {
	return testRunTemplate("incident_alert_source_private", `
# Look up the Team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# Create a team catalog entry for this test
resource "incident_catalog_entry" "test_team" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-privacy-test"
  name            = "Terraform Alert Source Privacy Test Team"
  attribute_values = []
}

resource "incident_alert_source" "test" {
  name        = "private-alert-source"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
    is_private = true
    visible_to_teams = {
      array_value = [{ literal = incident_catalog_entry.test_team.id }]
    }
  }
}
`, struct {
		Title, Description, TeamTypeName string
	}{
		Title:        testAlertSourceTitle,
		Description:  testAlertSourceDescription,
		TeamTypeName: teamTypeName(),
	})
}

// TestAccAlertSourceResource_Issue342_SpuriousMergeStrategy is a regression
// test for https://github.com/incident-io/terraform-provider-incident/issues/342.
//
// Original bug: when adding a new element to template.attributes (a Set), the
// framework couldn't reliably match an existing literal-only binding (e.g. the
// Priority attribute) between prior state and new plan, because state had
// merge_strategy=null and the plan resolved merge_strategy to Unknown. The
// resulting plan deleted-and-recreated the priority element with a spurious
// merge_strategy injected, which the API rejected with HTTP 422 (priority
// bindings rejected merge_strategy entirely).
//
// Fix landed server-side: priority bindings now always carry
// merge_strategy="last_wins" in the API response and accept either nil or
// "last_wins" on write (matching the actual evaluation semantics, which
// always overwrite the alert's PriorityID). With a concrete (non-null) value
// in state, the standard UseStateForUnknown plan modifier preserves it, set
// element identity stays stable, and the spurious diff disappears.
//
// The plan check asserts: no literal-only binding has merge_strategy marked
// Unknown in the planned state (the original bug signature), and any
// merge_strategy value present in `before` is preserved in `after`.
func TestAccAlertSourceResource_Issue342_SpuriousMergeStrategy(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: alert source with priority (literal-only binding) plus
			// three reference bindings that explicitly set merge_strategy.
			{
				Config: testAccAlertSourceResourceConfigIssue342(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "template.attributes.#", "4"),
				),
			},
			// Step 2: add a fifth attribute. The plan should add only that
			// new element and leave existing elements (notably the priority
			// binding) untouched.
			{
				Config: testAccAlertSourceResourceConfigIssue342(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						noSpuriousMergeStrategyPlanCheck("incident_alert_source.test"),
					},
				},
			},
		},
	})
}

// noSpuriousMergeStrategyPlanCheck asserts that adding a new element to
// template.attributes does NOT cause the framework to lose track of an
// existing literal-only binding's merge_strategy value. Specifically:
//
//   - No literal-only binding in the planned state has merge_strategy marked
//     Unknown (that was the original bug signature — state had null,
//     plan went Unknown, set diff treated the element as deleted-and-recreated).
//   - Every literal-only binding present in the prior state survives into
//     the planned state with the same merge_strategy value (matched by
//     alert_attribute_id).
func noSpuriousMergeStrategyPlanCheck(addr string) plancheck.PlanCheck {
	return spuriousMergeStrategyCheck{addr: addr}
}

type spuriousMergeStrategyCheck struct {
	addr string
}

func (c spuriousMergeStrategyCheck) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, rc := range req.Plan.ResourceChanges {
		if rc.Address != c.addr || rc.Change == nil {
			continue
		}

		beforeAttrs := extractAttributes(rc.Change.Before)
		afterAttrs := extractAttributes(rc.Change.After)
		unknownAttrs := extractAttributes(rc.Change.AfterUnknown)

		// Original bug signature: literal-only binding has merge_strategy
		// marked Unknown in the planned state.
		for i, planned := range afterAttrs {
			if !isLiteralOnlyBinding(planned) {
				continue
			}
			if i >= len(unknownAttrs) {
				continue
			}
			ub, _ := unknownAttrs[i]["binding"].(map[string]any)
			if ub == nil {
				continue
			}
			if ums, ok := ub["merge_strategy"]; ok && ums == true {
				resp.Error = fmt.Errorf(
					"plan marks merge_strategy as Unknown on literal-only binding at template.attributes[%d] (alert_attribute_id=%v); this is the issue #342 bug signature",
					i, planned["alert_attribute_id"],
				)
				return
			}
		}

		// Stronger check: every literal-only binding in prior state must
		// survive into the planned state with the same merge_strategy.
		afterByID := make(map[string]map[string]any, len(afterAttrs))
		for _, a := range afterAttrs {
			id, _ := a["alert_attribute_id"].(string)
			if id != "" {
				afterByID[id] = a
			}
		}
		for _, prior := range beforeAttrs {
			if !isLiteralOnlyBinding(prior) {
				continue
			}
			id, _ := prior["alert_attribute_id"].(string)
			planned, ok := afterByID[id]
			if !ok {
				resp.Error = fmt.Errorf(
					"prior state's literal-only binding (alert_attribute_id=%s) is not present in the planned state; the plan would delete and recreate it",
					id,
				)
				return
			}
			pBinding, _ := prior["binding"].(map[string]any)
			aBinding, _ := planned["binding"].(map[string]any)
			if !mergeStrategiesEqual(pBinding, aBinding) {
				resp.Error = fmt.Errorf(
					"merge_strategy on literal-only binding (alert_attribute_id=%s) changed between state (%v) and plan (%v) without any config change",
					id, pBinding["merge_strategy"], aBinding["merge_strategy"],
				)
				return
			}
		}
	}
}

func isLiteralOnlyBinding(attr map[string]any) bool {
	binding, _ := attr["binding"].(map[string]any)
	if binding == nil {
		return false
	}
	value, _ := binding["value"].(map[string]any)
	if value == nil {
		return false
	}
	literal, _ := value["literal"].(string)
	reference, _ := value["reference"].(string)
	arrayValue := binding["array_value"]
	return literal != "" && reference == "" && arrayValue == nil
}

func mergeStrategiesEqual(a, b map[string]any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a["merge_strategy"] == b["merge_strategy"]
}

func extractAttributes(after any) []map[string]any {
	root, _ := after.(map[string]any)
	if root == nil {
		return nil
	}
	template, _ := root["template"].(map[string]any)
	if template == nil {
		return nil
	}
	rawAttrs, _ := template["attributes"].([]any)
	out := make([]map[string]any, 0, len(rawAttrs))
	for _, a := range rawAttrs {
		if m, ok := a.(map[string]any); ok {
			out = append(out, m)
		} else {
			out = append(out, nil)
		}
	}
	return out
}

func testAccAlertSourceResourceConfigIssue342(withThirdAttribute bool) string {
	return testRunTemplate("incident_alert_source_issue_342", `
data "incident_alert_attribute" "priority" {
  name = "Priority"
}

data "incident_catalog_type" "alert_priority" {
  type_name = "AlertPriority"
}

data "incident_catalog_entries" "priorities" {
  catalog_type_id = data.incident_catalog_type.alert_priority.id
}

# A handful of regular attributes mirroring the user's setup (url, country,
# environment, service). They all use reference bindings with
# merge_strategy = "first_wins", which sits alongside the literal-only
# priority binding.
resource "incident_alert_attribute" "url" {
  name  = "issue-342-url"
  type  = "String"
  array = false
}

resource "incident_alert_attribute" "country" {
  name  = "issue-342-country"
  type  = "String"
  array = false
}

resource "incident_alert_attribute" "environment" {
  name  = "issue-342-environment"
  type  = "String"
  array = false
}

resource "incident_alert_attribute" "service" {
  name  = "issue-342-service"
  type  = "String"
  array = false
}

resource "incident_alert_source" "test" {
  name        = "issue-342-test-source"
  source_type = "datadog"

  template = {
    expressions = [
      {
        label          = "url"
        reference      = "url"
        root_reference = "payload"
        operations = [{
          operation_type = "parse"
          parse = {
            source = "$.url"
            returns = {
              type  = "String"
              array = false
            }
          }
        }]
      },
      {
        label          = "country"
        reference      = "country"
        root_reference = "payload"
        operations = [{
          operation_type = "parse"
          parse = {
            source = "$.country"
            returns = {
              type  = "String"
              array = false
            }
          }
        }]
      },
      {
        label          = "environment"
        reference      = "environment"
        root_reference = "payload"
        operations = [{
          operation_type = "parse"
          parse = {
            source = "$.environment"
            returns = {
              type  = "String"
              array = false
            }
          }
        }]
      },
      {
        label          = "service"
        reference      = "service"
        root_reference = "payload"
        operations = [{
          operation_type = "parse"
          parse = {
            source = "$.service"
            returns = {
              type  = "String"
              array = false
            }
          }
        }]
      },
    ]
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }

    attributes = concat(
      [
        # Priority binding: literal-only, no merge_strategy. The API stores no
        # merge_strategy server-side for this binding.
        {
          alert_attribute_id = data.incident_alert_attribute.priority.id
          binding = {
            value = {
              literal = data.incident_catalog_entries.priorities.catalog_entries[0].id
            }
          }
        },
        # Reference bindings with merge_strategy. Several of these alongside the
        # literal-only priority binding is what triggers the Set-diff bug when
        # we add another element in step 2.
        {
          alert_attribute_id = incident_alert_attribute.url.id
          binding = {
            value          = { reference = "expressions[\"url\"]" }
            merge_strategy = "first_wins"
          }
        },
        {
          alert_attribute_id = incident_alert_attribute.country.id
          binding = {
            value          = { reference = "expressions[\"country\"]" }
            merge_strategy = "first_wins"
          }
        },
        {
          alert_attribute_id = incident_alert_attribute.environment.id
          binding = {
            value          = { reference = "expressions[\"environment\"]" }
            merge_strategy = "first_wins"
          }
        },
      ],
      {{ if .WithThird }}[
        # The new element added in step 2. Adding this should NOT cause any
        # change to the priority binding above, but the bug causes the
        # framework to plan merge_strategy = "first_wins" on it anyway.
        {
          alert_attribute_id = incident_alert_attribute.service.id
          binding = {
            value          = { reference = "expressions[\"service\"]" }
            merge_strategy = "first_wins"
          }
        },
      ]{{ else }}[]{{ end }},
    )
  }
}
`, struct {
		Title, Description string
		WithThird          bool
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
		WithThird:   withThirdAttribute,
	})
}

// TestAccAlertSourceResource_OwningTeamIDs checks that owning_team_ids work correctly.
func TestAccAlertSourceResourceOwningTeamIDs(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create without owning_team_ids
			{
				Config: testAccAlertSourceResourceConfig("test-source-no-teams", "datadog"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source-no-teams"),
					resource.TestCheckNoResourceAttr("incident_alert_source.test", "owning_team_ids"),
				),
			},
			// Update to add owning_team_ids
			{
				Config: testAccAlertSourceResourceConfigWithOwningTeamIDs("test-source-with-teams"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source-with-teams"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "owning_team_ids.#", "1"),
					resource.TestCheckResourceAttrPair("incident_alert_source.test", "owning_team_ids.0", "incident_catalog_entry.owner_team", "id"),
				),
			},
			// Update to change the team
			{
				Config: testAccAlertSourceResourceConfigWithDifferentOwningTeamIDs("test-source-updated-teams"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source-updated-teams"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "owning_team_ids.#", "2"),
				),
			},
		},
	})
}

func testAccAlertSourceResourceConfigWithOwningTeamIDs(name string) string {
	return testRunTemplate("incident_alert_source_with_owning_teams", `
# Look up the Team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# Create a team catalog entry for this test
resource "incident_catalog_entry" "owner_team" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-owning-team-test"
  name            = "Terraform Alert Source Owning Team Test"
  attribute_values = []
}

resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "datadog"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
  }

  owning_team_ids = [incident_catalog_entry.owner_team.id]
}
`, struct {
		Name, Title, Description, TeamTypeName string
	}{
		Name:         name,
		Title:        testAlertSourceTitle,
		Description:  testAlertSourceDescription,
		TeamTypeName: teamTypeName(),
	})
}

func testAccAlertSourceResourceConfigWithDifferentOwningTeamIDs(name string) string {
	return testRunTemplate("incident_alert_source_with_different_owning_teams", `
# Look up the Team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# Create team catalog entries for this test
resource "incident_catalog_entry" "owner_team" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-owning-team-test"
  name            = "Terraform Alert Source Owning Team Test"
  attribute_values = []
}

resource "incident_catalog_entry" "owner_team_2" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-owning-team-test-2"
  name            = "Terraform Alert Source Owning Team Test 2"
  attribute_values = []
}

resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "datadog"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
  }

  owning_team_ids = [
    incident_catalog_entry.owner_team.id,
    incident_catalog_entry.owner_team_2.id
  ]
}
`, struct {
		Name, Title, Description, TeamTypeName string
	}{
		Name:         name,
		Title:        testAlertSourceTitle,
		Description:  testAlertSourceDescription,
		TeamTypeName: teamTypeName(),
	})
}
