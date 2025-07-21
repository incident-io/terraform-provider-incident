package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/samber/lo"
)

func TestAccIncidentAlertRouteResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccIncidentAlertRouteResourceConfig("test-route"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "is_private", "false"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},

				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "is_private", "false"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_route.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing
			{
				Config: testAccIncidentAlertRouteResourceConfig("test-route-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route-updated"),
				),
			},
		},
	})
}

func TestAccIncidentAlertRouteResourceHTMLCharacters(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing with HTML characters in incident template
			{
				Config: testAccIncidentAlertRouteResourceConfigWithHTMLChars("html-chars-route"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.html_test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.html_test", "name", "html-chars-route"),
					resource.TestCheckResourceAttr("incident_alert_route.html_test", "enabled", "true"),
					// Verify the incident template name contains the HTML characters
					resource.TestCheckResourceAttrSet("incident_alert_route.html_test", "incident_template.name.value.literal"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccIncidentAlertRouteResourceComprehensive(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing with comprehensive configuration
			{
				Config: testAccIncidentAlertRouteResourceConfigComprehensive("comprehensive-test-route"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.comprehensive", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "name", "comprehensive-test-route"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "is_private", "false"),

					// Check alert sources
					resource.TestCheckResourceAttrSet("incident_alert_route.comprehensive", "alert_sources.0.alert_source_id"),

					// Check condition groups
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "condition_groups.0.conditions.0.subject", "alert.title"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "condition_groups.0.conditions.0.operation", "is_set"),

					// Check incident config
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.auto_decline_enabled", "false"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.defer_time_seconds", "300"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_config.grouping_window_seconds", "1800"),

					// Check incident template
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_template.name.autogenerated", "true"),
					resource.TestCheckResourceAttrSet("incident_alert_route.comprehensive", "incident_template.name.value.literal"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_template.summary.autogenerated", "true"),
					resource.TestCheckResourceAttrSet("incident_alert_route.comprehensive", "incident_template.summary.value.literal"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_template.start_in_triage.value.literal", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_template.severity.merge_strategy", "first-wins"),

					// Verify custom field was created and has the correct merge strategy
					resource.TestCheckResourceAttrSet("incident_custom_field.type_field", "id"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_template.custom_fields.0.merge_strategy", "first-wins"),
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "incident_template.custom_fields.0.binding.value.literal", "Test incident"),
				),
			},
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
				ResourceName:      "incident_alert_route.comprehensive",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing
			{
				Config: testAccIncidentAlertRouteResourceConfigComprehensive("comprehensive-test-route-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.comprehensive", "name", "comprehensive-test-route-updated"),
				),
			},
		},
	})
}

