package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentCatalogTypeDataSource(t *testing.T) {
	typeName := generateTypeName()

	// Searching by name
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogTypeDataSourceConfig(catalogTypeDataSourceFixture{
					ResourceName:        catalogTypeDefault().Name,
					ResourceTypeName:    typeName,
					ResourceDescription: catalogTypeDefault().Description,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "name", catalogTypeDefault().Name),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_type.by_name", "name", catalogTypeDefault().Name),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_type.by_type_name", "type_name", typeName),
				),
			},
		},
	})
}

var catalogTypeDataSourceTemplate = template.Must(template.New("incident_catalog_type_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = {{ quote .ResourceName }}
  type_name    = {{ quote .ResourceTypeName }}
  description = {{ quote .ResourceDescription }}

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}
data "incident_catalog_type" "by_name" {
  name = incident_catalog_type.example.name
}

data "incident_catalog_type" "by_type_name" {
  type_name = incident_catalog_type.example.type_name
}
`))

type catalogTypeDataSourceFixture struct {
	ResourceName        string
	ResourceTypeName    string
	ResourceDescription string
}

func testAccIncidentCatalogTypeDataSourceConfig(payload catalogTypeDataSourceFixture) string {
	var buf bytes.Buffer
	if err := catalogTypeDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(err)
	}

	return buf.String()
}
