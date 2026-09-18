package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// TestAccUnlockInDashboard checks the claim the attribute controls actually lands in the
// account, in all three directions: created unlocked, locked by dropping the attribute,
// then unlocked again.
//
// It runs against incident_workflow because the workflow API is the one that reports
// management_meta publicly, so a test can read the claim back rather than infer it. The
// mechanism is also the most involved one: the API marks any write that isn't
// Terraform as managed externally, so an unlocked workflow is only handed back to the
// dashboard because the provider asks it to be, and that second call is what this pins.
//
// Terraform state says nothing about any of this - unlock_in_dashboard is configuration
// the API cannot answer for - so every assertion here goes to the API.
func TestAccUnlockInDashboard(t *testing.T) {
	var workflowID string

	captureID := func(s *terraform.State) error {
		res, ok := s.RootModule().Resources["incident_workflow.example"]
		if !ok {
			return fmt.Errorf("incident_workflow.example is not in state")
		}
		workflowID = res.Primary.ID

		return nil
	}

	// checkManagedBy reads the workflow back and fails unless the account agrees about
	// who manages it. "dashboard" is unclaimed and editable; "terraform" is claimed and
	// locked; "external" means the provider left the marker the API writes for any other
	// client, which for us is a bug rather than a third option.
	checkManagedBy := func(want client.ManagementMetaV2ManagedBy) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if workflowID == "" {
				return fmt.Errorf("workflow ID was not captured")
			}

			result, err := testClient.WorkflowsV2ShowWorkflowWithResponse(context.Background(), workflowID, nil)
			if err != nil {
				return fmt.Errorf("reading workflow %s: %w", workflowID, err)
			}
			if result.JSON200 == nil {
				return fmt.Errorf("reading workflow %s: unexpected response (status %s)", workflowID, result.Status())
			}

			if got := result.JSON200.ManagementMeta.ManagedBy; got != want {
				return fmt.Errorf("workflow %s is managed_by %q, want %q", workflowID, got, want)
			}

			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Created unlocked: Terraform manages the workflow without claiming it.
			{
				Config: testAccUnlockInDashboardConfig(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "unlock_in_dashboard", "true"),
					captureID,
					checkManagedBy(client.ManagementMetaV2ManagedByDashboard),
				),
			},
			// Dropping the attribute claims it, which is what locks the dashboard.
			{
				Config: testAccUnlockInDashboardConfig(false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_workflow.example", "unlock_in_dashboard"),
					checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
				),
			},
			// Setting it again hands a workflow Terraform had already claimed back. This
			// is the direction with no endpoint of its own: the claim endpoint reads an
			// empty annotation set as the dashboard.
			{
				Config: testAccUnlockInDashboardConfig(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_workflow.example", "unlock_in_dashboard", "true"),
					checkManagedBy(client.ManagementMetaV2ManagedByDashboard),
				),
			},
		},
	})
}

// testAccUnlockInDashboardConfig is the workflow the resource tests use, with the
// attribute set or left out entirely. Left out rather than false, so the steps also cover
// the default an existing configuration has.
func testAccUnlockInDashboardConfig(unlock bool) string {
	return testAccIncidentWorkflowResourceConfig(&workflowTemplateOverrides{
		Name:              StableSuffix("acc-unlock"),
		UnlockInDashboard: unlock,
	})
}
