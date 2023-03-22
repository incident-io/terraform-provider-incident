package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentCatalogEntryResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogEntryResourceConfig("One", "This is the first entry"),
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
				Config: testAccIncidentCatalogEntryResourceConfig("Two", "This is the second entry"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry.example", "name", "Two"),
				),
			},
		},
	})
}

var catalogEntryTemplate = template.Must(template.New("incident_catalog_entry").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = "Catalog Entry Acceptance Test ({{ .ID }})"
  description = "Used in terraform acceptance tests for incident_catalog_entry"
}

resource "incident_catalog_type_attribute" "example_description" {
  catalog_type_id = incident_catalog_type.example.id

  name = "Description"
  type = "Text"
}

resource "incident_catalog_entry" "example" {
  catalog_type_id = incident_catalog_type.example.id

  name = {{ quote .Name }}
	attribute_values = [
		{
			attribute = incident_catalog_type_attribute.example_description.id,
			value = {{ quote .Description }}
		}
	]
}
`))

func testAccIncidentCatalogEntryResourceConfig(name, description string) string {
	var buf bytes.Buffer
	if err := catalogEntryTemplate.Execute(&buf, struct {
		ID          string
		Name        string
		Description string
	}{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}
