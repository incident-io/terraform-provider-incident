package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccIncidentTeamGroupingPreferenceResource needs team grouping preferences enabled
// for the test organisation, as the API refuses every write otherwise.
func TestAccIncidentTeamGroupingPreferenceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentTeamGroupingPreferenceConfig(tgpSettingsEnabled(1800, "rolling")),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_team_grouping_preference.test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttrPair("incident_team_grouping_preference.test", "team_id", "incident_catalog_entry.team", "id"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "version", "1"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.enabled", "true"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.window_seconds", "1800"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.window_type", "rolling"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.grouping_keys.#", "1"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.grouping_keys.0.reference", "alert.title"),
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
			{
				ResourceName:      "incident_team_grouping_preference.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Changing the window is an update in place, and writes the next version.
			{
				Config: testAccIncidentTeamGroupingPreferenceConfig(tgpSettingsEnabled(900, "fixed")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_team_grouping_preference.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "version", "2"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.window_seconds", "900"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.window_type", "fixed"),
				),
			},
			// Switching grouping off drops the window and keys from state, as the API does.
			{
				Config: testAccIncidentTeamGroupingPreferenceConfig(`
      enabled = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "version", "3"),
					resource.TestCheckResourceAttr("incident_team_grouping_preference.test", "default.settings.enabled", "false"),
					resource.TestCheckNoResourceAttr("incident_team_grouping_preference.test", "default.settings.window_seconds"),
					resource.TestCheckNoResourceAttr("incident_team_grouping_preference.test", "default.settings.window_type"),
					resource.TestCheckNoResourceAttr("incident_team_grouping_preference.test", "default.settings.grouping_keys"),
				),
			},
		},
	})
}

func tgpSettingsEnabled(windowSeconds int, windowType string) string {
	return testRunTemplate("incident_team_grouping_preference_settings", `
      enabled        = true
      window_type    = {{ .WindowType | quote }}
      window_seconds = {{ .WindowSeconds }}
      grouping_keys  = [{ reference = "alert.title" }]`, struct {
		WindowSeconds int
		WindowType    string
	}{WindowSeconds: windowSeconds, WindowType: windowType})
}

// testAccIncidentTeamGroupingPreferenceConfig creates a team of its own to hold the
// preference, because a team can have only one and the shared test teams may already.
func testAccIncidentTeamGroupingPreferenceConfig(settings string) string {
	return testRunTemplate("incident_team_grouping_preference", `
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

resource "incident_team_grouping_preference" "test" {
  team_id = incident_catalog_entry.team.id

  default = {
    settings = {
{{ .Settings }}
    }
  }
}
`, struct {
		TeamTypeName string
		TeamName     string
		Settings     string
	}{
		TeamTypeName: teamTypeName(),
		TeamName:     StableSuffix("tf-acceptance-test-grouping-preference-team"),
		Settings:     settings,
	})
}
