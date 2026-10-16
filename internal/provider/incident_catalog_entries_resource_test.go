package provider

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"text/template"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/samber/lo"
)

func TestAccIncidentCatalogEntriesResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read. UsefulValue must be non-empty: value = "" means "clear this
			// attribute", and buildModel round-trips that empty string from the plan into
			// state. Import has no plan, so ImportStateVerify would fail if we left the
			// zero-value "" here (the template always emits the Useful attribute).
			{
				Config: testAccIncidentCatalogEntriesResourceConfig([]catalogEntryElement{
					{
						Name:        "One",
						ExternalID:  "one",
						Description: "This is the first entry",
						ArrayValue:  "null",
						UsefulValue: "true",
					},
					{
						Name:        "Two",
						ExternalID:  "two",
						Description: "This is the second entry",
						ArrayValue:  "[]",
						UsefulValue: "true",
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
						UsefulValue: "true",
					},
					{
						Name:        "Three",
						ExternalID:  "two",
						Description: "This is the third entry",
						ArrayValue:  "[]",
						UsefulValue: "true",
					},
				}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entries.example", "entries.two.name", "Three"),
				),
			},
			// Add an entry to an existing resource. The new key has no prior state, so
			// its Computed id must plan as unknown rather than null: see
			// UseNonNullStateForUnknown. Named four, as the step above renames two to
			// "Three".
			{
				Config: testAccIncidentCatalogEntriesResourceConfig([]catalogEntryElement{
					{
						Name:        "One",
						ExternalID:  "one",
						Description: "This is the first entry",
						ArrayValue:  "null",
						UsefulValue: "true",
					},
					{
						Name:        "Three",
						ExternalID:  "two",
						Description: "This is the third entry",
						ArrayValue:  "[]",
						UsefulValue: "true",
					},
					{
						Name:        "Four",
						ExternalID:  "four",
						Description: "This entry is added after the resource exists",
						Aliases:     []string{"quatre"},
						ArrayValue:  "[]",
						UsefulValue: "true",
					},
				}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entries.example", "entries.four.name", "Four"),
					resource.TestCheckResourceAttrSet(
						"incident_catalog_entries.example", "entries.four.id"),
					resource.TestCheckResourceAttr(
						"incident_catalog_entries.example", "entries.four.aliases.0", "quatre"),
				),
			},
		},
	})
}