func TestAccIncidentAlertRouteResourceAutoGenerated(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing with autogenerated incident template
			{
				Config: testAccIncidentAlertRouteResourceConfigAutoGenerated("autogenerated-test-route", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.auto_gen_test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.auto_gen_test", "name", "autogenerated-test-route"),
					resource.TestCheckResourceAttr("incident_alert_route.auto_gen_test", "incident_template.name.autogenerated", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.auto_gen_test", "incident_template.summary.autogenerated", "true"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Update to disabled autogeneration
			{
				Config: testAccIncidentAlertRouteResourceConfigAutoGenerated("autogenerated-test-route", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.auto_gen_test", "incident_template.name.autogenerated", "false"),
					resource.TestCheckResourceAttr("incident_alert_route.auto_gen_test", "incident_template.summary.autogenerated", "false"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccIncidentAlertRouteResourceChannelConfig(t *testing.T) {
	skipUnlessTypedSlackChannelID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing with channel configuration
			{
				Config: testAccIncidentAlertRouteResourceConfigWithChannelConfig("channel-test-route"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.channel_test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.channel_test", "name", "channel-test-route"),

					// Check channel config
					resource.TestCheckResourceAttr("incident_alert_route.channel_test", "channel_config.0.slack_targets.channel_visibility", "private"),
					resource.TestCheckResourceAttr("incident_alert_route.channel_test", "channel_config.0.slack_targets.binding.value.literal", os.Getenv("INCIDENT_SLACK_CHANNEL_ID")),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Update to use expression reference
			{
				Config: testAccIncidentAlertRouteResourceConfigWithEmptyChannelConfig("channel-test-route"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.channel_test", "channel_config.#", "0"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccIncidentAlertRouteResourceWithVars(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccIncidentAlertRouteResourceConfigWithVars("test-route-with-vars"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route-with-vars"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "is_private", "false"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccIncidentAlertRouteResourceConditionalConfig(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccIncidentAlertRouteResourceConfigWithVars("test-route"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_route.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccIncidentCustomFieldsAlphabeticalOrder(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// First step - create alert route with custom fields in reverse alphabetical order
			{
				Config: testAccIncidentAlertRouteWithAlphabeticalCustomFields("custom-fields-alpha-test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.custom_fields_alpha_test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),

					// Verify both custom fields exist
					resource.TestCheckResourceAttrSet("incident_custom_field.alpha_field1", "id"),
					resource.TestCheckResourceAttrSet("incident_custom_field.alpha_field2", "id"),

					// Check that we have exactly 2 custom fields
					resource.TestCheckResourceAttr("incident_alert_route.custom_fields_alpha_test", "incident_template.custom_fields.#", "2"),
				),
			},
			// Second step - refresh state and verify no diff
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					// Only check custom fields, not other attributes
					resource.TestCheckResourceAttr("incident_alert_route.custom_fields_alpha_test", "incident_template.custom_fields.#", "2"),
				),
			},
		},
	})
}

func skipUnlessTypedSlackChannelID(t *testing.T) {
	if os.Getenv("INCIDENT_SLACK_CHANNEL_ID") == "" {
		t.Skip("INCIDENT_SLACK_CHANNEL_ID must be set for this test")
	}
}

func testAccIncidentAlertRouteResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "incident_alert_route" "test" {
  name = %[1]q
  enabled = true
  is_private = false
  alert_sources = []
  channel_config = []
  condition_groups = []
  expressions = []
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  incident_config = {
    auto_decline_enabled    = true
    condition_groups        = []
    defer_time_seconds      = 0
    grouping_keys          = []
    grouping_window_seconds = 0
    enabled                 = true
  }
  incident_template = {
    name = {
      autogenerated = true
    }
    summary = {
      autogenerated = true
    }
    severity = {
      merge_strategy = "first-wins"
    }
  }
}
`, name)
}

func testAccIncidentAlertRouteResourceConfigAutoGenerated(name string, autogenerated bool) string {
	return fmt.Sprintf(`
resource "incident_alert_source" "auto_gen_test" {
  name        = "Auto Generated Test Source %[1]s"
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
          }
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
              }
            ]
            type = "paragraph"
          }
        ]
        type = "doc"
      })
    }
    attributes = []
    expressions = []
  }
}

resource "incident_alert_route" "auto_gen_test" {
  name = %[1]q
  enabled = true
  is_private = false
  alert_sources = [
    {
      alert_source_id = incident_alert_source.auto_gen_test.id
      condition_groups = []
    }
  ]
  channel_config = []
  condition_groups = []
  expressions = []
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  incident_config = {
    auto_decline_enabled    = false
    enabled                 = true
    condition_groups        = []
    defer_time_seconds      = 0
    grouping_keys          = []
    grouping_window_seconds = 0
  }
  incident_template = {
    name = {
      autogenerated = %[2]t
      value = %[2]t ? null : {
        literal = jsonencode({
          content = [
            {
              content = [
                {
                  text = "Custom incident name"
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
      autogenerated = %[2]t
      value = %[2]t ? null : {
        literal = jsonencode({
          content = [
            {
              content = [
                {
                  text = "Custom incident summary"
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
  }
}
`, name, autogenerated)
}

func testAccIncidentAlertRouteResourceConfigComprehensive(name string) string {
	return fmt.Sprintf(`
resource "incident_custom_field" "type_field" {
  name        = "Test Type Field %[1]s"
  description = "The type of the incident."
  field_type  = "text"
}
resource "incident_alert_source" "http_test" {
  name        = "HTTP Test Alert Source %[1]s"
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
          }
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
              }
            ]
            type = "paragraph"
          }
        ]
        type = "doc"
      })
    }
    attributes = []
    expressions = []
  }
}

resource "incident_alert_route" "comprehensive" {
  name = %[1]q
  enabled = true
  is_private = false
  alert_sources = [
    {
      alert_source_id = incident_alert_source.http_test.id
      condition_groups = []
    }
  ]
  channel_config = []
  condition_groups = [
    {
      conditions = [
        {
          subject   = "alert.title"
          operation = "is_set"
          param_bindings = []
        },
      ]
    }
  ]
  expressions = []
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  incident_config = {
    auto_decline_enabled    = false
    enabled                 = true
    condition_groups        = []
    defer_time_seconds      = 300
    grouping_keys          = []
    grouping_window_seconds = 1800
  }
  incident_template = {
    name = {
      autogenerated = true
    }
    summary = {
      autogenerated = true
    }
    start_in_triage = {
      value = {
        literal = "true"
      }
    }
    custom_fields = [
      {
        custom_field_id = incident_custom_field.type_field.id
        binding = {
          value = {
            literal = "Test incident"
          }
        }
        merge_strategy = "first-wins"
      }
    ]
    severity = {
      merge_strategy = "first-wins"
    }
  }
}
`, name)
}

func testAccIncidentAlertRouteResourceConfigWithChannelConfig(name string) string {
	return fmt.Sprintf(`
resource "incident_alert_source" "channel_test" {
  name        = "Channel Test Alert Source %[1]s"
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
          }
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
              }
            ]
            type = "paragraph"
          }
        ]
        type = "doc"
      })
    }
    attributes = []
    expressions = []
  }
}

