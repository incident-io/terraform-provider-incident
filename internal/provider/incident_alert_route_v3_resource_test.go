package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccIncidentAlertRouteV2ToV3Migration verifies the headline feature of the
// merged resource: an existing route configured with the v2 schema can be moved
// to the v3 schema in place. The second step switches the config from the v2
// layout (channel_config / incident_template / grouping fields under
// incident_config) to the v3 layout (grouping_config / message_config /
// incident_config.template) and asserts the plan is an Update, not a
// replacement — i.e. the same underlying alert route is reused, even though the
// provider switches from the v2 to the v3 API to manage it.
func TestAccIncidentAlertRouteV2ToV3Migration(t *testing.T) {
	name := StableSuffix("migration-route")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Start on the v2 schema.
				Config: testAccIncidentAlertRouteResourceConfig(name),
				Check:  resource.TestCheckResourceAttrSet("incident_alert_route.test", "id"),
			},
			{
				// Switch to the v3 schema in place: expect an update, not a replace.
				Config: testAccIncidentAlertRouteV3ResourceConfig(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_alert_route.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.test", "grouping_config.default.enabled", "false"),
					// The deprecated v2-only block is cleared once migrated.
					resource.TestCheckNoResourceAttr("incident_alert_route.test", "incident_template"),
				),
			},
		},
	})
}

func TestAccIncidentAlertRouteV3Resource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccIncidentAlertRouteV3ResourceConfig("test-route-v3"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route-v3"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "is_private", "false"),
					// Grouping config lives at the top level in v3.
					resource.TestCheckResourceAttr("incident_alert_route.test", "grouping_config.default.enabled", "false"),
					// When grouping is disabled the optional window fields are unset.
					resource.TestCheckNoResourceAttr("incident_alert_route.test", "grouping_config.default.window_type"),
					resource.TestCheckNoResourceAttr("incident_alert_route.test", "grouping_config.default.window_seconds"),
				),
			},
			// Refresh and ensure there's no perpetual diff.
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_route.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing
			{
				Config: testAccIncidentAlertRouteV3ResourceConfig("test-route-v3-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route-v3-updated"),
				),
			},
		},
	})
}

func TestAccIncidentAlertRouteV3ResourceComprehensive(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAlertRouteV3ResourceConfigComprehensive("comprehensive-test-route-v3"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.comprehensive", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "name", "comprehensive-test-route-v3"),

					// Alert sources.
					resource.TestCheckResourceAttrSet("incident_alert_route.comprehensive", "alert_sources.0.alert_source_id"),

					// Grouping config.
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "grouping_config.default.enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "grouping_config.default.window_seconds", "1800"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "grouping_config.default.window_type", "fixed"),

					// Message config (destinations).
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "message_config.destinations.#", "1"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "message_config.destinations.0.slack_targets.channel_visibility", "public"),

					// Escalation config with when_alert_joins_group.
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "escalation_config.when_alert_joins_group.mode", "on_each_new_alert"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "escalation_config.when_alert_joins_group.grace_period_seconds", "60"),

					// Incident config and nested template.
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.auto_decline_enabled", "false"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.template.name.autogenerated", "true"),
					resource.TestCheckResourceAttrSet("incident_alert_route.comprehensive", "incident_config.template.name.value.literal"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.template.severity.merge_strategy", "first-wins"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.template.custom_fields.0.merge_strategy", "first-wins"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.template.custom_fields.0.binding.value.literal", "Test incident"),
				),
			},
			// Refresh and ensure there's no perpetual diff on the full config.
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// ImportState testing.
			{
				ResourceName:      "incident_alert_route.comprehensive",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing.
			{
				Config: testAccIncidentAlertRouteV3ResourceConfigComprehensive("comprehensive-test-route-v3-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "name", "comprehensive-test-route-v3-updated"),
				),
			},
		},
	})
}

// TestAccIncidentAlertRouteV3ResourceBranchesValidation tests that expressions
// using branches operations with a non-"." root_reference are rejected at plan
// time, mirroring the v2 behaviour.
func TestAccIncidentAlertRouteV3ResourceBranchesValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccIncidentAlertRouteV3ResourceConfigInvalidBranches(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid root_reference for branches operation`),
			},
		},
	})
}

// TestAccIncidentAlertRouteV3ResourceGroupingValidation asserts that setting a
// grouping detail field while grouping is disabled is rejected at plan time,
// rather than being silently dropped on apply (which would leave state out of
// sync with the remote configuration).
func TestAccIncidentAlertRouteV3ResourceGroupingValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccIncidentAlertRouteV3ResourceConfigGroupingDisabledWithWindow(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must not be set when`),
			},
		},
	})
}

