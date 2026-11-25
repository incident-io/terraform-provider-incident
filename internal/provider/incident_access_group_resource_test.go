package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// skipUnlessAccessGroups gates the access group and grant tests. The API behind them is not
// yet documented or released, and the API key CI runs with may not have the access group
// and grant scopes yet, so these run only when TF_ACC_ACCESS_GROUPS is set.
func skipUnlessAccessGroups(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC_ACCESS_GROUPS") == "" {
		t.Skip("No TF_ACC_ACCESS_GROUPS environment variable set, skipping")
	}
}

// TestAccIncidentAccessGroupResource covers a custom access group and its grant: created,
// looked up, renamed, given different privileges, and imported.
func TestAccIncidentAccessGroupResource(t *testing.T) {
	skipUnlessAccessGroups(t)

	name := StableSuffix("Terraform access group")
	slug := strings.ToLower(StableSuffix("tf-access-group"))
	var grantID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAccessGroupConfig(name, slug, "Platform engineers", []string{"catalog_types.create"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_access_group.test", "name", name),
					resource.TestCheckResourceAttr("incident_access_group.test", "slug", slug),
					resource.TestCheckResourceAttr("incident_access_group.test", "description", "Platform engineers"),
					resource.TestCheckResourceAttr("incident_access_group.test", "type", "custom"),
					resource.TestCheckResourceAttrPair(
						"incident_access_grant.test", "principal.id",
						"incident_access_group.test", "id",
					),
					resource.TestCheckResourceAttr("incident_access_grant.test", "resource.type", "organization"),
					resource.TestCheckResourceAttr("incident_access_grant.test", "privileges.#", "1"),
					resource.TestCheckTypeSetElemAttr("incident_access_grant.test", "privileges.*", "catalog_types.create"),
					captureAttr("incident_access_grant.test", "id", &grantID),

					// Both lookups find the group.
					resource.TestCheckResourceAttrPair(
						"data.incident_access_group.by_id", "slug",
						"incident_access_group.test", "slug",
					),
					resource.TestCheckResourceAttrPair(
						"data.incident_access_group.by_slug", "id",
						"incident_access_group.test", "id",
					),
					// Every organization has the base groups.
					resource.TestCheckResourceAttr("data.incident_access_group.standard", "type", "base"),
					resource.TestCheckResourceAttrSet("data.incident_access_privileges.all", "privileges.0.name"),
				),
			},
			{
				ResourceName:            "incident_access_group.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"unlock_in_dashboard"},
			},
			{
				ResourceName:            "incident_access_grant.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"unlock_in_dashboard"},
			},
			{
				// A new name, no description, and different privileges. The grant keeps its
				// ID: editing privileges changes a grant in place.
				Config: testAccIncidentAccessGroupConfig(name+" renamed", slug, "", []string{"catalog_types.create", "schedules.create"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_access_group.test", "name", name+" renamed"),
					resource.TestCheckResourceAttr("incident_access_group.test", "slug", slug),
					resource.TestCheckNoResourceAttr("incident_access_group.test", "description"),
					resource.TestCheckResourceAttr("incident_access_grant.test", "privileges.#", "2"),
					resource.TestCheckTypeSetElemAttr("incident_access_grant.test", "privileges.*", "schedules.create"),
					func(s *terraform.State) error {
						return resource.TestCheckResourceAttr("incident_access_grant.test", "id", grantID)(s)
					},
				),
			},
		},
	})
}

// captureAttr copies an attribute out of state, so a later step can compare against it.
func captureAttr(resourceName, attr string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		*into = rs.Primary.Attributes[attr]
		return nil
	}
}

func testAccIncidentAccessGroupConfig(name, slug, description string, privileges []string) string {
	descriptionLine := ""
	if description != "" {
		descriptionLine = fmt.Sprintf("description = %q", description)
	}

	quoted := make([]string, len(privileges))
	for i, privilege := range privileges {
		quoted[i] = fmt.Sprintf("%q", privilege)
	}

	return fmt.Sprintf(`
resource "incident_access_group" "test" {
  name = %q
  slug = %q
  %s
}

resource "incident_access_grant" "test" {
  principal = {
    type = "access_group"
    id   = incident_access_group.test.id
  }
  resource = {
    type = "organization"
  }
  privileges = [%s]
}

data "incident_access_group" "by_id" {
  id = incident_access_group.test.id
}

data "incident_access_group" "by_slug" {
  slug = incident_access_group.test.slug
}

data "incident_access_group" "standard" {
  slug = "standard"
}

data "incident_access_privileges" "all" {}
`, name, slug, descriptionLine, strings.Join(quoted, ", "))
}
