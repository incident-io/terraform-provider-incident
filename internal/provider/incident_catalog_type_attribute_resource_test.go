package provider

import (
	"bytes"
	"fmt"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
					Type:  "String",
					Array: true,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "name", "Description"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "type", "String"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "array", "true"),
				),
			},
			// Schema-only
			{
				Config: testAccIncidentCatalogTypeAttributeResourceConfig(client.CatalogTypeAttributeV2{
					Name: "Description",
					Type: "String",
					Mode: "dashboard",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "name", "Description"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "type", "String"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "schema_only", "true"),
				),
			},
			// Test importing the resource
			{
				ResourceName:      "incident_catalog_type_attribute.example",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccIncidentCatalogTypeAttributeImportStateIDFunc,
			},
		},
	})
}

// testAccIncidentCatalogTypeAttributeImportStateIDFunc generates the import ID
// in the format catalog_type_id:attribute_id for testing import.
func testAccIncidentCatalogTypeAttributeImportStateIDFunc(s *terraform.State) (string, error) {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "incident_catalog_type_attribute" {
			continue
		}

		catalogTypeID := rs.Primary.Attributes["catalog_type_id"]
		attributeID := rs.Primary.ID

		return fmt.Sprintf("%s:%s", catalogTypeID, attributeID), nil
	}

	return "", fmt.Errorf("Couldn't find catalog_type_attribute resource")
}

var catalogTypeAttributeTemplate = template.Must(template.New("incident_catalog_type_attribute").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = "Example ({{ .ID }})"
  description = "Used in terraform acceptance tests"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "example" {
  catalog_type_id = incident_catalog_type.example.id

  name = {{ quote .Attribute.Name }}
  type = {{ quote .Attribute.Type }}
  {{ if .Attribute.Array }}
  array = true
  {{ end }}
  {{ if eq .Attribute.Mode "dashboard" }}
  schema_only = true
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
