package provider

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"
	"text/template"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func TestAccIncidentCatalogEntryResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogEntryResourceConfig("One", "This is the first entry", []string{}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry.example", "name", "One"),
				),
			},
			// Import
			{
				ResourceName:      "incident_catalog_entry.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCatalogEntryResourceConfig("Two", "This is the second entry", []string{}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry.example", "name", "Two"),
				),
			},
		},
	})
}

func TestAccIncidentCatalogEntryResourceWithAlias(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogEntryResourceConfig("One", "This is the first entry", []string{"one"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry.example", "name", "One"),
				),
			},
			// Import
			{
				ResourceName:      "incident_catalog_entry.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCatalogEntryResourceConfig("Two", "This is the second entry", []string{"two"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry.example", "name", "Two"),
				),
			},
		},
	})
}

func TestAccIncidentCatalogEntryResourceWithManagedAttributes(t *testing.T) {
	// Use a stable ID across steps - we want to edit the same one over and over
	testEntryID := uuid.NewString()

	// checkUsefulIsTrue checks that:
	// 1. The Description attribute has the expected value
	// 2. The Useful attribute is still set to "true"
	// This verifies that managed_attributes works correctly by only updating attributes we specify
	checkUsefulIsTrue := func(description string) func(s *terraform.State) error {
		return func(s *terraform.State) error {
			resources := s.RootModule().Resources

			// Verify required resources exist in state
			resourceIDs := map[string]string{}
			for _, name := range []string{
				"incident_catalog_type.example",
				"incident_catalog_entry.example",
				"incident_catalog_type_attribute.example_description",
				"incident_catalog_type_attribute.example_bool",
			} {
				if res, ok := resources[name]; !ok {
					return fmt.Errorf("required resource %s not found in state", name)
				} else {
					resourceIDs[name] = res.Primary.ID
				}
			}

			// Fetch all entries for this catalog type
			entries, err := testClient.CatalogV3ListEntriesWithResponse(context.Background(), &client.CatalogV3ListEntriesParams{
				CatalogTypeId: resourceIDs["incident_catalog_type.example"],
				PageSize:      250,
			})
			if err != nil {
				return fmt.Errorf("error fetching catalog entries: %w", err)
			}

			// Find our specific entry
			entryID := resourceIDs["incident_catalog_entry.example"]
			entry, found := lo.Find(entries.JSON200.CatalogEntries, func(e client.CatalogEntryV3) bool {
				return e.Id == entryID
			})
			if !found {
				return fmt.Errorf("entry (%s) not found in API response", entryID)
			}

			// Check both attributes
			expectedValues := map[string]struct {
				attributeID string
				value       string
			}{
				"Description": {
					attributeID: resourceIDs["incident_catalog_type_attribute.example_description"],
					value:       description,
				},
				"Useful": {
					attributeID: resourceIDs["incident_catalog_type_attribute.example_bool"],
					value:       "true",
				},
			}

			for name, expected := range expectedValues {
				attrID := expected.attributeID

				// Check attribute exists
				binding, ok := entry.AttributeValues[attrID]
				if !ok {
					return fmt.Errorf("expected %s attribute to be present: it is not\n\n%s", name, spew.Sdump(entry))
				}

				// Check attribute is a literal value (not an array)
				if binding.Value == nil {
					return fmt.Errorf("expected %s attribute to be a literal value: it is nil or an array\n\n%s", name, spew.Sdump(entry))
				}

				// Check attribute has expected value
				value := binding.Value.Literal
				if value == nil || *value != expected.value {
					return fmt.Errorf("expected %s attribute to be %q: got %q",
						name, expected.value, lo.FromPtrOr(value, "nil"))
				}
			}

			return nil
		}
	}

	conf := testAccIncidentCatalogEntryResourceConfigWithID(testEntryID, "Partial Update", "Updated description only", []string{}, true)

	_ = conf
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// First create with both attributes (Useful=true)
			{
				Config: testAccIncidentCatalogEntryResourceConfigWithID(testEntryID, "Initial", "Initial description", []string{}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entry.example", "name", "Initial"),
					checkUsefulIsTrue("Initial description"),
				),
			},
			// Now switch to managed_attributes for description only and update description
			// The Useful attribute should remain true from previous step
			{
				Config: testAccIncidentCatalogEntryResourceConfigWithID(testEntryID, "Partial Update", "Updated description only", []string{}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entry.example", "name", "Partial Update"),
					checkUsefulIsTrue("Updated description only"),
				),
			},
			// Update description again with managed_attributes
			// Useful should still remain unchanged from first step
			{
				Config: testAccIncidentCatalogEntryResourceConfigWithID(testEntryID, "Another Update", "Another description change", []string{}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entry.example", "name", "Another Update"),
					checkUsefulIsTrue("Another description change"),
				),
			},
		},
	})
}

