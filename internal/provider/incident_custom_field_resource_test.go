package provider

import (
	"bytes"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
						"incident_custom_field.example", "name", StableSuffix("Features")),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "description", "Features impacted by this incident"),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "field_type", "multi_select"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "filter_by"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "fixed_filter"),
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

// TestAccIncidentCustomFieldResource_FieldTypes covers each field_type the API
// accepts, matching the split examples in examples/resources/incident_custom_field.
func TestAccIncidentCustomFieldResource_FieldTypes(t *testing.T) {
	for _, fieldType := range []string{"single_select", "multi_select", "text", "link", "numeric"} {
		t.Run(fieldType, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testAccIncidentCustomFieldResourceConfig(customFieldTemplateParams{
							Name:      fieldType,
							FieldType: fieldType,
						}),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(
								"incident_custom_field.example", "name", StableSuffix(fieldType)),
							resource.TestCheckResourceAttr(
								"incident_custom_field.example", "field_type", fieldType),
						),
					},
					{
						ResourceName:      "incident_custom_field.example",
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
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
						"incident_custom_field.example", "name", StableSuffix("Features")),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "description", "Features impacted by this incident"),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "field_type", "multi_select"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "filter_by"),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "fixed_filter"),
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
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "fixed_filter"),
				),
			},
			// Swap the dynamic filter for a fixed one. The API stores both on the same
			// field, so setting the fixed filter has to clear filter_by.
			{
				Config: testAccIncidentCustomFieldResourceConfig(customFieldTemplateParams{WithCatalogType: true, WithFixedFilter: true}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"incident_custom_field.example", "fixed_filter.catalog_attribute_id",
						"incident_catalog_type_attribute.example_catalog_attr", "id",
					),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "fixed_filter.values.#", "2"),
					resource.TestCheckTypeSetElemAttrPair(
						"incident_custom_field.example", "fixed_filter.values.*",
						"incident_catalog_entry.other_first", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"incident_custom_field.example", "fixed_filter.values.*",
						"incident_catalog_entry.other_second", "id",
					),
					resource.TestCheckNoResourceAttr(
						"incident_custom_field.example", "filter_by"),
				),
			},
			// Import, to check the fixed filter round-trips through a read
			{
				ResourceName:      "incident_custom_field.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccIncidentCustomFieldResource_BothFilters checks we reject a config asking for both
// filters before it reaches the API, which would otherwise silently keep only one.
func TestAccIncidentCustomFieldResource_BothFilters(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentCustomFieldResourceConfig(customFieldTemplateParams{
					WithCatalogType: true, WithFilter: true, WithFixedFilter: true,
				}),
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

var customFieldTemplate = template.Must(template.New("incident_custom_field").Funcs(testTemplateFuncs()).Parse(`
{{- if .WithCatalogType }}
resource "incident_catalog_type" "example" {
  name = {{ stableSuffix "My type" | quote }}
  description = "My type description"

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}

resource "incident_catalog_type_attribute" "example_string_attr" {
  catalog_type_id = incident_catalog_type.example.id
  name = "My string attribute"
  type = "String"
}

resource "incident_catalog_type" "other" {
  name = {{ stableSuffix "My other type" | quote }}
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
  name = {{ stableSuffix "Other field" | quote }}
  description = "Other field description"

  field_type = "single_select"
  catalog_type_id = incident_catalog_type.other.id
}
{{- end }}

{{- if .WithFixedFilter }}
resource "incident_catalog_entry" "other_first" {
  catalog_type_id = incident_catalog_type.other.id
  name = "First other entry"

  attribute_values = []
}

resource "incident_catalog_entry" "other_second" {
  catalog_type_id = incident_catalog_type.other.id
  name = "Second other entry"

  attribute_values = []
}
{{- end }}

resource "incident_custom_field" "example" {
  name                          = {{ .Name | default "Features" | stableSuffix | quote }}
  description                   = "Features impacted by this incident"
  field_type                     = {{ .FieldType | default "multi_select" | quote }}

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

  {{- if .WithFixedFilter }}
  fixed_filter = {
    catalog_attribute_id = incident_catalog_type_attribute.example_catalog_attr.id
    values = [
      incident_catalog_entry.other_first.id,
      incident_catalog_entry.other_second.id,
    ]
  }
  {{- end }}
}
`))

type customFieldTemplateParams struct {
	Name            string
	FieldType       string
	WithCatalogType bool
	WithFilter      bool
	WithFixedFilter bool
}

func testAccIncidentCustomFieldResourceConfig(opts customFieldTemplateParams) string {
	var buf bytes.Buffer
	if err := customFieldTemplate.Execute(&buf, opts); err != nil {
		panic(err)
	}
	return buf.String()
}
