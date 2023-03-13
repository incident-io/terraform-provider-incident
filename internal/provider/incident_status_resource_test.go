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

func TestAccIncidentStatusResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentStatusResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_status.example", "name", incidentStatusDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_status.example", "description", incidentStatusDefault().Description),
					resource.TestCheckResourceAttr(
						"incident_status.example", "category", string(incidentStatusDefault().Category)),
				),
			},
			// Import
			{
				ResourceName:      "incident_status.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentStatusResourceConfig(&client.IncidentStatusV1{
					Name: "Clean-up",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_status.example", "name", "Clean-up"),
				),
			},
		},
	})
}

var incidentStatusTemplate = template.Must(template.New("incident_status").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_status" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
  category     = {{ quote .Category }}
}
`))

func incidentStatusDefault() client.IncidentStatusV1 {
	return client.IncidentStatusV1{
		Name:        "Complete",
		Description: "Impact has been fully mitigated.",
		Category:    client.IncidentStatusV1CategoryClosed,
	}
}

func testAccIncidentStatusResourceConfig(override *client.IncidentStatusV1) string {
	model := incidentStatusDefault()

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
	if err := incidentStatusTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
