package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func TestIncidentCatalogEntryExtensionResource_ValidateCustomAliases(t *testing.T) {
	for _, alias := range []string{"", " padded", "padded "} {
		t.Run(fmt.Sprintf("%q", alias), func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
resource "incident_catalog_entry_extension" "test" {
  catalog_type_id = "01HXYZ"
  identifier      = "01HABC"
  custom_aliases  = [%q]
}
`, alias),
					PlanOnly: true,
					// Terraform wraps the message, so match only its start.
					ExpectError: regexp.MustCompile(`must not be empty, or start or end`),
				}},
			})
		})
	}
}

// Extends a schedule's synced entry with custom aliases. The middle step renames the
// schedule in the same apply, so the sync changes the entry while Terraform writes it:
// the extension tracks none of the synced fields, so this must apply cleanly.
func TestAccIncidentCatalogEntryExtensionResourceSchedule(t *testing.T) {
	suffix := uuid.NewString()[:8]
	var scheduleID string

	captureScheduleID := func(s *terraform.State) error {
		res, ok := s.RootModule().Resources["incident_schedule.test"]
		if !ok {
			return fmt.Errorf("incident_schedule.test is not in state")
		}
		scheduleID = res.Primary.ID
		return nil
	}

	// The sync creates a schedule's catalog entry after the schedule, so wait for it rather
	// than racing it: extending a missing entry is meant to fail.
	waitForScheduleEntry := func() {
		deadline := time.Now().Add(2 * time.Minute)
		for {
			entry, err := findScheduleCatalogEntry(scheduleID)
			if err == nil && entry != nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("catalog entry for schedule %s never appeared: %v", scheduleID, err)
			}
			time.Sleep(5 * time.Second)
		}
	}

	checkEntryReleased := func(s *terraform.State) error {
		entry, err := findScheduleCatalogEntry(scheduleID)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("expected the schedule's catalog entry to outlive the resource, but it is gone")
		}
		if len(entry.CustomAliases) > 0 {
			return fmt.Errorf("expected custom aliases to be cleared, got %v", entry.CustomAliases)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogEntryExtensionScheduleConfig("one", nil),
				Check:  captureScheduleID,
			},
			{
				PreConfig: waitForScheduleEntry,
				Config:    testAccIncidentCatalogEntryExtensionScheduleConfig("one", []string{"tf-acc-one-" + suffix, "tf-acc-two-" + suffix}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("incident_catalog_entry_extension.test", tfjsonpath.New("custom_aliases"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("tf-acc-one-" + suffix),
							knownvalue.StringExact("tf-acc-two-" + suffix),
						})),
				},
			},
			{
				Config: testAccIncidentCatalogEntryExtensionScheduleConfig("two", []string{"tf-acc-three-" + suffix}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_schedule.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("incident_catalog_entry_extension.test", plancheck.ResourceActionUpdate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("incident_catalog_entry_extension.test", tfjsonpath.New("custom_aliases"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("tf-acc-three-" + suffix)})),
				},
			},
			// Removing the resource keeps the schedule, so we can check its entry survived.
			{
				Config: testAccIncidentCatalogEntryExtensionScheduleConfig("two", nil),
				Check:  checkEntryReleased,
			},
		},
	})
}

// Extends an entry in a catalog type of the test's own, so it can add attributes without
// changing the schema of a type other tests share. incident_catalog_entry owns the
// entry and one attribute, and the extension owns another.
func TestAccIncidentCatalogEntryExtensionResourceAttributes(t *testing.T) {
	var entryID string

	captureEntryID := func(s *terraform.State) error {
		res, ok := s.RootModule().Resources["incident_catalog_entry.test"]
		if !ok {
			return fmt.Errorf("incident_catalog_entry.test is not in state")
		}
		entryID = res.Primary.ID
		return nil
	}

	checkAttributeValue := func(want *string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			res, ok := s.RootModule().Resources["incident_catalog_type_attribute.extended"]
			if !ok {
				return fmt.Errorf("incident_catalog_type_attribute.extended is not in state")
			}

			result, err := testClient.CatalogV3ShowEntryWithResponse(context.Background(), entryID, nil)
			if err != nil {
				return err
			}
			entry := result.JSON200.CatalogEntry

			binding, ok := entry.AttributeValues[res.Primary.ID]
			got := lo.If(ok && binding.Value != nil, lo.FromPtr(lo.FromPtr(binding.Value).Literal)).Else("")
			if lo.FromPtr(want) != got {
				return fmt.Errorf("expected the extended attribute to be %q, got %q", lo.FromPtr(want), got)
			}
			if _, ok := entry.AttributeValues[s.RootModule().Resources["incident_catalog_type_attribute.owned"].Primary.ID]; !ok {
				return fmt.Errorf("expected the extension to leave the attribute incident_catalog_entry owns alone")
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogEntryExtensionAttributesConfig(true, lo.ToPtr("first")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					captureEntryID,
					resource.TestCheckResourceAttrPair("incident_catalog_entry_extension.test", "id", "incident_catalog_entry.test", "id"),
					checkAttributeValue(lo.ToPtr("first")),
				),
			},
			{
				Config: testAccIncidentCatalogEntryExtensionAttributesConfig(true, lo.ToPtr("second")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: checkAttributeValue(lo.ToPtr("second")),
			},
			// Dropping the attribute from the extension clears it.
			{
				Config: testAccIncidentCatalogEntryExtensionAttributesConfig(true, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: checkAttributeValue(nil),
			},
			{
				Config: testAccIncidentCatalogEntryExtensionAttributesConfig(true, lo.ToPtr("third")),
				Check:  checkAttributeValue(lo.ToPtr("third")),
			},
			// Destroying the extension clears its attribute and leaves the entry.
			{
				Config: testAccIncidentCatalogEntryExtensionAttributesConfig(false, nil),
				Check:  checkAttributeValue(nil),
			},
		},
	})
}

func TestAccIncidentCatalogEntryExtensionResourceMissingEntry(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

resource "incident_catalog_entry_extension" "test" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  identifier      = "` + uuid.NewString() + `"
  custom_aliases  = ["never-written"]
}
`,
			ExpectError: regexp.MustCompile(`Catalog entry not found`),
		}},
	})
}

