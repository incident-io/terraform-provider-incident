package provider

import (
	"bytes"
	"reflect"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentCatalogTypeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogTypeResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "name", catalogTypeDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "description", catalogTypeDefault().Description),
				),
			},
			// Import
			{
				ResourceName:      "incident_catalog_type.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCatalogTypeResourceConfig(&client.CatalogTypeV2{
					Name: StableSuffix("Spaceships"),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "name", StableSuffix("Spaceships")),
				),
			},
		},
	})
}

var catalogTypeTemplate = template.Must(template.New("incident_catalog_type").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = {{ quote .Name }}
  description = {{ quote .Description }}
}
`))

func catalogTypeDefault() client.CatalogTypeV2 {
	return client.CatalogTypeV2{
		Name:        StableSuffix("Service"),
		Description: "Catalog Type Acceptance tests",
	}
}

func testAccIncidentCatalogTypeResourceConfig(override *client.CatalogTypeV2) string {
	model := catalogTypeDefault()

	// Merge any non-zero fields in override into the model.
	if override != nil {
		for idx := 0; idx < reflect.TypeOf(*override).NumField(); idx++ {
			field := reflect.ValueOf(*override).Field(idx)
			if !field.IsZero() {
				reflect.ValueOf(&model).Elem().Field(idx).Set(field)
			}
		}
	}

	var buf bytes.Buffer
	if err := catalogTypeTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
