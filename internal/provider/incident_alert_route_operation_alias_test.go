package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccIncidentAlertRouteOperationAlias reproduces ONC-12602 against the real
// API: `one_of` on a String alert attribute is an alias the API returns as
// `contains_one_of`. Before the fix the create step failed with "Provider
// produced inconsistent result after apply". The second step re-applies the same
// config to prove the refresh path holds the alias too, rather than showing a
// permanent diff.
func TestAccIncidentAlertRouteOperationAlias(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAlertRouteOperationAliasConfig("one_of"),
				Check: resource.TestCheckResourceAttr(
					"incident_alert_route.alias", "condition_groups.0.conditions.0.operation", "one_of"),
			},
			{
				Config: testAccIncidentAlertRouteOperationAliasConfig("one_of"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			// The canonical name must still round-trip: we only ever swap back to
			// what the config asked for.
			{
				Config: testAccIncidentAlertRouteOperationAliasConfig("contains_one_of"),
				Check: resource.TestCheckResourceAttr(
					"incident_alert_route.alias", "condition_groups.0.conditions.0.operation", "contains_one_of"),
			},
		},
	})
}

func testAccIncidentAlertRouteOperationAliasConfig(operation string) string {
	return fmt.Sprintf(`
resource "incident_alert_attribute" "alias" {
  name  = %[1]q
  type  = "String"
  array = false
}

resource "incident_alert_route" "alias" {
  name       = %[2]q
  enabled    = true
  is_private = false

  alert_sources = []
  expressions   = []

  condition_groups = [{
    conditions = [{
      subject        = "alert.attributes.${incident_alert_attribute.alias.id}"
      operation      = %[3]q
      param_bindings = [{ array_value = [{ literal = "org/repo" }] }]
    }]
  }]

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
  }
}
`, StableSuffix("alias-attr"), StableSuffix("alias-route"), operation)
}
