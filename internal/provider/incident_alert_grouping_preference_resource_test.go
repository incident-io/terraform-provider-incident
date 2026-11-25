package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const groupingPreferenceAddress = "incident_alert_grouping_preference.test"

// TestAccIncidentAlertGroupingPreferenceResource needs an organisation on alert rules, as the API
// refuses to add or change a team's branches otherwise, so it skips unless TF_ACC_ALERT_RULES
// is set.
func TestAccIncidentAlertGroupingPreferenceResource(t *testing.T) {
	if os.Getenv("TF_ACC_ALERT_RULES") == "" {
		t.Skip("No TF_ACC_ALERT_RULES environment variable set, skipping")
	}

	// The branch keeps its ID when another is inserted ahead of it, or its alert groups would
	// pass to the new one.
	branchID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAlertGroupingPreferenceConfig(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(groupingPreferenceAddress, "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr(groupingPreferenceAddress, "kind", "team"),
					resource.TestCheckResourceAttrPair(groupingPreferenceAddress, "team_id", "incident_catalog_entry.team", "id"),
					resource.TestCheckResourceAttr(groupingPreferenceAddress, "version", "1"),
					resource.TestCheckResourceAttr(groupingPreferenceAddress, "branch.#", "1"),
					resource.TestMatchResourceAttr(groupingPreferenceAddress, "branch.0.id", regexp.MustCompile("^[0-7][0-9A-HJKMNP-TV-Z]{25}$")),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					branchID.AddStateValue(groupingPreferenceAddress, tfjsonpath.New("branch").AtSliceIndex(0).AtMapKey("id")),
				},
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
			{
				ResourceName:      groupingPreferenceAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccIncidentAlertGroupingPreferenceConfig(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(groupingPreferenceAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(groupingPreferenceAddress, "version", "2"),
					resource.TestCheckResourceAttr(groupingPreferenceAddress, "branch.#", "2"),
					resource.TestCheckResourceAttr(groupingPreferenceAddress, "branch.0.condition_groups.0.conditions.0.subject", "alert.description"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					branchID.AddStateValue(groupingPreferenceAddress, tfjsonpath.New("branch").AtSliceIndex(1).AtMapKey("id")),
				},
			},
		},
	})
}

// testAccIncidentAlertGroupingPreferenceConfig creates a team of its own, because a team can have
// only one preference and the shared test teams may already. insertBranch adds a branch
// ahead of the existing one.
func testAccIncidentAlertGroupingPreferenceConfig(insertBranch bool) string {
	return testRunTemplate("incident_alert_grouping_preference", `
data "incident_catalog_type" "team" {
  name = {{ .TeamTypeName | quote }}
}

resource "incident_catalog_entry" "team" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = {{ .TeamName | quote }}
  name               = {{ .TeamName | quote }}
  attribute_values   = []
  managed_attributes = []
}

resource "incident_alert_grouping_preference" "test" {
  kind    = "team"
  team_id = incident_catalog_entry.team.id

  default = {
    settings = {
      enabled        = true
      window_type    = "rolling"
      window_seconds = 1800
      grouping_keys  = [{ reference = "alert.title" }]
    }
  }
{{ if .InsertBranch }}
  branch {
    condition_groups = [{
      conditions = [{ subject = "alert.description", operation = "is_set", param_bindings = [] }]
    }]
    settings = { enabled = false }
  }
{{ end }}
  branch {
    condition_groups = [{
      conditions = [{ subject = "alert.title", operation = "is_set", param_bindings = [] }]
    }]
    settings = {
      enabled        = true
      window_type    = "fixed"
      window_seconds = 600
    }
  }
}
`, struct {
		TeamTypeName string
		TeamName     string
		InsertBranch bool
	}{
		TeamTypeName: teamTypeName(),
		TeamName:     StableSuffix("tf-acceptance-test-grouping-preference-branches-team"),
		InsertBranch: insertBranch,
	})
}
