package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentCatalogEntryDataSource(t *testing.T) {
	typeName := generateTypeName()

	// Lookup by external_id
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogEntryDataSourceConfig(catalogEntryDataSourceFixture{
					TypeName:   typeName,
					EntryName:  "Test Entry External",
					ExternalID: "test-external-id",
					Alias:      "test-external-alias",
					Identifier: "test-external-id",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry.example", "external_id", "test-external-id"),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_entry.example", "name", "Test Entry External"),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_entry.example", "external_id", "test-external-id"),
				),
			},
		},
	})

	// Lookup by alias
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogEntryDataSourceConfig(catalogEntryDataSourceFixture{
					TypeName:   typeName,
					EntryName:  "Test Entry Alias",
					ExternalID: "test-alias-id",
					Alias:      "test-lookup-alias",
					Identifier: "test-lookup-alias",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry.example", "name", "Test Entry Alias"),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_entry.example", "name", "Test Entry Alias"),
					resource.TestCheckResourceAttr(
						"data.incident_catalog_entry.example", "external_id", "test-alias-id"),
				),
			},
		},
	})
}

var catalogEntryDataSourceTemplate = template.Must(template.New("incident_catalog_entry_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = "Catalog Type For Entry Test"
  type_name   = {{ quote .TypeName }}
  description = "Used in terraform acceptance tests"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_entry" "example" {
  catalog_type_id = incident_catalog_type.example.id
  name = {{ quote .EntryName }}
  external_id = {{ quote .ExternalID }}
  aliases = [{{ quote .Alias }}]

  attribute_values = []
}

data "incident_catalog_entry" "example" {
  catalog_type_id = incident_catalog_type.example.id
  identifier = {{ quote .Identifier }}

  depends_on = [incident_catalog_entry.example]
}
`))

type catalogEntryDataSourceFixture struct {
	TypeName   string
	EntryName  string
	ExternalID string
	Alias      string
	Identifier string
}

func testAccIncidentCatalogEntryDataSourceConfig(payload catalogEntryDataSourceFixture) string {
	var buf bytes.Buffer
	if err := catalogEntryDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(err)
	}

	return buf.String()
}
