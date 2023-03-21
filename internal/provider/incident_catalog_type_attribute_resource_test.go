package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentCatalogTypeAttributeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogTypeAttributeResourceConfig(client.CatalogTypeAttributeV2{
					Name: "Name",
					Type: "Text",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "name", "Name"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "type", "Text"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "array", "false"),
				),
			},
			// Update and read
			{
				Config: testAccIncidentCatalogTypeAttributeResourceConfig(client.CatalogTypeAttributeV2{
					Name:  "Description",
					Type:  "Text",
					Array: true,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "name", "Description"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "type", "Text"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "array", "true"),
				),
			},
		},
	})
}

var catalogTypeAttributeTemplate = template.Must(template.New("incident_catalog_type_attribute").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = "Example ({{ .ID }})"
  description = "Used in terraform acceptance tests"
}

resource "incident_catalog_type_attribute" "example" {
  catalog_type_id = incident_catalog_type.example.id

  name = {{ quote .Attribute.Name }}
  type = {{ quote .Attribute.Type }}
  {{ if .Attribute.Array }}
  array = true
  {{ end }}
}
`))

func testAccIncidentCatalogTypeAttributeResourceConfig(attribute client.CatalogTypeAttributeV2) string {
	var buf bytes.Buffer
	if err := catalogTypeAttributeTemplate.Execute(&buf, struct {
		ID        string
		Attribute client.CatalogTypeAttributeV2
	}{
		ID:        uuid.NewString(),
		Attribute: attribute,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}
