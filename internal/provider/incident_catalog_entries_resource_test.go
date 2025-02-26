package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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
				}),
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
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entries.example", "entries.two.name", "Three"),
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

resource "incident_catalog_entries" "example" {
  id = incident_catalog_type.example.id

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
}

func testAccIncidentCatalogEntriesResourceConfig(entries []catalogEntryElement) string {
	var buf bytes.Buffer
	if err := catalogEntriesTemplate.Execute(&buf, struct {
		ID      string
		Entries []catalogEntryElement
	}{
		ID:      uuid.NewString(),
		Entries: entries,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}
