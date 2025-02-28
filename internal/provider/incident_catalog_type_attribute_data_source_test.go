package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentCatalogTypeAttributeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogTypeAttributeDataSourceConfig(client.CatalogTypeAttributeV2{
					Name: "Test Attribute",
					Type: "Text",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "name", "Test Attribute"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "type", "Text"),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_type_attribute.by_name", "name", "Test Attribute"),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_type_attribute.by_name", "type", "Text"),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_type_attribute.by_name", "array", "false"),
				),
			},
		},
	})
}

var catalogTypeAttributeDataSourceTemplate = template.Must(template.New("incident_catalog_type_attribute_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = "Example ({{ .ID }})"
  description = "Used in terraform acceptance tests"
  type_name   = {{ quote .TypeName }}

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

data "incident_catalog_type_attribute" "by_name" {
  catalog_type_id = incident_catalog_type.example.id
  name = incident_catalog_type_attribute.example.name
}
`))

func testAccIncidentCatalogTypeAttributeDataSourceConfig(attribute client.CatalogTypeAttributeV2) string {
	var buf bytes.Buffer
	if err := catalogTypeAttributeDataSourceTemplate.Execute(&buf, struct {
		ID        string
		TypeName  string
		Attribute client.CatalogTypeAttributeV2
	}{
		ID:        testRunID,
		TypeName:  generateTypeName(),
		Attribute: attribute,
	}); err != nil {
		panic(err)
	}

	return buf.String()
}
