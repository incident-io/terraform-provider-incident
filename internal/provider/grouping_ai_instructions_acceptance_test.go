package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// skipUnlessAIGroupingInstructions skips a test that writes ai_instructions. The API
// accepts them only once incident-io/core#84493 is deployed, and only for an organisation
// with the alert-grouping-ai-instructions feature, so set
// INCIDENT_TEST_AI_GROUPING_INSTRUCTIONS=true when the test account has both.
func skipUnlessAIGroupingInstructions(t *testing.T) {
	t.Helper()

	if os.Getenv("INCIDENT_TEST_AI_GROUPING_INSTRUCTIONS") != "true" {
		t.Skip("INCIDENT_TEST_AI_GROUPING_INSTRUCTIONS is not true: skipping the AI grouping instructions test")
	}
}

const (
	testAIInstructions        = "Alerts from the old and new clusters describe the same outage, so group them together."
	testAIInstructionsChanged = "Alerts from the EU and US regions are separate outages, so keep them apart."
)

// aiInstructionsSteps sets, changes and then removes ai_instructions. The last step must
// plan an update, and the refresh after it must find no instructions on the server: if
// the API kept them, the refresh would read them back and plan to remove them again.
func aiInstructionsSteps(resourceName, attribute string, config func(instructions string) string) []resource.TestStep {
	return []resource.TestStep{
		{
			Config: config(testAIInstructions),
			Check:  resource.TestCheckResourceAttr(resourceName, attribute, testAIInstructions),
		},
		{
			Config: config(testAIInstructionsChanged),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
				},
			},
			Check: resource.TestCheckResourceAttr(resourceName, attribute, testAIInstructionsChanged),
		},
		{
			Config: config(""),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
				},
			},
			Check: resource.TestCheckNoResourceAttr(resourceName, attribute),
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
	}
}

// aiInstructionsLine is the ai_instructions line of a config, or nothing when instructions
// is empty, which leaves the attribute out.
func aiInstructionsLine(instructions string) string {
	if instructions == "" {
		return ""
	}

	return fmt.Sprintf("ai_instructions = %q", instructions)
}

func TestAccIncidentAlertRouteV3AIInstructions(t *testing.T) {
	skipUnlessAIGroupingInstructions(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: aiInstructionsSteps(
			"incident_alert_route.test", "grouping_config.default.ai_instructions",
			testAccIncidentAlertRouteV3AIInstructionsConfig,
		),
	})
}

func TestAccIncidentTeamGroupingPreferenceAIInstructions(t *testing.T) {
	skipUnlessAIGroupingInstructions(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: aiInstructionsSteps(
			"incident_team_grouping_preference.test", "default.settings.ai_instructions",
			func(instructions string) string {
				return testAccIncidentTeamGroupingPreferenceConfig(
					tgpSettingsEnabled(1800, "rolling") + "\n      " + aiInstructionsLine(instructions))
			},
		),
	})
}

func testAccIncidentAlertRouteV3AIInstructionsConfig(instructions string) string {
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
      enabled = false
      %[2]s
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
%[3]s
  }
}
`, StableSuffix("test-route-v3-ai-instructions"), aiInstructionsLine(instructions), alertRouteV3IncidentTemplateBlock)
}
