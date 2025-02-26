package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentCustomFieldResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read testing all fields
			{
				Config: testAccIncidentCustomFieldResourceConfig(customFieldTemplateParams{}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "name", "Affected teams"),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "description", "The teams that are affected by this incident"),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "field_type", "multi_select"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "filter_by"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "group_by_catalog_attribute_id"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "helptext_catalog_attribute_id"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "catalog_type_id"),
				),
			},
			// Import
			{
				ResourceName:      "incident_custom_field.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccIncidentCustomFieldResource_CatalogBacked(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read testing all fields
			{
				Config: testAccIncidentCustomFieldResourceConfig(customFieldTemplateParams{WithCatalogType: true}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "name", "Affected teams"),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "description", "The teams that are affected by this incident"),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "field_type", "multi_select"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "filter_by"),
					resource.TestCheckResourceAttrPair(
						"incident_custom_field.example", "group_by_catalog_attribute_id",
						"incident_catalog_type_attribute.example_string_attr", "id",
					),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "helptext_catalog_attribute_id"),
					resource.TestCheckResourceAttrSet(
						"incident_custom_field.example", "catalog_type_id"),
				),
			},
			// Add filtering
			{
				Config: testAccIncidentCustomFieldResourceConfig(customFieldTemplateParams{WithCatalogType: true, WithFilter: true}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"incident_custom_field.example", "filter_by.custom_field_id",
						"incident_custom_field.other", "id",
					),
					resource.TestCheckResourceAttrPair(
						"incident_custom_field.example", "filter_by.catalog_attribute_id",
						"incident_catalog_type_attribute.example_catalog_attr", "id",
					),
				),
			},
		},
	})
}

var customFieldTemplate = template.Must(template.New("incident_custom_field").Funcs(sprig.TxtFuncMap()).Parse(`
{{- if .WithCatalogType }}
resource "incident_catalog_type" "example" {
  name = "My type"
  description = "My type description"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "example_string_attr" {
  catalog_type_id = incident_catalog_type.example.id
  name = "My string attribute"
  type = "String"
}

resource "incident_catalog_type" "other" {
  name = "My other type"
  description = "My other type description"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "example_catalog_attr" {
  catalog_type_id = incident_catalog_type.example.id
  name = "My other attr"
  type = incident_catalog_type.other.type_name
}
{{- end }}

{{- if .WithFilter }}
resource "incident_custom_field" "other" {
  name = "Other field"
  description = "Other field description"

  field_type = "single_select"
  catalog_type_id = incident_catalog_type.other.id
}
{{- end }}

resource "incident_custom_field" "example" {
  name                          = "Affected teams"
  description                   = "The teams that are affected by this incident"
  field_type                     = "multi_select"

  {{- if .WithCatalogType }}
  catalog_type_id               = incident_catalog_type.example.id

  group_by_catalog_attribute_id  = incident_catalog_type_attribute.example_string_attr.id
  {{- end }}

  {{- if .WithFilter }}
  filter_by = {
    catalog_attribute_id = incident_catalog_type_attribute.example_catalog_attr.id
    custom_field_id      = incident_custom_field.other.id
  }
  {{- end }}
}
`))

type customFieldTemplateParams struct {
	WithCatalogType bool
	WithFilter      bool
}

func testAccIncidentCustomFieldResourceConfig(opts customFieldTemplateParams) string {
	var buf bytes.Buffer
	if err := customFieldTemplate.Execute(&buf, opts); err != nil {
		panic(err)
	}
	return buf.String()
}
