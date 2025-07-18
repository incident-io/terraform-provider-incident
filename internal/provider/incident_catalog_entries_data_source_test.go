package provider

import (
	"bytes"
	"fmt"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentCatalogEntriesDataSource(t *testing.T) {
	typeName := generateTypeName()

	// Test basic listing
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogEntriesDataSourceConfig(catalogEntriesDataSourceFixture{
					TypeName: typeName,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check that we have exactly 3 catalog entries
					resource.TestCheckResourceAttr("data.incident_catalog_entries.test", "catalog_entries.#", "3"),
					// Check the catalog type ID is set
					resource.TestCheckResourceAttrSet("data.incident_catalog_entries.test", "catalog_type_id"),
					// Check that our test entries exist in the results
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_catalog_entries.test", "catalog_entries.*", map[string]string{
						"name":        "Test Entry 1",
						"external_id": "test-entry-1",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_catalog_entries.test", "catalog_entries.*", map[string]string{
						"name":        "Test Entry 2",
						"external_id": "test-entry-2",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_catalog_entries.test", "catalog_entries.*", map[string]string{
						"name":        "Test Entry 3",
						"external_id": "test-entry-3",
					}),
				),
			},
		},
	})
}

func TestAccIncidentCatalogEntriesDataSource_WithAliases(t *testing.T) {
	typeName := generateTypeName()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogEntriesDataSourceConfigWithAliases(catalogEntriesDataSourceFixture{
					TypeName: typeName,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check that we have the entry with aliases
					resource.TestCheckResourceAttr("data.incident_catalog_entries.test", "catalog_entries.#", "1"),
					// Check aliases are present
					resource.TestCheckTypeSetElemAttr("data.incident_catalog_entries.test", "catalog_entries.0.aliases.*", "alias-1"),
					resource.TestCheckTypeSetElemAttr("data.incident_catalog_entries.test", "catalog_entries.0.aliases.*", "alias-2"),
					resource.TestCheckTypeSetElemAttr("data.incident_catalog_entries.test", "catalog_entries.0.aliases.*", "alias-3"),
				),
			},
		},
	})
}

func TestAccIncidentCatalogEntriesDataSource_Empty(t *testing.T) {
	typeName := generateTypeName()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCatalogEntriesDataSourceConfigEmpty(catalogEntriesDataSourceFixture{
					TypeName: typeName,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check that we have no catalog entries
					resource.TestCheckResourceAttr("data.incident_catalog_entries.test", "catalog_entries.#", "0"),
					// Check the catalog type ID is set
					resource.TestCheckResourceAttrSet("data.incident_catalog_entries.test", "catalog_type_id"),
				),
			},
		},
	})
}

var catalogEntriesDataSourceTemplate = template.Must(template.New("incident_catalog_entries_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "test" {
  name        = "Test Catalog Type"
  type_name   = {{ quote .TypeName }}
  description = "Used in terraform acceptance tests"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_entry" "test1" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Test Entry 1"
  external_id     = "test-entry-1"
  aliases         = []
  attribute_values = []
}

resource "incident_catalog_entry" "test2" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Test Entry 2"
  external_id     = "test-entry-2"
  aliases         = []
  attribute_values = []
}

resource "incident_catalog_entry" "test3" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Test Entry 3"
  external_id     = "test-entry-3"
  aliases         = []
  attribute_values = []
}

data "incident_catalog_entries" "test" {
  catalog_type_id = incident_catalog_type.test.id
  
  depends_on = [
    incident_catalog_entry.test1,
    incident_catalog_entry.test2,
    incident_catalog_entry.test3
  ]
}
`))

var catalogEntriesDataSourceTemplateWithAliases = template.Must(template.New("incident_catalog_entries_data_source_aliases").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "test" {
  name        = "Test Catalog Type With Aliases"
  type_name   = {{ quote .TypeName }}
  description = "Used in terraform acceptance tests"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_entry" "test_with_aliases" {
  catalog_type_id = incident_catalog_type.test.id
  name            = "Test Entry With Aliases"
  external_id     = "test-entry-aliases"
  aliases         = ["alias-1", "alias-2", "alias-3"]
  attribute_values = []
}

data "incident_catalog_entries" "test" {
  catalog_type_id = incident_catalog_type.test.id
  
  depends_on = [incident_catalog_entry.test_with_aliases]
}
`))

var catalogEntriesDataSourceTemplateEmpty = template.Must(template.New("incident_catalog_entries_data_source_empty").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "test" {
  name        = "Empty Catalog Type"
  type_name   = {{ quote .TypeName }}
  description = "Used in terraform acceptance tests"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

data "incident_catalog_entries" "test" {
  catalog_type_id = incident_catalog_type.test.id
}
`))

type catalogEntriesDataSourceFixture struct {
	TypeName string
}

func testAccIncidentCatalogEntriesDataSourceConfig(payload catalogEntriesDataSourceFixture) string {
	var buf bytes.Buffer
	if err := catalogEntriesDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(fmt.Errorf("failed to execute template: %w", err))
	}
	return buf.String()
}

func testAccIncidentCatalogEntriesDataSourceConfigWithAliases(payload catalogEntriesDataSourceFixture) string {
	var buf bytes.Buffer
	if err := catalogEntriesDataSourceTemplateWithAliases.Execute(&buf, payload); err != nil {
		panic(fmt.Errorf("failed to execute template: %w", err))
	}
	return buf.String()
}

func testAccIncidentCatalogEntriesDataSourceConfigEmpty(payload catalogEntriesDataSourceFixture) string {
	var buf bytes.Buffer
	if err := catalogEntriesDataSourceTemplateEmpty.Execute(&buf, payload); err != nil {
		panic(fmt.Errorf("failed to execute template: %w", err))
	}
	return buf.String()
}
