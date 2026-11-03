package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"
)

func TestIncidentCatalogEntryAttributeValueResource_ValidateValue(t *testing.T) {
	for name, values := range map[string]string{
		"neither":             ``,
		"both":                `value = "one"` + "\n" + `array_value = ["two"]`,
		"empty value":         `value = ""`,
		"empty array":         `array_value = []`,
		"empty array element": `array_value = [""]`,
	} {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
resource "incident_catalog_entry_attribute_value" "test" {
  entry_id     = "01HABC"
  attribute_id = "01HXYZ"
  %s
}
`, values),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(`(Invalid|Missing) Attribute`),
				}},
			})
		})
	}
}

// Sets two attributes on an entry in a catalog type of the test's own, so it can add
// attributes without changing the schema of a type other tests share. The two resources
// write the same entry in the same apply, and incident_catalog_entry owns a third
// attribute: none of them may undo another's write.
func TestAccIncidentCatalogEntryAttributeValueResource(t *testing.T) {
	var entryID string

	captureEntryID := func(s *terraform.State) error {
		res, ok := s.RootModule().Resources["incident_catalog_entry.test"]
		if !ok {
			return fmt.Errorf("incident_catalog_entry.test is not in state")
		}
		entryID = res.Primary.ID
		return nil
	}

	// checkValues checks each attribute's value on the entry, with "" for no value.
	checkValues := func(want map[string]string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			result, err := testClient.CatalogV3ShowEntryWithResponse(context.Background(), entryID, nil)
			if err != nil {
				return err
			}
			entry := result.JSON200.CatalogEntry

			for name, wantValue := range want {
				res, ok := s.RootModule().Resources["incident_catalog_type_attribute."+name]
				if !ok {
					return fmt.Errorf("incident_catalog_type_attribute.%s is not in state", name)
				}
				binding, ok := entry.AttributeValues[res.Primary.ID]
				got := lo.If(ok && binding.Value != nil, lo.FromPtr(lo.FromPtr(binding.Value).Literal)).Else("")
				if got != wantValue {
					return fmt.Errorf("expected attribute %s to be %q, got %q", name, wantValue, got)
				}
			}
			return nil
		}
	}

	emptyPlan := resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:           testAccIncidentCatalogEntryAttributeValueConfig(map[string]string{"first": "one", "second": "two"}),
				ConfigPlanChecks: emptyPlan,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureEntryID,
					checkValues(map[string]string{"owned": "owned", "first": "one", "second": "two"}),
				),
			},
			{
				Config:           testAccIncidentCatalogEntryAttributeValueConfig(map[string]string{"first": "three", "second": "four"}),
				ConfigPlanChecks: emptyPlan,
				Check:            checkValues(map[string]string{"owned": "owned", "first": "three", "second": "four"}),
			},
			{
				ResourceName:      "incident_catalog_entry_attribute_value.first",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Destroying one resource clears its attribute and leaves the rest of the entry.
			{
				Config:           testAccIncidentCatalogEntryAttributeValueConfig(map[string]string{"second": "four"}),
				ConfigPlanChecks: emptyPlan,
				Check:            checkValues(map[string]string{"owned": "owned", "first": "", "second": "four"}),
			},
			{
				Config: testAccIncidentCatalogEntryAttributeValueConfig(nil),
				Check:  checkValues(map[string]string{"owned": "owned", "first": "", "second": ""}),
			},
		},
	})
}

// testAccIncidentCatalogEntryAttributeValueConfig sets up a catalog type with an entry
// that incident_catalog_entry owns, and an incident_catalog_entry_attribute_value for each
// of the first and second attributes in values.
func testAccIncidentCatalogEntryAttributeValueConfig(values map[string]string) string {
	return testRunTemplate("incident_catalog_entry_attribute_value", `
resource "incident_catalog_type" "test" {
  name        = {{ stableSuffix "acc-catalog-entry-attribute-value" | quote }}
  description = "Used in terraform acceptance tests for incident_catalog_entry_attribute_value"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "owned" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Owned"
  type            = "String"
}

resource "incident_catalog_type_attribute" "first" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "First"
  type            = "String"
}

resource "incident_catalog_type_attribute" "second" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Second"
  type            = "String"
}

resource "incident_catalog_entry" "test" {
  catalog_type_id    = incident_catalog_type.test.id
  name               = "Test entry"
  external_id        = "test-entry"
  managed_attributes = [incident_catalog_type_attribute.owned.id]
  attribute_values = [
    {
      attribute = incident_catalog_type_attribute.owned.id
      value     = "owned"
    },
  ]
}
{{ range $name, $value := .Values }}
resource "incident_catalog_entry_attribute_value" "{{ $name }}" {
  entry_id     = incident_catalog_entry.test.id
  attribute_id = incident_catalog_type_attribute.{{ $name }}.id
  value        = {{ $value | quote }}
}
{{ end }}`, map[string]any{"Values": values})
}