resource "incident_alert_route" "channel_test" {
  name = %[1]q
  enabled = true
  is_private = false
  alert_sources = [
    {
      alert_source_id = incident_alert_source.channel_test.id
      condition_groups = []
    }
  ]
  channel_config = [
    {
      condition_groups = []
      slack_targets = {
        channel_visibility = "private"
        binding = {
          value = {
            literal = %[2]q
          }
        }
      }
    }
  ]
  condition_groups = []
  expressions = []
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  incident_config = {
    auto_decline_enabled    = false
    enabled                 = true
    condition_groups        = []
    defer_time_seconds      = 0
    grouping_keys          = []
    grouping_window_seconds = 0
  }
  incident_template = {
    name = {
      autogenerated = true
    }
    summary = {
      autogenerated = true
    }
    severity = {
      merge_strategy = "first-wins"
    }
  }
}
`, name, lo.Must(os.LookupEnv("INCIDENT_SLACK_CHANNEL_ID")))
}

func testAccIncidentAlertRouteResourceConfigWithEmptyChannelConfig(name string) string {
	return fmt.Sprintf(`
resource "incident_alert_source" "channel_test" {
  name        = "Channel Test Alert Source %[1]s"
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
          }
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
              }
            ]
            type = "paragraph"
          }
        ]
        type = "doc"
      })
    }
    attributes = []
    expressions = []
  }
}

resource "incident_alert_route" "channel_test" {
  name = %[1]q
  enabled = true
  is_private = false
  alert_sources = [
    {
      alert_source_id = incident_alert_source.channel_test.id
      condition_groups = []
    }
  ]
  channel_config = []
  condition_groups = []
  expressions = []
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  incident_config = {
    auto_decline_enabled    = false
    enabled                 = true
    condition_groups        = []
    defer_time_seconds      = 0
    grouping_keys          = []
    grouping_window_seconds = 0
  }
  incident_template = {
    name = {
      autogenerated = true
    }
    summary = {
      autogenerated = true
    }
    severity = {
      merge_strategy = "first-wins"
    }
  }
}
`, name)
}

func testAccIncidentAlertRouteResourceConfigWithVars(name string) string {
	return fmt.Sprintf(`
locals {
  with_conds = false
}
resource "incident_alert_route" "test" {
  name = %[1]q
  enabled = true
  is_private = false
  alert_sources = []
  channel_config = []
  condition_groups = local.with_conds ? [
    {
      conditions = [
        {
          subject   = "alert.title"
          operation = "is_set"
          param_bindings = []
        },
      ]
    }
  ] : []
  expressions = []
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  incident_config = {
    auto_decline_enabled    = true
    condition_groups        = []
    defer_time_seconds      = 0
    grouping_keys          = []
    grouping_window_seconds = 0
    enabled                 = true
  }
  incident_template = {
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

	custom_fields = []

	severity = {
      merge_strategy = "first-wins"
	}
  }
}
`, name)
}

func testAccIncidentAlertRouteResourceConfigWithHTMLChars(name string) string {
	return fmt.Sprintf(`
