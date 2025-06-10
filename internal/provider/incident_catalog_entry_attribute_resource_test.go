package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentCatalogEntryAttributeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogEntryAttributeResourceConfig("test-value"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry_attribute.example", "value", "test-value"),
				),
			},
			// Import
			{
				ResourceName:      "incident_catalog_entry_attribute.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCatalogEntryAttributeResourceConfig("updated-value"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry_attribute.example", "value", "updated-value"),
				),
			},
		},
	})
}

func TestAccIncidentCatalogEntryAttributeResourceArray(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read array attribute
			{
				Config: testAccIncidentCatalogEntryAttributeResourceArrayConfig([]string{"value1", "value2"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry_attribute.example_array", "array_value.0", "value1"),
					resource.TestCheckResourceAttr(
						"incident_catalog_entry_attribute.example_array", "array_value.1", "value2"),
				),
			},
			// Update array values
			{
				Config: testAccIncidentCatalogEntryAttributeResourceArrayConfig([]string{"new1", "new2", "new3"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_entry_attribute.example_array", "array_value.0", "new1"),
					resource.TestCheckResourceAttr(
						"incident_catalog_entry_attribute.example_array", "array_value.1", "new2"),
					resource.TestCheckResourceAttr(
						"incident_catalog_entry_attribute.example_array", "array_value.2", "new3"),
				),
			},
		},
	})
}

func testAccIncidentCatalogEntryAttributeResourceConfig(value string) string {
	return fmt.Sprintf(`
resource "incident_catalog_type" "example" {
  name        = "Test Type"
  description = "A test catalog type"
  type_name   = "Custom[\"TestType\"]"
}

resource "incident_catalog_type_attribute" "example" {
  catalog_type_id = incident_catalog_type.example.id
  name            = "test_attribute"
  type            = "Text"
  array           = false
}

resource "incident_catalog_entry" "example" {
  catalog_type_id = incident_catalog_type.example.id
  name            = "Test Entry"
  
  attribute_values = [
    {
      attribute = incident_catalog_type_attribute.example.id
      value     = "initial-value"
    }
  ]
}

resource "incident_catalog_entry_attribute" "example" {
  catalog_entry_id = incident_catalog_entry.example.id
  attribute_id     = incident_catalog_type_attribute.example.id
  value            = %q
}
`, value)
}

func testAccIncidentCatalogEntryAttributeResourceArrayConfig(values []string) string {
	valuesStr := ""
	for i, v := range values {
		if i > 0 {
			valuesStr += ", "
		}
		valuesStr += fmt.Sprintf("%q", v)
	}

	return fmt.Sprintf(`
resource "incident_catalog_type" "example_array" {
  name        = "Test Array Type"
  description = "A test catalog type with array attribute"
  type_name   = "Custom[\"TestArrayType\"]"
}

resource "incident_catalog_type_attribute" "example_array" {
  catalog_type_id = incident_catalog_type.example_array.id
  name            = "test_array_attribute"
  type            = "Text"
  array           = true
}

resource "incident_catalog_entry" "example_array" {
  catalog_type_id = incident_catalog_type.example_array.id
  name            = "Test Array Entry"
  
  attribute_values = [
    {
      attribute   = incident_catalog_type_attribute.example_array.id
      array_value = ["initial1", "initial2"]
    }
  ]
}

resource "incident_catalog_entry_attribute" "example_array" {
  catalog_entry_id = incident_catalog_entry.example_array.id
  attribute_id     = incident_catalog_type_attribute.example_array.id
  array_value      = [%s]
}
`, valuesStr)
}