// findScheduleCatalogEntry returns the Schedule catalog entry synced for a schedule, or nil
// if it hasn't synced yet. The entry's external ID is the schedule's ID.
func findScheduleCatalogEntry(scheduleID string) (*client.CatalogEntryV3, error) {
	ctx := context.Background()
	catalogTypes, err := testClient.CatalogV3ListTypesWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing catalog types: %w", err)
	}
	scheduleType, ok := lo.Find(catalogTypes.JSON200.CatalogTypes, func(catalogType client.CatalogTypeV3) bool {
		return catalogType.TypeName == "Schedule"
	})
	if !ok {
		return nil, fmt.Errorf("no Schedule catalog type")
	}

	entries, err := testClient.CatalogV3ListEntriesWithResponse(ctx, &client.CatalogV3ListEntriesParams{
		CatalogTypeId: scheduleType.Id,
		Identifier:    &scheduleID,
		PageSize:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing schedule catalog entries: %w", err)
	}
	if len(entries.JSON200.CatalogEntries) == 0 {
		return nil, nil
	}

	return &entries.JSON200.CatalogEntries[0], nil
}

// testAccIncidentCatalogEntryExtensionScheduleConfig extends the test schedule's catalog
// entry with the given custom aliases, or leaves the entry alone when there are none.
func testAccIncidentCatalogEntryExtensionScheduleConfig(scheduleName string, customAliases []string) string {
	return testRunTemplate("incident_catalog_entry_extension_schedule", `
data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

resource "incident_schedule" "test" {
  name     = {{ stableSuffix (printf "acc-catalog-entry-extension-%s" .ScheduleName) | quote }}
  timezone = "Europe/London"
}
{{ if .CustomAliases }}
resource "incident_catalog_entry_extension" "test" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  identifier      = incident_schedule.test.id
  custom_aliases  = {{ toJson .CustomAliases }}
}
{{ end }}`, map[string]any{"ScheduleName": scheduleName, "CustomAliases": customAliases})
}

// testAccIncidentCatalogEntryExtensionAttributesConfig sets up a catalog type with an entry,
// and extends it with the given value for one attribute, or with no attributes when value
// is nil. withExtension false leaves the extension out.
func testAccIncidentCatalogEntryExtensionAttributesConfig(withExtension bool, value *string) string {
	return testRunTemplate("incident_catalog_entry_extension_attributes", `
resource "incident_catalog_type" "test" {
  name        = {{ stableSuffix "acc-catalog-entry-extension" | quote }}
  description = "Used in terraform acceptance tests for incident_catalog_entry_extension"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "owned" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Owned"
  type            = "String"
}

resource "incident_catalog_type_attribute" "extended" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Extended"
  type            = "String"
}

resource "incident_catalog_entry" "test" {
  catalog_type_id    = incident_catalog_type.test.id
  name               = "Extended entry"
  external_id        = "extended-entry"
  managed_attributes = [incident_catalog_type_attribute.owned.id]
  attribute_values = [
    {
      attribute = incident_catalog_type_attribute.owned.id
      value     = "owned"
    },
  ]
}
{{ if .WithExtension }}
resource "incident_catalog_entry_extension" "test" {
  catalog_type_id = incident_catalog_type.test.id
  identifier      = incident_catalog_entry.test.external_id
  attribute_values = [
{{- if .Value }}
    {
      attribute = incident_catalog_type_attribute.extended.id
      value     = {{ .Value | quote }}
    },
{{- end }}
  ]
}
{{ end }}`, map[string]any{"WithExtension": withExtension, "Value": lo.FromPtr(value)})
}