resource "incident_alert_source" "html_test" {
  name        = "HTML Test Alert Source %[1]s"
  source_type = "http"
  template = {
    title = {
      literal = jsonencode({
        content = [
          {
            content = [
              {
                attrs = {
                  label   = "Alert -> Title"
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
    description = {
      literal = jsonencode({
        content = [
          {
            content = [
              {
                attrs = {
                  label   = "Alert -> Description"
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
    attributes = []
    expressions = []
  }
}

resource "incident_alert_route" "html_test" {
  name = %[1]q
  enabled = true
  is_private = false
  alert_sources = [
    {
      alert_source_id = incident_alert_source.html_test.id
      condition_groups = []
    }
  ]
  channel_config = []
  condition_groups = []
  expressions = []
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  incident_config = {
    auto_decline_enabled    = false
    enabled                 = true
    condition_groups        = []
    defer_time_seconds      = 0
    grouping_keys          = []
    grouping_window_seconds = 0
  }
  incident_template = {
    name = {
      autogenerated = false
      value = {
        literal = jsonencode({
          content = [
            {
              content = [
                {
                  attrs = {
                    label   = "Alert -> Title"
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
                    label   = "Test <summary> with & special > characters"
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
  }
}
`, name)
}
func testAccIncidentAlertRouteWithAlphabeticalCustomFields(name string) string {
	return `
  resource "incident_custom_field" "alpha_field1" {
    name        = "Alpha Test Field 1"
    description = "First alphabetical custom field"
    field_type  = "text"
  }

  resource "incident_custom_field" "alpha_field2" {
    name        = "Alpha Test Field 2"
    description = "Second alphabetical custom field"
    field_type  = "text"
    depends_on = [incident_custom_field.alpha_field1]

  }

  resource "incident_alert_source" "alpha_test" {
    name        = "Alpha Test Alert Source"
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

  resource "incident_alert_route" "custom_fields_alpha_test" {
    name       = "` + name + `"
    enabled    = true
    is_private = false

    alert_sources = [
      {
        alert_source_id = incident_alert_source.alpha_test.id
        condition_groups = []
      }
    ]

    # Minimal configuration, focusing only on custom fields
    condition_groups = []
    expressions = []
    channel_config = []

    escalation_config = {
      auto_cancel_escalations = true
      escalation_targets = []
    }

    incident_config = {
      auto_decline_enabled    = false
      enabled                 = true
      condition_groups        = []
      defer_time_seconds      = 300
      grouping_keys           = []
      grouping_window_seconds = 1800
    }

    incident_template = {
      # Focus on custom fields in reverse lexicographical order
      custom_fields = [
       {
          custom_field_id = incident_custom_field.alpha_field2.id
          merge_strategy = "first-wins"
          binding = {
            value = {
              literal = "First alphabetical custom field value"
            }
          }
        },
        {
          custom_field_id = incident_custom_field.alpha_field1.id
          merge_strategy = "first-wins"
          binding = {
            value = {
              literal = "Second alphabetical custom field value"
            }
          }
        }
      ]

      # Minimal required configuration for name and summary
      name = {
        autogenerated = false
        value = {
          literal = jsonencode({
            content = [
              {
                content = [
                  {
                    text = "Test Incident"
                    type = "text"
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
                    text = "Test Incident Summary"
                    type = "text"
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
    }
  }
  `
}

func channelID(withTeam bool) string {
	if os.Getenv("CI") == "true" {
		// This is a channel that exists in our integration test workspace
		return lo.If(withTeam, "T04TJRY5UMB/C04U0DJSG0Z").Else("C04U0DJSG0Z")
	}
	// If you're running against a different Slack workspace, override the channel ID with this env var.
	if envChannelID := os.Getenv("TF_ACC_CHANNEL_ID"); envChannelID != "" {
		return envChannelID
	}

	// This channel exists in the default workspace used by incident.io engineers for testing, so is a helpful default.
	return lo.If(withTeam, "T02A1FSLE8J/C0392FG9C20").Else("C0392FG9C20")
}
