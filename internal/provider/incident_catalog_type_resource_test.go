package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentCatalogTypeResource(t *testing.T) {
	// Not setting the type name
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

	// Setting the type name explicitly
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCatalogTypeResourceConfig(&client.CatalogTypeV2{
					TypeName: generateTypeName(),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "name", catalogTypeWithTypeName().Name),
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "type_name", catalogTypeWithTypeName().TypeName),
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "description", catalogTypeWithTypeName().Description),
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
					Name:     StableSuffix("Spaceships"),
					TypeName: generateTypeName(),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "name", StableSuffix("Spaceships")),
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "type_name", generateTypeName(),
					),
				),
			},
		},
	})

	// Test use_name_as_identifier field
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with use_name_as_identifier = true
			{
				Config: testAccIncidentCatalogTypeResourceConfigWithUseNameAsIdentifier(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "name", StableSuffix("Service")),
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "use_name_as_identifier", "true"),
				),
			},
			// Import
			{
				ResourceName:      "incident_catalog_type.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update to use_name_as_identifier = false
			{
				Config: testAccIncidentCatalogTypeResourceConfigWithUseNameAsIdentifier(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_catalog_type.example", "use_name_as_identifier", "false"),
				),
			},
		},
	})
}

func generateTypeName() string {
	// The test run ID is a uuid, which won't be accepted. Strip it down to
	// something allowed
	allowedRunID := strings.ReplaceAll(testRunID, "-", "")
	// Numbers are not allowed
	numberRegexp := regexp.MustCompile("[0-9]")
	allowedRunID = numberRegexp.ReplaceAllString(allowedRunID, "")

	return fmt.Sprintf(`Custom["Spaceships%s"]`, allowedRunID)
}

var catalogTypeTemplate = template.Must(template.New("incident_catalog_type").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name        = {{ quote .Name }}
  {{ if ne .TypeName "" }}type_name   = {{ quote .TypeName }}{{ end }}
  description = {{ quote .Description }}

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}
`))

var catalogTypeWithUseNameAsIdentifierTemplate = template.Must(template.New("incident_catalog_type_with_use_name").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_catalog_type" "example" {
  name                   = {{ quote .Name }}
  description            = {{ quote .Description }}
  use_name_as_identifier = {{ .UseNameAsIdentifier }}

  source_repo_url = "https://github.com/incident-io/terraform-demo"
}
`))

func catalogTypeDefault() client.CatalogTypeV2 {
	return client.CatalogTypeV2{
		Name:        StableSuffix("Service"),
		Description: "Catalog Type Acceptance tests",
	}
}

func catalogTypeWithTypeName() client.CatalogTypeV2 {
	return client.CatalogTypeV2{
		Name:        StableSuffix("Service"),
		TypeName:    generateTypeName(),
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

func testAccIncidentCatalogTypeResourceConfigWithUseNameAsIdentifier(useNameAsIdentifier bool) string {
	model := struct {
		Name                 string
		Description          string
		UseNameAsIdentifier  bool
	}{
		Name:                 StableSuffix("Service"),
		Description:          "Catalog Type Acceptance tests",
		UseNameAsIdentifier:  useNameAsIdentifier,
	}

	var buf bytes.Buffer
	if err := catalogTypeWithUseNameAsIdentifierTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
