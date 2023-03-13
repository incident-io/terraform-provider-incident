package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentRoleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentRoleResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_incident_role.example", "name", incidentRoleDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_incident_role.example", "description", incidentRoleDefault().Description),
					resource.TestCheckResourceAttr(
						"incident_incident_role.example", "instructions", incidentRoleDefault().Instructions),
					resource.TestCheckResourceAttr(
						"incident_incident_role.example", "shortform", incidentRoleDefault().Shortform),
					resource.TestCheckResourceAttr(
						"incident_incident_role.example", "required", fmt.Sprintf("%v", incidentRoleDefault().Required)),
				),
			},
			// Import
			{
				ResourceName:      "incident_incident_role.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentRoleResourceConfig(&client.IncidentRoleV1{
					Name: "Communications Follow",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_incident_role.example", "name", "Communications Follow"),
				),
			},
		},
	})
}

var incidentRoleTemplate = template.Must(template.New("incident_role").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_incident_role" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
  instructions = {{ quote .Instructions }}
  shortform    = {{ quote .Shortform }}
  required     = {{ toJson .Required }}
}
`))

func incidentRoleDefault() client.IncidentRoleV1 {
	return client.IncidentRoleV1{
		Name:         "Communications Lead",
		Description:  "Responsible for communications on behalf of the response team.",
		Instructions: "Manage internal and external communications on behalf of the response team.",
		Shortform:    "communications",
		Required:     false,
	}
}

func testAccIncidentRoleResourceConfig(override *client.IncidentRoleV1) string {
	model := incidentRoleDefault()

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
	if err := incidentRoleTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
