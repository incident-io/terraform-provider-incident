package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// mockCatalogAPI stands up an in-process incident.io API that serves a single
// catalog type (and accepts schema updates), so the import + read + plan flow
// can be driven end-to-end without a real backend.
func mockCatalogAPI(t *testing.T, catalogType client.CatalogTypeV3) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Showing the catalog type: used by import + read.
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v3/catalog_types/"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(client.CatalogShowTypeResultV3{CatalogType: catalogType})
			return
		// Updating the schema: used by create/update/delete.
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/update_schema"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(client.CatalogUpdateTypeSchemaResultV3{CatalogType: catalogType})
			return
		}

		t.Errorf("unexpected request to mock incident.io API: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	return server
}

// TestIncidentCatalogTypeAttributeResourceImportBlock reproduces RESP-18431.
//
// Importing an incident_catalog_type_attribute via a Terraform `import {}` block
// (Terraform 1.5+) used to fail during planning with:
//
//	Error: Value Conversion Error
//	Expected framework type from provider logic: types.ListType[basetypes.StringType]
//	Received framework type from provider logic: types.ListType[!!! MISSING TYPE !!!]
//	Path: path
//
// This is a unit test (no external dependencies) that stands up a mock incident.io
// API and drives a real Terraform binary through an import block, so it exercises
// the same import-time planning path that the real CLI does.
func TestIncidentCatalogTypeAttributeResourceImportBlock(t *testing.T) {
	const (
		catalogTypeID = "01ABCCATALOGTYPE"
		attributeID   = "01XYZATTRIBUTE"
	)

	catalogType := client.CatalogTypeV3{
		Id:       catalogTypeID,
		Name:     "Team",
		TypeName: `Custom["Team"]`,
		Schema: client.CatalogTypeSchemaV3{
			Version: 1,
			Attributes: []client.CatalogTypeAttributeV3{
				{
					Id:    attributeID,
					Name:  "Members",
					Type:  "User",
					Array: true,
					Mode:  client.CatalogTypeAttributeV3ModeApi,
				},
			},
		},
	}

	server := mockCatalogAPI(t, catalogType)

	t.Setenv("INCIDENT_ENDPOINT", server.URL)
	t.Setenv("INCIDENT_API_KEY", "test-api-key")

	config := `
resource "incident_catalog_type_attribute" "team_members" {
  catalog_type_id = "` + catalogTypeID + `"
  name            = "Members"
  type            = "User"
  array           = true
}

import {
  id = "` + catalogTypeID + `:` + attributeID + `"
  to = incident_catalog_type_attribute.team_members
}
`

	resource.UnitTest(t, resource.TestCase{
		// `import {}` config blocks are only supported from Terraform 1.5.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_5_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.team_members", "id", attributeID),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.team_members", "catalog_type_id", catalogTypeID),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.team_members", "name", "Members"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.team_members", "type", "User"),
					resource.TestCheckResourceAttr(
						"incident_catalog_type_attribute.team_members", "array", "true"),
				),
			},
		},
	})
}

// TestIncidentCatalogTypeAttributeResourceImportBlockMissingAttribute checks
// that importing an attribute that doesn't exist in the catalog type's schema
// produces a clear diagnostic rather than the cryptic "Value Conversion Error"
// from RESP-18431.
//
// Before the fix, buildModel left `path` as an untyped zero-value list when the
// attribute wasn't found, so the import-time Read failed while writing state:
//
//	Error: Value Conversion Error
//	Received framework type from provider logic: types.ListType[!!! MISSING TYPE !!!]
//	Path: path
func TestIncidentCatalogTypeAttributeResourceImportBlockMissingAttribute(t *testing.T) {
	const (
		catalogTypeID = "01ABCCATALOGTYPE"
		attributeID   = "01XYZATTRIBUTE"
	)

	catalogType := client.CatalogTypeV3{
		Id:       catalogTypeID,
		Name:     "Team",
		TypeName: `Custom["Team"]`,
		Schema: client.CatalogTypeSchemaV3{
			Version: 1,
			// The imported attribute ID is deliberately absent from the schema.
			Attributes: []client.CatalogTypeAttributeV3{
				{
					Id:   "01SOMEOTHERATTRIBUTE",
					Name: "Something else",
					Type: "Text",
					Mode: client.CatalogTypeAttributeV3ModeApi,
				},
			},
		},
	}

	server := mockCatalogAPI(t, catalogType)

	t.Setenv("INCIDENT_ENDPOINT", server.URL)
	t.Setenv("INCIDENT_API_KEY", "test-api-key")

	config := `
resource "incident_catalog_type_attribute" "team_members" {
  catalog_type_id = "` + catalogTypeID + `"
  name            = "Members"
  type            = "User"
  array           = true
}

import {
  id = "` + catalogTypeID + `:` + attributeID + `"
  to = incident_catalog_type_attribute.team_members
}
`

	resource.UnitTest(t, resource.TestCase{
		// `import {}` config blocks are only supported from Terraform 1.5.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_5_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`Attribute Not Found`),
			},
		},
	})
}
