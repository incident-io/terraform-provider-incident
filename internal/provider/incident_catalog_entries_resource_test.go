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

func TestAccIncidentCatalogEntriesResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogEntriesResourceConfig([]catalogEntryElement{
					{
						Name:        "One",
						ExternalID:  "one",
						Description: "This is the first entry",
						ArrayValue:  "null",
					},
					{
						Name:        "Two",
						ExternalID:  "two",
						Description: "This is the second entry",
						ArrayValue:  "[]",
					},
				}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entries.example", "entries.one.name", "One"),
					resource.TestCheckResourceAttr(
						"incident_catalog_entries.example", "entries.two.name", "Two"),
				),
			},
			// Import
			{
				ResourceName:      "incident_catalog_entries.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCatalogEntriesResourceConfig([]catalogEntryElement{
					{
						Name:        "One",
						ExternalID:  "one",
						Description: "This is the first entry",
						ArrayValue:  "null",
					},
					{
						Name:        "Three",
						ExternalID:  "two",
						Description: "This is the third entry",
						ArrayValue:  "[]",
					},
				}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entries.example", "entries.two.name", "Three"),
				),
			},
		},
	})
}

func TestIncidentCatalogEntriesResource_ValidateConfig(t *testing.T) {
	description := "desc-123"
	priority := "priority-456"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Test validation error when attribute isn't in managed_attributes
			{
				Config: fmt.Sprintf(`
resource "incident_catalog_entries" "test" {
  id = "catalog-type-id-123"

  # Only manage the description attribute
  managed_attributes = ["%s"]

  entries = {
    "test-entry" = {
      name = "Test Entry"

      attribute_values = {
        # This is managed, should be fine
        "%s" = {
          value = "A description"
        }
        # This is not managed, should cause an error
        "%s" = {
          value = "High"
        }
      }
    }
  }
}
`, description, description, priority),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`not in the managed_attributes set`),
			},
		},
	})
}

func TestAccIncidentCatalogEntriesResourceWithManagedAttributes(t *testing.T) {
	// Use a stable ID across steps
	testCatalogID := uuid.NewString()

	// checkUsefulIsTrue verifies that:
	// 1. The Description attribute has the expected value
	// 2. The Useful attribute is still set to "true"
	// This validates that managed_attributes works correctly
	checkUsefulIsTrue := func(description string) func(s *terraform.State) error {
		return func(s *terraform.State) error {
			resources := s.RootModule().Resources

			// Verify required resources exist in state
			resourceIDs := map[string]string{}
			for _, name := range []string{
				"incident_catalog_type.example",
				"incident_catalog_entries.example",
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

			// Find our specific entry (use external_id "test")
			entry, found := lo.Find(entries.JSON200.CatalogEntries, func(e client.CatalogEntryV3) bool {
				return e.ExternalId != nil && *e.ExternalId == "test"
			})
			if !found {
				return fmt.Errorf("entry with external_id 'test' not found in API response")
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

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// First create with both attributes (Useful=true)
			{
				Config: testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, []catalogEntryElement{
					{
						Name:        "Initial Entry",
						ExternalID:  "test",
						Description: "Initial description",
						UsefulValue: "true",
					},
				}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entries.example", "entries.test.name", "Initial Entry"),
					checkUsefulIsTrue("Initial description"),
				),
			},
			// Now switch to managed_attributes for description only and update description
			// The Useful attribute should remain true from previous step
			{
				Config: testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, []catalogEntryElement{
					{
						Name:        "Partial Update",
						ExternalID:  "test",
						Description: "Updated description only",
						// UsefulValue omitted intentionally
					},
				}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entries.example", "entries.test.name", "Partial Update"),
					checkUsefulIsTrue("Updated description only"),
				),
			},
			// Update description again with managed_attributes
			// Useful should still remain unchanged from first step
			{
				Config: testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, []catalogEntryElement{
					{
						Name:        "Another Update",
						ExternalID:  "test",
						Description: "Another description change",
						// UsefulValue omitted intentionally
					},
				}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entries.example", "entries.test.name", "Another Update"),
					checkUsefulIsTrue("Another description change"),
				),
			},
		},
	})
}

var catalogEntriesTemplate = template.Must(template.New("incident_catalog_entries").Funcs(sprig.TxtFuncMap()).Parse(`
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

resource "incident_catalog_type_attribute" "example_array" {
  catalog_type_id = incident_catalog_type.example.id

  name  = "Array"
  type  = "String"
  array = true
}

resource "incident_catalog_type_attribute" "example_bool" {
  catalog_type_id = incident_catalog_type.example.id

  name = "Useful"
  type = "Bool"

  {{ if $.ManagedAttributes }}
  schema_only = true
  {{ end }}
}

resource "incident_catalog_entries" "example" {
  id = incident_catalog_type.example.id

  {{ if $.ManagedAttributes }}
  managed_attributes = [incident_catalog_type_attribute.example_description.id, incident_catalog_type_attribute.example_array.id]
  {{ end }}

  entries = {
  {{ range .Entries }}
    {{ quote .ExternalID }} = {
      name    = {{ quote .Name }}
      aliases = {{ toJson .Aliases }}

      attribute_values = {
        (incident_catalog_type_attribute.example_description.id) = {
          value = {{ quote .Description }}
        }
        (incident_catalog_type_attribute.example_array.id) = {
          array_value = {{ .ArrayValue }}
        }
        {{ if not $.ManagedAttributes }}
        (incident_catalog_type_attribute.example_bool.id) = {
          value = {{ .UsefulValue }}
        }
        {{ end }}
      }
    },
  {{ end }}
  }
}
`))

type catalogEntryElement struct {
	Name        string
	ExternalID  string
	Aliases     []string
	Description string
	ArrayValue  string
	UsefulValue string
}

func testAccIncidentCatalogEntriesResourceConfigWithID(catalogID string, entries []catalogEntryElement, managedAttributes bool) string {
	// Set default ArrayValue if not provided
	for i := range entries {
		if entries[i].ArrayValue == "" {
			entries[i].ArrayValue = "null"
		}
	}

	var buf bytes.Buffer
	if err := catalogEntriesTemplate.Execute(&buf, struct {
		ID                string
		Entries           []catalogEntryElement
		ManagedAttributes bool
	}{
		ID:                catalogID,
		Entries:           entries,
		ManagedAttributes: managedAttributes,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccIncidentCatalogEntriesResourceConfig(entries []catalogEntryElement, managedAttributes bool) string {
	return testAccIncidentCatalogEntriesResourceConfigWithID(uuid.NewString(), entries, managedAttributes)
}
