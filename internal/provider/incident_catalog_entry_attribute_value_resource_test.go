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

// Sets two attributes, which Terraform adds to the Schedule type, on a schedule's synced
// entry. The two resources write the same entry in the same apply, so neither may undo
// the other's write.
func TestAccIncidentCatalogEntryAttributeValueResource(t *testing.T) {
	var scheduleID, entryID string

	captureScheduleID := func(s *terraform.State) error {
		res, ok := s.RootModule().Resources["incident_schedule.test"]
		if !ok {
			return fmt.Errorf("incident_schedule.test is not in state")
		}
		scheduleID = res.Primary.ID
		return nil
	}

	captureEntryID := func(s *terraform.State) error {
		res, ok := s.RootModule().Resources["data.incident_catalog_entry.schedule"]
		if !ok {
			return fmt.Errorf("data.incident_catalog_entry.schedule is not in state")
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
				Config: testAccIncidentCatalogEntryAttributeValueConfig(false, nil),
				Check:  captureScheduleID,
			},
			{
				PreConfig:        func() { waitForScheduleCatalogEntry(t, scheduleID) },
				Config:           testAccIncidentCatalogEntryAttributeValueConfig(true, map[string]string{"first": "one", "second": "two"}),
				ConfigPlanChecks: emptyPlan,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureEntryID,
					checkValues(map[string]string{"first": "one", "second": "two"}),
				),
			},
			{
				Config:           testAccIncidentCatalogEntryAttributeValueConfig(true, map[string]string{"first": "three", "second": "four"}),
				ConfigPlanChecks: emptyPlan,
				Check:            checkValues(map[string]string{"first": "three", "second": "four"}),
			},
			{
				ResourceName:      "incident_catalog_entry_attribute_value.first",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Destroying one resource clears its attribute and leaves the rest of the entry.
			{
				Config:           testAccIncidentCatalogEntryAttributeValueConfig(true, map[string]string{"second": "four"}),
				ConfigPlanChecks: emptyPlan,
				Check:            checkValues(map[string]string{"first": "", "second": "four"}),
			},
			{
				Config: testAccIncidentCatalogEntryAttributeValueConfig(true, nil),
				Check:  checkValues(map[string]string{"first": "", "second": ""}),
			},
		},
	})
}

// An entry that incident_catalog_entry owns belongs to Terraform already, so the resource
// refuses to write it.
func TestAccIncidentCatalogEntryAttributeValueResourceUnmanagedType(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testRunTemplate("incident_catalog_entry_attribute_value_unmanaged", `
resource "incident_catalog_type" "test" {
  name        = {{ stableSuffix "acc-catalog-entry-attribute-value" | quote }}
  description = "Used in terraform acceptance tests for incident_catalog_entry_attribute_value"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "test" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Test"
  type            = "String"
}

resource "incident_catalog_entry" "test" {
  catalog_type_id  = incident_catalog_type.test.id
  name             = "Test entry"
  attribute_values = []
}

resource "incident_catalog_entry_attribute_value" "test" {
  entry_id     = incident_catalog_entry.test.id
  attribute_id = incident_catalog_type_attribute.test.id
  value        = "never-written"
}
`, nil),
			ExpectError: regexp.MustCompile(`Catalog type isn't managed by incident.io`),
		}},
	})
}

// testAccIncidentCatalogEntryAttributeValueConfig creates a schedule, and adds first and
// second attributes to the Schedule type. With withValues, it also looks up the
// schedule's entry and sets each attribute in values on it.
func testAccIncidentCatalogEntryAttributeValueConfig(withValues bool, values map[string]string) string {
	return testRunTemplate("incident_catalog_entry_attribute_value", `
data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

resource "incident_schedule" "test" {
  name     = {{ stableSuffix "acc-catalog-entry-attribute-value" | quote }}
  timezone = "Europe/London"
}

resource "incident_catalog_type_attribute" "first" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  name            = {{ stableSuffix "acc-first" | quote }}
  type            = "String"
}

resource "incident_catalog_type_attribute" "second" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  name            = {{ stableSuffix "acc-second" | quote }}
  type            = "String"
}
{{ if .WithValues }}
data "incident_catalog_entry" "schedule" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  identifier      = incident_schedule.test.id
}
{{ range $name, $value := .Values }}
resource "incident_catalog_entry_attribute_value" "{{ $name }}" {
  entry_id     = data.incident_catalog_entry.schedule.id
  attribute_id = incident_catalog_type_attribute.{{ $name }}.id
  value        = {{ $value | quote }}
}
{{ end }}{{ end }}`, map[string]any{"WithValues": withValues, "Values": values})
}