// TestAccIncidentAlertRouteV3ResourceIncidentTemplateValidation asserts that
// setting an incident template while incident creation is disabled is rejected
// at plan time (the API drops the template otherwise).
func TestAccIncidentAlertRouteV3ResourceIncidentTemplateValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccIncidentAlertRouteV3ResourceConfigTemplateWhileIncidentDisabled(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("`incident_config.template` must not be set when"),
			},
		},
	})
}

// alertRouteV3IncidentTemplateBlock is a minimal, valid v3 incident template
// using non-autogenerated name and summary literals.
const alertRouteV3IncidentTemplateBlock = `
    template = {
      custom_fields = []
      name = {
        autogenerated = false
        value = {
          literal = jsonencode({
            content = [
              {
                content = [
                  {
                    attrs = {
                      label   = "Alert → Title"
                      missing = false
                      name    = "alert.title"
                    }
                    type = "varSpec"
                  }
                ]
                type = "paragraph"
              }
            ]
            type = "doc"
          })
        }
      }
      summary = {
        autogenerated = false
        value = {
          literal = jsonencode({
            content = [
              {
                content = [
                  {
                    attrs = {
                      label   = "Alert → Description"
                      missing = false
                      name    = "alert.description"
                    }
                    type = "varSpec"
                  }
                ]
                type = "paragraph"
              }
            ]
            type = "doc"
          })
        }
      }
      severity = {
        merge_strategy = "first-wins"
      }
    }`

func testAccIncidentAlertRouteV3ResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "incident_alert_route" "test" {
  name       = %[1]q
  enabled    = true
  is_private = false

  alert_sources    = []
  condition_groups = []
  expressions      = []

  grouping_config = {
    default = {
      # Grouping disabled: the window fields are optional and must be omitted
      # when disabled (they're only valid, and required, when enabled).
      enabled = false
    }
  }

  message_config = {
    destinations = []
  }

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }

  incident_config = {
    auto_decline_enabled = true
    enabled              = true
    condition_groups     = []
%[2]s
  }
}
`, name, alertRouteV3IncidentTemplateBlock)
}

func testAccIncidentAlertRouteV3ResourceConfigComprehensive(name string) string {
	return fmt.Sprintf(`
resource "incident_custom_field" "type_field" {
  # Keep the name within the API's 50-character limit, even when the route name
  # suffix is the longer "-updated" variant.
  name        = "Type %[1]s"
  description = "The type of the incident."
  field_type  = "text"
}

resource "incident_alert_source" "http_test" {
  name        = "HTTP Test Alert Source V3 %[1]s"
  source_type = "http"
  template = {
    title = {
      literal = jsonencode({
        content = [
          {
            content = [
              {
                attrs = {
                  label   = "Payload → Title"
                  missing = false
                  name    = "title"
                }
                type = "varSpec"
              },
            ]
            type = "paragraph"
          },
        ]
        type = "doc"
      })
    }
    description = {
      literal = jsonencode({
        content = [
          {
            content = [
              {
                attrs = {
                  label   = "Payload → Description"
                  missing = false
                  name    = "description"
                }
                type = "varSpec"
              },
            ]
            type = "paragraph"
          },
        ]
        type = "doc"
      })
    }
    attributes  = []
    expressions = []
  }
}

