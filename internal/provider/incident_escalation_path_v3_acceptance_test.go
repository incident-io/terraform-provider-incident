package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// These cover an escalation path choosing between the V2 and V3 APIs against the real
// API. TestAccEscalationPath covers a V2 config. What they leave to the fake API in
// incident_escalation_path_version_fake_api_test.go is branching on urgency, since the
// test account's escalation urgency ids aren't something a config can look up.

// testAccEscalationPathConfigV3 is testAccEscalationPathConfig moved to V3: the same
// path, with each target's urgency renamed to interruption_level. It branches on working
// hours, which both versions test the same way.
func testAccEscalationPathConfigV3() string {
	return strings.ReplaceAll(testAccEscalationPathConfig(), `urgency = "`, `interruption_level = "`)
}

// TestAccEscalationPathV3 creates a path through V3, reads it back unchanged, and imports it.
func TestAccEscalationPathV3(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEscalationPathConfigV3(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_escalation_path.test", "sequences.in_hours.nodes.0.level.targets.0.interruption_level", "high"),
					resource.TestCheckNoResourceAttr("incident_escalation_path.test", "sequences.in_hours.nodes.0.level.targets.0.urgency"),
					resource.TestCheckResourceAttr("incident_escalation_path.test", "sequences.out_of_hours.nodes.0.level.targets.0.interruption_level", "low"),
				),
			},
			{
				ResourceName:            "incident_escalation_path.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"start", "sequences"},
			},
		},
	})
}

// TestAccEscalationPathMigrateToV3 moves a path written through V2 to V3, which is a
// config change applied as an update in place.
func TestAccEscalationPathMigrateToV3(t *testing.T) {
	pathID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEscalationPathConfig(),
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path.test", tfjsonpath.New("id")),
				},
			},
			{
				Config: testAccEscalationPathConfigV3(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_escalation_path.test", plancheck.ResourceActionUpdate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path.test", tfjsonpath.New("id")),
				},
				Check: resource.TestCheckResourceAttr("incident_escalation_path.test", "sequences.in_hours.nodes.0.level.targets.0.interruption_level", "high"),
			},
		},
	})
}

// v7ProviderVersion is the last release before escalation paths could use V3.
const v7ProviderVersion = "7.13.0"

// TestAccUpgradeEscalationPathV2Config checks that a V2 config applied by the last release
// plans no change on this one. Every target had to set urgency before, so every existing
// path is a V2 path.
func TestAccUpgradeEscalationPathV2Config(t *testing.T) {
	useReleasedProviderNamespace(t)

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"incident": {
						Source:            "incident-io/incident",
						VersionConstraint: "= " + v7ProviderVersion,
					},
				},
				Config: releasedProviderRequirement() + testAccEscalationPathConfig(),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   releasedProviderRequirement() + testAccEscalationPathConfig(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
