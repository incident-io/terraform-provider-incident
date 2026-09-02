package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccAlertSourceBeta covers the basic lifecycle: create, read back unchanged,
// import, and update.
func TestAccAlertSourceBeta(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceBetaConfig("test-source", "a title", "a description"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "name", StableSuffix("test-source")),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "source_type", "http"),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "title.literal", "a title"),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "description.literal", "a description"),
					resource.TestCheckResourceAttrSet("incident_alert_source_beta.test", "id"),
					resource.TestCheckResourceAttrSet("incident_alert_source_beta.test", "secret_token"),
				),
			},
			{
				ResourceName:      "incident_alert_source_beta.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccAlertSourceBetaConfig("test-source-renamed", "a title", "a description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "name", StableSuffix("test-source-renamed")),
				),
			},
		},
	})
}

func testAccAlertSourceBetaConfig(name, title, description string) string {
	return testRunTemplate("incident_alert_source_beta", `
resource "incident_alert_source_beta" "test" {
  name        = {{ quote .Name }}
  source_type = "http"

  title       = { literal = {{ quote .Title }} }
  description = { literal = {{ quote .Description }} }
}
`, struct {
		Name        string
		Title       string
		Description string
	}{
		Name:        StableSuffix(name),
		Title:       title,
		Description: description,
	})
}

// TestAccAlertSourceBetaPriority covers priority = { expression_ref = ... }, which is the
// beta resource's whole reason for existing on this field: incident_alert_source has to
// smuggle priority through a binding on the built-in Priority alert attribute, and here it
// is a field.
//
// Two expressions, because the payload is opaque JSON: an expression reaches into it with
// parse, and a condition can only ask whether payload as a whole is set, so the branches
// condition tests the first expression's result rather than payload.severity.
func TestAccAlertSourceBetaPriority(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceBetaPriorityConfig(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.priority", "priority.expression_ref", "priority_lookup"),
					resource.TestCheckResourceAttr("incident_alert_source_beta.priority", "named_expression.#", "2"),
					resource.TestCheckResourceAttrSet("incident_alert_source_beta.priority", "id"),
				),
			},
			{
				ResourceName:      "incident_alert_source_beta.priority",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccAlertSourceBetaPriorityConfig() string {
	return testRunTemplate("incident_alert_source_beta_priority", `
data "incident_catalog_type" "alert_priority" {
  type_name = "AlertPriority"
}

# Indexed rather than looked up by name, because which priorities an org has is its own
# business and this test only cares that the binding round-trips.
data "incident_catalog_entries" "priorities" {
  catalog_type_id = data.incident_catalog_type.alert_priority.id
}

resource "incident_alert_source_beta" "priority" {
  name        = {{ stableSuffix "beta-priority" | quote }}
  source_type = "http"

  title       = { literal = "a title" }
  description = { literal = "a description" }

  named_expression {
    name       = "severity"
    start_from = "payload"

    operation {
      parse {
        function = "$.severity"
        as       = "String"
      }
    }
  }

  named_expression {
    name       = "priority_lookup"
    start_from = "."

    operation {
      branches {
        as = "CatalogEntry[\"AlertPriority\"]"

        if {
          conditions = [{
            subject   = "expressions[\"severity\"]"
            operation = "one_of"
            params    = [{ values = ["CRITICAL"] }]
          }]
          result = { value_literal = data.incident_catalog_entries.priorities.catalog_entries[0].id }
        }
      }
    }

    fallback {
      result = { value_literal = data.incident_catalog_entries.priorities.catalog_entries[0].id }
    }
  }

  priority = {
    expression_ref = "priority_lookup"
  }
}
`, nil)
}