func TestIncidentCatalogEntryResource_ValidateConfig(t *testing.T) {
	description := "desc-123"
	priority := "priority-456"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Test validation error when attribute isn't in managed_attributes
			{
				Config: fmt.Sprintf(`
resource "incident_catalog_entry" "test" {
  catalog_type_id = "catalog-type-id-123"
  name = "Test Entry"

  # Only manage the description attribute
  managed_attributes = ["%s"]

  attribute_values = [
    {
      # This is managed, should be fine
      attribute = "%s"
      value = "A description"
    },
    {
      # This is not managed, should cause an error
      attribute = "%s"
      value = "High"
    }
  ]
}
`, description, description, priority),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`specified in attribute_values`),
			},
		},
	})
}

func TestAccIncidentCatalogEntryResourceWithEmptyManagedAttributes(t *testing.T) {
	// Use a stable ID across steps
	testEntryID := uuid.NewString()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create initial resource without managed_attributes
			{
				Config: testAccIncidentCatalogEntryResourceConfigWithID(testEntryID, "Initial", "Initial description", []string{}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entry.example", "name", "Initial"),
				),
			},
			// Update with empty managed_attributes (should have no effect on syncing)
			{
				Config: testAccIncidentCatalogEntryResourceConfigWithEmptyManagedAttributes(testEntryID, "Initial", "Initial description", []string{}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entry.example", "name", "Initial"),
					// Plan should be empty - this is the key test - showing empty managed_attributes doesn't change anything
				),
			},
			// Change the name with empty managed_attributes
			{
				Config: testAccIncidentCatalogEntryResourceConfigWithEmptyManagedAttributes(testEntryID, "Updated", "Initial description", []string{}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entry.example", "name", "Updated"),
				),
			},
		},
	})
}

func TestAccIncidentCatalogEntryResourceManagedAttributesDestroy(t *testing.T) {
	// This test verifies that when managed_attributes is set, destroying the
	// Terraform resource clears the managed attributes instead of deleting the entry.
	testID := uuid.NewString()

	var entryID string
	var descriptionAttrID string

	// Capture resource IDs during step 1
	captureIDs := func(s *terraform.State) error {
		resources := s.RootModule().Resources

		if res, ok := resources["incident_catalog_entry.example"]; ok {
			entryID = res.Primary.ID
		}
		if res, ok := resources["incident_catalog_type_attribute.example_description"]; ok {
			descriptionAttrID = res.Primary.ID
		}

		return nil
	}

	// Verify entry still exists but has cleared Description attribute
	checkEntryExistsWithClearedAttribute := func(s *terraform.State) error {
		if entryID == "" {
			return fmt.Errorf("entry ID not captured during test")
		}

		// Fetch the entry directly from the API
		entry, err := testClient.CatalogV3ShowEntryWithResponse(context.Background(), entryID, nil)
		if err != nil {
			return fmt.Errorf("error fetching catalog entry: %w", err)
		}

		// Entry should still exist
		if entry.JSON200 == nil {
			return fmt.Errorf("expected entry to still exist after destroy with managed_attributes, but it was deleted")
		}

		// The managed attribute (Description) should be cleared
		if descriptionAttrID != "" {
			if binding, ok := entry.JSON200.CatalogEntry.AttributeValues[descriptionAttrID]; ok {
				// If the attribute exists, its value should be nil/empty
				if binding.Value != nil && binding.Value.Literal != nil && *binding.Value.Literal != "" {
					return fmt.Errorf("expected Description attribute to be cleared, but got: %q", *binding.Value.Literal)
				}
			}
			// If attribute doesn't exist at all, that's fine - it's been cleared
		}

		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create entry with managed_attributes set
			{
				Config: testAccIncidentCatalogEntryResourceConfigWithID(testID, "ManagedDestroy", "This should be cleared on destroy", []string{}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entry.example", "name", "ManagedDestroy"),
					captureIDs,
				),
			},
			// Step 2: Remove the entry from config (keep catalog type)
			// This triggers destroy of the entry, which should clear attributes not delete
			{
				Config: testAccIncidentCatalogEntryResourceConfigTypeOnly(testID),
				Check:  checkEntryExistsWithClearedAttribute,
			},
		},
	})
}

