package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccIncidentScheduleBeta covers the lifecycle of a schedule on its own:
// create, read back unchanged, import, and update.
//
// Nothing had applied this resource against the API before now - its other
// tests all work on the model and the schema - and v7 makes it the only way to
// manage a schedule. The empty-plan check after each apply is the part that
// earns its keep: it catches a field the API returns in a shape the resource
// can't store, which is how every "provider produced inconsistent result" bug
// on this resource's neighbours has started.
func TestAccIncidentScheduleBeta(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentScheduleBetaConfig("acc-schedule", `["GB"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_beta.test", "name", StableSuffix("acc-schedule")),
					resource.TestCheckResourceAttr("incident_schedule_beta.test", "timezone", "Europe/London"),
					resource.TestCheckResourceAttr("incident_schedule_beta.test", "holidays_public_config.country_codes.#", "1"),
					resource.TestCheckResourceAttr("incident_schedule_beta.test", "holidays_public_config.country_codes.0", "GB"),
					resource.TestCheckResourceAttrSet("incident_schedule_beta.test", "id"),
				),
			},
			{
				ResourceName:      "incident_schedule_beta.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccIncidentScheduleBetaConfig("acc-schedule-renamed", `["GB", "FR"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_beta.test", "name", StableSuffix("acc-schedule-renamed")),
					resource.TestCheckResourceAttr("incident_schedule_beta.test", "holidays_public_config.country_codes.#", "2"),
				),
			},
			// Omitting the block is how a schedule says it shows no public
			// holidays, so removing it has to clear the two set above rather
			// than leave them in place.
			{
				Config: testAccIncidentScheduleBetaWithoutHolidays("acc-schedule-renamed"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_schedule_beta.test", "holidays_public_config.country_codes.#"),
				),
			},
		},
	})
}

func testAccIncidentScheduleBetaConfig(name, countryCodes string) string {
	return testRunTemplate("incident_schedule_beta_acc", `
resource "incident_schedule_beta" "test" {
  name     = {{ quote .Name }}
  timezone = "Europe/London"

  holidays_public_config = {
    country_codes = {{ .CountryCodes }}
  }
}
`, struct {
		Name         string
		CountryCodes string
	}{
		Name:         StableSuffix(name),
		CountryCodes: countryCodes,
	})
}

func testAccIncidentScheduleBetaWithoutHolidays(name string) string {
	return testRunTemplate("incident_schedule_beta_acc_no_holidays", `
resource "incident_schedule_beta" "test" {
  name     = {{ quote .Name }}
  timezone = "Europe/London"
}
`, struct {
		Name string
	}{
		Name: StableSuffix(name),
	})
}
