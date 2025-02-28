package provider

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/samber/lo"
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

var catalogEntryTemplate = template.Must(template.New("incident_catalog_entry").Funcs(sprig.TxtFuncMap()).Parse(`
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

resource "incident_catalog_entry" "example" {
  catalog_type_id = incident_catalog_type.example.id

  name    = {{ quote .Name }}
  aliases = {{ toJson .Aliases }}

  attribute_values = [
    {
      attribute = incident_catalog_type_attribute.example_description.id,
      value = {{ quote .Description }}
    },
    {{ if not .OnlyManageDescription }}
    {
      attribute = incident_catalog_type_attribute.example_bool.id,
      value = "true"
    }
    {{ end }}
  ]

  {{ if .OnlyManageDescription }}
  managed_attributes = [incident_catalog_type_attribute.example_description.id]
  {{ end }}
}
`))

func testAccIncidentCatalogEntryResourceConfigWithID(id, name, description string, aliases []string, onlyManageDescription bool) string {
	var buf bytes.Buffer
	if err := catalogEntryTemplate.Execute(&buf, struct {
		ID                    string
		Name                  string
		Description           string
		Aliases               []string
		OnlyManageDescription bool
	}{
		ID:                    id,
		Name:                  name,
		Description:           description,
		Aliases:               aliases,
		OnlyManageDescription: onlyManageDescription,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccIncidentCatalogEntryResourceConfig(name, description string, aliases []string) string {
	return testAccIncidentCatalogEntryResourceConfigWithID(uuid.NewString(), name, description, aliases, false)
}
