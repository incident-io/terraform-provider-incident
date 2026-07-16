package provider

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

// TestAccIncidentCatalogTypeAttributeResourceImportMissingAttribute is a
// regression test for RESP-18431. Importing an attribute ID that doesn't exist
// on the catalog type used to fail with a cryptic "Value Conversion Error"
// against `path`, because buildModel left the optional `path` list untyped
// (types.List[DynamicPseudoType]) when the attribute wasn't found. Import now
// returns a clear diagnostic instead.
func TestAccIncidentCatalogTypeAttributeResourceImportMissingAttribute(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a catalog type + attribute so we have a real catalog type ID
			// to build a (deliberately wrong) import ID from.
			{
				Config: testAccIncidentCatalogTypeAttributeResourceConfig(client.CatalogTypeAttributeV2{
					Name: "Members",
					Type: "Text",
				}),
			},
			{
				ResourceName:      "incident_catalog_type_attribute.example",
				ImportState:       true,
				ImportStateIdFunc: testAccIncidentCatalogTypeAttributeMissingImportStateIDFunc,
				ExpectError:       regexp.MustCompile("Attribute Not Found"),
			},
		},
	})
}

// testAccIncidentCatalogTypeAttributeMissingImportStateIDFunc generates an
// import ID that points at a real catalog type but a non-existent attribute.
func testAccIncidentCatalogTypeAttributeMissingImportStateIDFunc(s *terraform.State) (string, error) {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "incident_catalog_type_attribute" {
			continue
		}

		catalogTypeID := rs.Primary.Attributes["catalog_type_id"]

		return fmt.Sprintf("%s:%s", catalogTypeID, "01THISATTRIBUTEDOESNOTEXIST"), nil
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

// TestBuildModel_PathAlwaysTyped is a regression test for RESP-18431.
//
// buildModel used to set the optional `path` list only inside the branch that
// matched the requested attribute ID. When the attribute wasn't present in the
// returned schema, `path` was left as a zero-value types.List whose element
// type is nil, which serialises to types.List[DynamicPseudoType] and is
// rejected by the framework with a "Value Conversion Error" when writing state.
// buildModel must always return a String-typed `path`, whether or not the
// attribute is found.
func TestBuildModel_PathAlwaysTyped(t *testing.T) {
	ctx := context.Background()
	r := &IncidentCatalogTypeAttributeResource{}

	catalogType := client.CatalogTypeV3{
		Id: "01CATALOGTYPE",
		Schema: client.CatalogTypeSchemaV3{
			Attributes: []client.CatalogTypeAttributeV3{
				{
					Id:    "01FOUND",
					Name:  "Members",
					Type:  "User",
					Array: true,
					Mode:  client.CatalogTypeAttributeV3ModeApi,
				},
			},
		},
	}

	tests := []struct {
		name        string
		attributeID string
		wantFound   bool
	}{
		{"attribute found", "01FOUND", true},
		{"attribute missing", "01MISSING", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, found := r.buildModel(catalogType, tt.attributeID)
			if found != tt.wantFound {
				t.Fatalf("buildModel() found = %v, want %v", found, tt.wantFound)
			}

			if !model.Path.IsNull() {
				t.Errorf("buildModel() path = %v, want null", model.Path)
			}
			if elemType := model.Path.ElementType(ctx); elemType != types.StringType {
				t.Errorf("buildModel() path element type = %v, want %v", elemType, types.StringType)
			}
		})
	}
}