func TestIncidentCatalogEntryResource_ValidateConfigConditionalArray(t *testing.T) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Test validation error when attribute isn't in managed_attributes
			{
				Config: `
resource "incident_catalog_entry" "test" {
  for_each = {
    for name, value in [
      {
        name       = "foo"
        test_value = "ABC"
      },
      {
        name       = "bar"
        test_value = null
      },
    ] : value.name => value
  }

  catalog_type_id = "catalog-type-id-123"
  name = "Test Entry"

  attribute_values = each.value.test_value == null ? [] : [
    {
      attribute = "test",
      value     = each.value.test_value
    },
  ]
}
`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

var catalogEntryTemplate = template.Must(template.New("incident_catalog_entry").Funcs(testTemplateFuncs()).Parse(`
resource "incident_catalog_type" "example" {
  name        = "Catalog Entry Acceptance Test ({{ .ID }})"
  description = "Used in terraform acceptance tests for incident_catalog_entry"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "example_description" {
  catalog_type_id = incident_catalog_type.example.id

  name = "Description"
  type = "Text"
}

resource "incident_catalog_type_attribute" "example_bool" {
  catalog_type_id = incident_catalog_type.example.id

  name = "Useful"
  type = "Bool"
}

{{ if .IncludeEntry }}
resource "incident_catalog_entry" "example" {
  catalog_type_id = incident_catalog_type.example.id

  name    = {{ quote .Name }}
  aliases = {{ toJson .Aliases }}

  attribute_values = {{ if .UseEmptyManagedAttributes }}[]{{ else }}[
    {
      attribute = incident_catalog_type_attribute.example_description.id,
      value = {{ quote .Description }}
    },
    {{ if not .OnlyManageDescription }}{
      attribute = incident_catalog_type_attribute.example_bool.id,
      value = "true"
    }{{ end }}
  ]{{ end }}

  {{ if .OnlyManageDescription }}
  managed_attributes = [incident_catalog_type_attribute.example_description.id]
  {{ else if .UseEmptyManagedAttributes }}
  managed_attributes = []
  {{ end }}
}
{{ end }}
`))

func testAccIncidentCatalogEntryResourceConfigWithID(id, name, description string, aliases []string, onlyManageDescription bool) string {
	var buf bytes.Buffer
	if err := catalogEntryTemplate.Execute(&buf, struct {
		ID                        string
		Name                      string
		Description               string
		Aliases                   []string
		OnlyManageDescription     bool
		UseEmptyManagedAttributes bool
		IncludeEntry              bool
	}{
		ID:                        id,
		Name:                      name,
		Description:               description,
		Aliases:                   aliases,
		OnlyManageDescription:     onlyManageDescription,
		UseEmptyManagedAttributes: false,
		IncludeEntry:              true,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccIncidentCatalogEntryResourceConfigWithEmptyManagedAttributes(id, name, description string, aliases []string) string {
	var buf bytes.Buffer
	if err := catalogEntryTemplate.Execute(&buf, struct {
		ID                        string
		Name                      string
		Description               string
		Aliases                   []string
		OnlyManageDescription     bool
		UseEmptyManagedAttributes bool
		IncludeEntry              bool
	}{
		ID:                        id,
		Name:                      name,
		Description:               description,
		Aliases:                   aliases,
		OnlyManageDescription:     false,
		UseEmptyManagedAttributes: true,
		IncludeEntry:              true,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccIncidentCatalogEntryResourceConfigTypeOnly(id string) string {
	var buf bytes.Buffer
	if err := catalogEntryTemplate.Execute(&buf, struct {
		ID                        string
		Name                      string
		Description               string
		Aliases                   []string
		OnlyManageDescription     bool
		UseEmptyManagedAttributes bool
		IncludeEntry              bool
	}{
		ID:           id,
		IncludeEntry: false,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccIncidentCatalogEntryResourceConfig(name, description string, aliases []string) string {
	return testAccIncidentCatalogEntryResourceConfigWithID(uuid.NewString(), name, description, aliases, false)
}

// TestIncidentCatalogEntryResource_ValidateAdoptConfig covers the split between creating an
// entry and adopting one: an adopted entry's sync owns its name, aliases, external ID and
// rank, and only adopted entries take custom aliases.
func TestIncidentCatalogEntryResource_ValidateAdoptConfig(t *testing.T) {
	cases := []struct {
		name   string
		config string
		error  string
	}{
		{
			name: "adopting with a name",
			config: `
resource "incident_catalog_entry" "test" {
  catalog_type_id           = "catalog-type-id-123"
  existing_entry_identifier = "primary"
  name                      = "Primary"
}`,
			error: `name can't be set with existing_entry_identifier`,
		},
		{
			name: "adopting with aliases",
			config: `
resource "incident_catalog_entry" "test" {
  catalog_type_id           = "catalog-type-id-123"
  existing_entry_identifier = "primary"
  aliases                   = ["primary"]
}`,
			error: `aliases can't be set with existing_entry_identifier`,
		},
		{
			name: "adopting with an unmanaged attribute",
			config: `
resource "incident_catalog_entry" "test" {
  catalog_type_id           = "catalog-type-id-123"
  existing_entry_identifier = "primary"
  attribute_values = [{
    attribute = "attribute-id-123"
    value     = "A value"
  }]
}`,
			error: `Unmanaged Attribute`,
		},
		{
			name: "custom aliases without adopting",
			config: `
resource "incident_catalog_entry" "test" {
  catalog_type_id  = "catalog-type-id-123"
  name             = "Test Entry"
  custom_aliases   = ["test"]
  attribute_values = []
}`,
			error: `custom_aliases can only be set with existing_entry_identifier`,
		},
		{
			name: "creating without a name",
			config: `
resource "incident_catalog_entry" "test" {
  catalog_type_id  = "catalog-type-id-123"
  attribute_values = []
}`,
			error: `name is required, unless existing_entry_identifier is set`,
		},
		{
			name: "creating without attribute values",
			config: `
resource "incident_catalog_entry" "test" {
  catalog_type_id = "catalog-type-id-123"
  name            = "Test Entry"
}`,
			error: `attribute_values is required, unless existing_entry_identifier is set`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      tc.config,
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(tc.error),
				}},
			})
		})
	}

	t.Run("adopting with custom aliases and managed attributes", func(t *testing.T) {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{{
				Config: `
resource "incident_catalog_entry" "test" {
  catalog_type_id           = "catalog-type-id-123"
  existing_entry_identifier = "primary"
  custom_aliases            = ["primary-on-call"]
  managed_attributes        = ["attribute-id-123"]
  attribute_values = [{
    attribute = "attribute-id-123"
    value     = "A value"
  }]
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			}},
		})
	})
}

// TestIncidentCatalogEntryResource_ModifyPlanAdoptedAliases checks that aliases, which list
// an adopted entry's custom aliases too, are only planned as unknown when Terraform is about
// to change those custom aliases. Otherwise they keep the value in state.
func TestIncidentCatalogEntryResource_ModifyPlanAdoptedAliases(t *testing.T) {
	var schemaResp frameworkresource.SchemaResponse
	NewIncidentCatalogEntryResource().Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}
	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}

	stringSet := func(values ...string) tftypes.Value {
		if values == nil {
			return tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, nil)
		}
		elements := lo.Map(values, func(value string, _ int) tftypes.Value {
			return tftypes.NewValue(tftypes.String, value)
		})
		return tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, elements)
	}
	entry := func(identifier *string, customAliases tftypes.Value) tftypes.Value {
		values := map[string]tftypes.Value{}
		for name, attrType := range objType.AttributeTypes {
			values[name] = tftypes.NewValue(attrType, nil)
		}
		values["id"] = tftypes.NewValue(tftypes.String, "01ENTRY")
		values["catalog_type_id"] = tftypes.NewValue(tftypes.String, "01TYPE")
		values["name"] = tftypes.NewValue(tftypes.String, "Primary")
		values["aliases"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "synced"),
		})
		if identifier != nil {
			values["existing_entry_identifier"] = tftypes.NewValue(tftypes.String, *identifier)
		}
		values["custom_aliases"] = customAliases

		return tftypes.NewValue(objType, values)
	}

	cases := []struct {
		name           string
		identifier     *string
		prior, planned tftypes.Value
		wantUnknown    bool
	}{
		{
			name:        "adopted, custom aliases change",
			identifier:  lo.ToPtr("primary"),
			prior:       stringSet("one"),
			planned:     stringSet("two"),
			wantUnknown: true,
		},
		{
			name:       "adopted, custom aliases unchanged",
			identifier: lo.ToPtr("primary"),
			prior:      stringSet("one"),
			planned:    stringSet("one"),
		},
		{
			name:       "adopted, custom aliases left out",
			identifier: lo.ToPtr("primary"),
			prior:      stringSet(),
			planned:    stringSet(),
		},
		{
			name:    "not adopted",
			prior:   stringSet(),
			planned: stringSet(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := tfsdk.State{Schema: schemaResp.Schema, Raw: entry(tc.identifier, tc.prior)}
			plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: entry(tc.identifier, tc.planned)}
			resp := frameworkresource.ModifyPlanResponse{Plan: plan}

			r := &IncidentCatalogEntryResource{}
			r.ModifyPlan(context.Background(), frameworkresource.ModifyPlanRequest{State: state, Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %+v", resp.Diagnostics)
			}

			var aliases types.List
			resp.Plan.GetAttribute(context.Background(), path.Root("aliases"), &aliases)
			if aliases.IsUnknown() != tc.wantUnknown {
				t.Fatalf("expected aliases unknown=%v, got %s", tc.wantUnknown, aliases)
			}
		})
	}
}

// TestAccIncidentCatalogEntryResourceAdoptSchedule adopts the catalog entry incident.io syncs
// for a schedule, owns its custom aliases, then lets go of it. The entry has to outlive the
// resource, with only the custom aliases cleared.
func TestAccIncidentCatalogEntryResourceAdoptSchedule(t *testing.T) {
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
	// than racing it: adopting a missing entry is meant to fail.
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
				Config: testAccIncidentCatalogEntryAdoptConfig(nil),
				Check:  captureScheduleID,
			},
			{
				PreConfig: waitForScheduleEntry,
				Config:    testAccIncidentCatalogEntryAdoptConfig([]string{"tf-acc-one-" + suffix, "tf-acc-two-" + suffix}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("incident_catalog_entry.test", "external_id", "incident_schedule.test", "id"),
					resource.TestCheckResourceAttrPair("incident_catalog_entry.test", "name", "incident_schedule.test", "name"),
					resource.TestCheckResourceAttr("incident_catalog_entry.test", "custom_aliases.#", "2"),
					resource.TestCheckTypeSetElemAttr("incident_catalog_entry.test", "aliases.*", "tf-acc-one-"+suffix),
					resource.TestCheckTypeSetElemAttr("incident_catalog_entry.test", "aliases.*", "tf-acc-two-"+suffix),
				),
			},
			{
				Config: testAccIncidentCatalogEntryAdoptConfig([]string{"tf-acc-three-" + suffix}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("incident_catalog_entry.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("incident_catalog_entry.test", tfjsonpath.New("custom_aliases"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("tf-acc-three-" + suffix)})),
				},
			},
			// Removing the resource keeps the schedule, so we can check its entry survived.
			{
				Config: testAccIncidentCatalogEntryAdoptConfig(nil),
				Check:  checkEntryReleased,
			},
		},
	})
}

func TestAccIncidentCatalogEntryResourceAdoptMissingEntry(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

resource "incident_catalog_entry" "test" {
  catalog_type_id           = data.incident_catalog_type.schedule.id
  existing_entry_identifier = "` + uuid.NewString() + `"
  custom_aliases            = ["never-written"]
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

// testAccIncidentCatalogEntryAdoptConfig adopts the test schedule's catalog entry with the
// given custom aliases, or leaves the entry alone when there are none.
func testAccIncidentCatalogEntryAdoptConfig(customAliases []string) string {
	return testRunTemplate("incident_catalog_entry_adopt", `
data "incident_catalog_type" "schedule" {
  type_name = "Schedule"
}

resource "incident_schedule" "test" {
  name     = {{ stableSuffix "acc-catalog-entry-adopt" | quote }}
  timezone = "Europe/London"
}
{{ if .CustomAliases }}
resource "incident_catalog_entry" "test" {
  catalog_type_id           = data.incident_catalog_type.schedule.id
  existing_entry_identifier = incident_schedule.test.id
  custom_aliases            = {{ toJson .CustomAliases }}
}
{{ end }}`, map[string]any{"CustomAliases": customAliases})
}
