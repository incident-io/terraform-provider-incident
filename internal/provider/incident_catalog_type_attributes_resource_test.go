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

func TestAccIncidentCatalogTypeAttributesResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogTypeAttributesResourceConfig([]client.CatalogTypeAttributeV2{
					{
						Name: "Name",
						Type: "Text",
					},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attributes.example", "attributes.0.name", "Name"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attributes.example", "attributes.0.type", "Text"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attributes.example", "attributes.0.array", "false"),
				),
			},
			// Import
			{
				ResourceName:      "incident_catalog_type_attributes.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCatalogTypeAttributesResourceConfig([]client.CatalogTypeAttributeV2{
					{
						Name: "Name",
						Type: "Text",
					},
					{
						Name: "Description",
						Type: "Text",
					},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attributes.example", "attributes.1.name", "Description"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attributes.example", "attributes.1.type", "Text"),
				),
			},
		},
	})
}

var catalogTypeAttributesTemplate = template.Must(template.New("incident_catalog_type_attributes").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = "Example ({{ .ID }})"
  description = "Used in terraform acceptance tests"
}

resource "incident_catalog_type_attributes" "example" {
  catalog_type_id = incident_catalog_type.example.id

  attributes = [
    {{ range .Attributes }}
    {
      name = {{ quote .Name }},
      type = {{ quote .Type }},
      {{ if .Array }}
      array = true,
      {{ end }}
    },
    {{ end }}
  ]
}
`))

func testAccIncidentCatalogTypeAttributesResourceConfig(attributes []client.CatalogTypeAttributeV2) string {
	var buf bytes.Buffer
	if err := catalogTypeAttributesTemplate.Execute(&buf, struct {
		ID         string
		Attributes []client.CatalogTypeAttributeV2
	}{
		ID:         uuid.NewString(),
		Attributes: attributes,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}