// TestAccIncidentCatalogEntriesResourceEmptyStringClearsAttribute is the regression test for
// Pylon 52773 / PR-115: value = "" is documented as the way to clear a scalar attribute, but
// the catalog API doesn't accept a literal empty-string write - it silently drops it and
// keeps the previous value. Without the fix in buildPayloads/buildModel/reconcile, clearing
// an attribute this way fails apply with "Provider produced inconsistent result after
// apply". managed_attributes is deliberately left unset here, which exercises the fallback
// that has to make update_attributes explicit for an entry once it clears anything.
func TestAccIncidentCatalogEntriesResourceEmptyStringClearsAttribute(t *testing.T) {
	testCatalogID := uuid.NewString()

	var entryID, descriptionAttrID string
	captureIDs := func(s *terraform.State) error {
		resources := s.RootModule().Resources
		if res, ok := resources["incident_catalog_entries.example"]; ok {
			entryID = res.Primary.Attributes["entries.one.id"]
		}
		if res, ok := resources["incident_catalog_type_attribute.example_description"]; ok {
			descriptionAttrID = res.Primary.ID
		}
		if entryID == "" || descriptionAttrID == "" {
			return fmt.Errorf("entry or attribute ID not found in state")
		}

		return nil
	}

	// Sets Description behind Terraform's back, as someone editing in the dashboard would.
	setDescriptionOutOfBand := func() {
		_, err := testClient.CatalogV3UpdateEntryWithResponse(context.Background(), entryID, client.CatalogUpdateEntryPayloadV3{
			Name: "One",
			AttributeValues: map[string]client.CatalogEngineParamBindingPayloadV3{
				descriptionAttrID: {Value: &client.CatalogEngineParamBindingValuePayloadV3{Literal: lo.ToPtr("set in the dashboard")}},
			},
			UpdateAttributes: &[]string{descriptionAttrID},
		})
		if err != nil {
			t.Fatalf("setting Description out of band: %s", err)
		}
	}

	checkDescriptionCleared := func(s *terraform.State) error {
		resources := s.RootModule().Resources

		catalogTypeRes, ok := resources["incident_catalog_type.example"]
		if !ok {
			return fmt.Errorf("incident_catalog_type.example not found in state")
		}
		descriptionAttrRes, ok := resources["incident_catalog_type_attribute.example_description"]
		if !ok {
			return fmt.Errorf("incident_catalog_type_attribute.example_description not found in state")
		}

		entries, err := testClient.CatalogV3ListEntriesWithResponse(context.Background(), &client.CatalogV3ListEntriesParams{
			CatalogTypeId: catalogTypeRes.Primary.ID,
			PageSize:      250,
		})
		if err != nil {
			return fmt.Errorf("error fetching catalog entries: %w", err)
		}

		entry, found := lo.Find(entries.JSON200.CatalogEntries, func(e client.CatalogEntryV3) bool {
			return e.ExternalId != nil && *e.ExternalId == "one"
		})
		if !found {
			return fmt.Errorf("entry with external_id 'one' not found in API response")
		}

		// An attribute cleared via value = "" either disappears from the response
		// entirely, or comes back with a nil/empty literal - either is a pass. A
		// non-empty literal means the clear never reached the API, and the Useful
		// attribute (never cleared) should be untouched by the fallback that makes
		// update_attributes explicit once something is cleared.
		if binding, ok := entry.AttributeValues[descriptionAttrRes.Primary.ID]; ok {
			if binding.Value != nil && binding.Value.Literal != nil && *binding.Value.Literal != "" {
				return fmt.Errorf("expected Description attribute to be cleared by value = \"\", but the API still has %q", *binding.Value.Literal)
			}
		}

		return nil
	}

	entryWithDescription := func(description string) []catalogEntryElement {
		return []catalogEntryElement{
			{
				Name:        "One",
				ExternalID:  "one",
				Description: description,
				ArrayValue:  "null",
				UsefulValue: "true",
			},
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with a real Description value.
			{
				Config: testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, entryWithDescription("a real description"), false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_catalog_entries.example", "entries.one.name", "One"),
				),
			},
			// Clear it with value = "". This is the Pylon 52773 shape: without the fix the
			// API silently keeps the old value and apply fails with "inconsistent result".
			{
				Config: testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, entryWithDescription(""), false),
				Check:  resource.ComposeAggregateTestCheckFunc(checkDescriptionCleared, captureIDs),
			},
			// Re-reading should be a no-op: Read's buildModel also has to round-trip the
			// empty string we planned, or this would be a perpetual diff.
			{
				Config:   testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, entryWithDescription(""), false),
				PlanOnly: true,
			},
			// A value set out of band after the clear is drift for the next apply to fix,
			// not a refresh error.
			{
				PreConfig:          setDescriptionOutOfBand,
				Config:             testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, entryWithDescription(""), false),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, entryWithDescription(""), false),
				Check:  checkDescriptionCleared,
			},
			// Setting a value again after a clear: the API response no longer has the key,
			// so reconcile has to notice the payload adds it rather than skip the entry.
			{
				Config: testAccIncidentCatalogEntriesResourceConfigWithID(testCatalogID, entryWithDescription("set again"), false),
				Check: func(s *terraform.State) error {
					return resource.TestCheckResourceAttr("incident_catalog_entries.example",
						fmt.Sprintf("entries.one.attribute_values.%s.value", descriptionAttrID), "set again")(s)
				},
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

var catalogEntriesTemplate = template.Must(template.New("incident_catalog_entries").Funcs(testTemplateFuncs()).Parse(`
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
          value = {{ quote .UsefulValue }}
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