resource "incident_alert_route" "comprehensive" {
  name       = %[1]q
  enabled    = true
  is_private = false

  alert_sources = [
    {
      alert_source_id  = incident_alert_source.http_test.id
      condition_groups = []
    }
  ]

  condition_groups = [
    {
      conditions = [
        {
          subject        = "alert.title"
          operation      = "is_set"
          param_bindings = []
        }
      ]
    }
  ]

  expressions = []

  grouping_config = {
    default = {
      enabled        = true
      grouping_keys  = []
      window_seconds = 1800
      window_type    = "fixed"
    }
  }

  message_config = {
    destinations = [
      {
        condition_groups = []
        slack_targets = {
          channel_visibility = "public"
          binding = {
            value = {
              literal = %[2]q
            }
          }
        }
      }
    ]
  }

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
    when_alert_joins_group = {
      mode                 = "on_each_new_alert"
      grace_period_seconds = 60
    }
  }

  incident_config = {
    auto_decline_enabled = false
    enabled              = true
    condition_groups     = []
    template = {
      custom_fields = [
        {
          custom_field_id = incident_custom_field.type_field.id
          merge_strategy  = "first-wins"
          binding = {
            value = {
              literal = "Test incident"
            }
          }
        }
      ]
      name = {
        autogenerated = true
        value = {
          literal = jsonencode({
            content = [
              {
                content = [
                  {
                    attrs = {
                      label   = "Alert → Title"
                      missing = false
                      name    = "alert.title"
                    }
                    type = "varSpec"
                  }
                ]
                type = "paragraph"
              }
            ]
            type = "doc"
          })
        }
      }
      summary = {
        autogenerated = true
        value = {
          literal = jsonencode({
            content = [
              {
                content = [
                  {
                    attrs = {
                      label   = "Alert → Description"
                      missing = false
                      name    = "alert.description"
                    }
                    type = "varSpec"
                  }
                ]
                type = "paragraph"
              }
            ]
            type = "doc"
          })
        }
      }
      start_in_triage = {
        value = {
          literal = "true"
        }
      }
      severity = {
        merge_strategy = "first-wins"
      }
    }
  }
}
`, name, channelID(true))
}

func testAccIncidentAlertRouteV3ResourceConfigInvalidBranches() string {
	return fmt.Sprintf(`
resource "incident_alert_route" "invalid_branches" {
  name       = "invalid-branches-test-v3"
  enabled    = true
  is_private = false

  alert_sources    = []
  condition_groups = []

  # Invalid: a branches operation requires root_reference to be ".".
  expressions = [
    {
      label          = "Test Expression"
      reference      = "test-expr"
      root_reference = "alert.attributes.some-attribute-id"
      operations = [
        {
          operation_type = "branches"
          branches = {
            returns = {
              type  = "IncidentSeverity"
              array = false
            }
            branches = [
              {
                condition_groups = [
                  {
                    conditions = [
                      {
                        subject        = "alert.title"
                        operation      = "is_set"
                        param_bindings = []
                      }
                    ]
                  }
                ]
                result = {
                  value = {
                    literal = "01ABC123"
                  }
                }
              }
            ]
          }
        }
      ]
    }
  ]

  grouping_config = {
    default = {
      enabled = false
    }
  }

  message_config = {
    destinations = []
  }

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }

  incident_config = {
    auto_decline_enabled = true
    enabled              = true
    condition_groups     = []
%[1]s
  }
}
`, alertRouteV3IncidentTemplateBlock)
}

// testAccIncidentAlertRouteV3ResourceConfigGroupingDisabledWithWindow is a
// route that disables grouping yet sets window_seconds, which ValidateConfig
// must reject.
func testAccIncidentAlertRouteV3ResourceConfigGroupingDisabledWithWindow() string {
	return fmt.Sprintf(`
resource "incident_alert_route" "test" {
  name       = "grouping-validation"
  enabled    = false
  is_private = false

  alert_sources    = []
  condition_groups = []
  expressions      = []

  grouping_config = {
    default = {
      enabled        = false
      window_seconds = 300
    }
  }

  message_config = {
    destinations = []
  }

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }

  incident_config = {
    auto_decline_enabled = true
    enabled              = true
    condition_groups     = []
%[1]s
  }
}
`, alertRouteV3IncidentTemplateBlock)
}

// testAccIncidentAlertRouteV3ResourceConfigTemplateWhileIncidentDisabled sets an
// incident template while incident creation is disabled, which ValidateConfig
// must reject.
func testAccIncidentAlertRouteV3ResourceConfigTemplateWhileIncidentDisabled() string {
	return fmt.Sprintf(`
resource "incident_alert_route" "test" {
  name       = "template-validation"
  enabled    = false
  is_private = false

  alert_sources    = []
  condition_groups = []
  expressions      = []

  grouping_config = {
    default = {
      enabled = false
    }
  }

  message_config = {
    destinations = []
  }

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }

  incident_config = {
    enabled          = false
    condition_groups = []
%[1]s
  }
}
`, alertRouteV3IncidentTemplateBlock)
}
