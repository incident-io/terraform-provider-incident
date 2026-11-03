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

func TestIncidentCatalogEntryAliasesResource_ValidateAliases(t *testing.T) {
	for _, alias := range []string{"", " padded", "padded "} {
		t.Run(fmt.Sprintf("%q", alias), func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
resource "incident_catalog_entry_aliases" "test" {
  entry_id = "01HABC"
  aliases  = [%q]
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

// Adds aliases to a schedule's synced entry. The middle step renames the schedule in the
// same apply, so the sync changes the entry while Terraform writes it: the resource tracks
// none of the synced fields, so this must apply cleanly.
func TestAccIncidentCatalogEntryAliasesResourceSchedule(t *testing.T) {
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
	// than racing it: the data source fails until the entry exists.
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
				Config: testAccIncidentCatalogEntryAliasesScheduleConfig("one", nil),
				Check:  captureScheduleID,
			},
			{
				PreConfig: waitForScheduleEntry,
				Config:    testAccIncidentCatalogEntryAliasesScheduleConfig("one", []string{"tf-acc-one-" + suffix, "tf-acc-two-" + suffix}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("incident_catalog_entry_aliases.test", tfjsonpath.New("aliases"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("tf-acc-one-" + suffix),
							knownvalue.StringExact("tf-acc-two-" + suffix),
						})),
				},
			},
			{
				Config: testAccIncidentCatalogEntryAliasesScheduleConfig("two", []string{"tf-acc-three-" + suffix}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_schedule.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("incident_catalog_entry_aliases.test", plancheck.ResourceActionUpdate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("incident_catalog_entry_aliases.test", tfjsonpath.New("aliases"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("tf-acc-three-" + suffix)})),
				},
			},
			{
				ResourceName:                         "incident_catalog_entry_aliases.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "entry_id",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["incident_catalog_entry_aliases.test"].Primary.Attributes["entry_id"], nil
				},
			},
			// Removing the resource keeps the schedule, so we can check its entry survived.
			{
				Config: testAccIncidentCatalogEntryAliasesScheduleConfig("two", nil),
				Check:  checkEntryReleased,
			},
		},
	})
}

func TestAccIncidentCatalogEntryAliasesResourceMissingEntry(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
resource "incident_catalog_entry_aliases" "test" {
  entry_id = "` + uuid.NewString() + `"
  aliases  = ["never-written"]
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

// testAccIncidentCatalogEntryAliasesScheduleConfig adds the given aliases to the test
// schedule's catalog entry, or leaves the entry alone when there are none.
func testAccIncidentCatalogEntryAliasesScheduleConfig(scheduleName string, aliases []string) string {
	return testRunTemplate("incident_catalog_entry_aliases_schedule", `
data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

resource "incident_schedule" "test" {
  name     = {{ stableSuffix (printf "acc-catalog-entry-aliases-%s" .ScheduleName) | quote }}
  timezone = "Europe/London"
}
{{ if .Aliases }}
data "incident_catalog_entry" "schedule" {
  catalog_type_id = data.incident_catalog_type.schedule.id
  identifier      = incident_schedule.test.id
}

resource "incident_catalog_entry_aliases" "test" {
  entry_id = data.incident_catalog_entry.schedule.id
  aliases  = {{ toJson .Aliases }}
}
{{ end }}`, map[string]any{"ScheduleName": scheduleName, "Aliases": aliases})
}
