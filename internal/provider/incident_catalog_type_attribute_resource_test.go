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
	"github.com/samber/lo"

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
					// We haven't set mode, so should default to "api", meaning schema_only is false
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "schema_only", "false"),
				),
			},
			// Update and read
			{
				Config: testAccIncidentCatalogTypeAttributeResourceConfig(client.CatalogTypeAttributeV2{
					Name:  "Description",
					Type:  "String",
					Array: true,
					Mode:  "api",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "name", "Description"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "type", "String"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "array", "true"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.example", "schema_only", "false"),
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

func TestAttributeToPayload_PreservesModes(t *testing.T) {
	tests := []struct {
		mode              client.CatalogTypeAttributeV3Mode
		backlinkAttribute *string
		path              *[]client.CatalogTypeAttributePathItemV3
		expectedMode      client.CatalogTypeAttributePayloadV3Mode
	}{
		// Schema-only modes should be preserved
		{client.CatalogTypeAttributeV3ModeDashboard, nil, nil, client.CatalogTypeAttributePayloadV3ModeDashboard},
		{client.CatalogTypeAttributeV3ModeInternal, nil, nil, client.CatalogTypeAttributePayloadV3ModeInternal},
		{client.CatalogTypeAttributeV3ModeExternal, nil, nil, client.CatalogTypeAttributePayloadV3ModeExternal},
		{client.CatalogTypeAttributeV3ModeDynamic, nil, nil, client.CatalogTypeAttributePayloadV3ModeDynamic},
		// Non-schema-only modes default to api
		{client.CatalogTypeAttributeV3ModeApi, nil, nil, client.CatalogTypeAttributePayloadV3ModeApi},
		{client.CatalogTypeAttributeV3ModeEmpty, nil, nil, client.CatalogTypeAttributePayloadV3ModeApi},
		// Backlink and path modes
		{client.CatalogTypeAttributeV3ModeBacklink, lo.ToPtr("other-attr"), nil, client.CatalogTypeAttributePayloadV3ModeBacklink},
		{client.CatalogTypeAttributeV3ModePath, nil, &[]client.CatalogTypeAttributePathItemV3{{AttributeId: "attr-1"}}, client.CatalogTypeAttributePayloadV3ModePath},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			r := &IncidentCatalogTypeAttributeResource{}
			attribute := client.CatalogTypeAttributeV3{
				Id:                "attr-id",
				Name:              "Test Attribute",
				Type:              "Text",
				Array:             false,
				Mode:              tt.mode,
				BacklinkAttribute: tt.backlinkAttribute,
				Path:              tt.path,
			}

			payload := r.attributeToPayload(attribute)

			if *payload.Mode != tt.expectedMode {
				t.Errorf("attributeToPayload() Mode = %v, want %v", *payload.Mode, tt.expectedMode)
			}
		})
	}
}
