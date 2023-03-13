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

func TestAccIncidentCustomFieldOptionResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCustomFieldOptionResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field_option.example", "value", customFieldOptionDefault().Value),
				),
			},
			// Import
			{
				ResourceName:      "incident_custom_field_option.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCustomFieldOptionResourceConfig(&client.CustomFieldOptionV1{
					Value: "Dashboard",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field_option.example", "value", "Dashboard"),
				),
			},
		},
	})
}

var customFieldOptionTemplate = template.Must(template.New("incident_custom_field").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_custom_field" "affected_teams" {
  name        = "Affected Teams"
  description = "The teams that are affected by this incident."
  field_type  = "multi_select"
  required    = "always"

  show_before_creation      = true
  show_before_closure       = true
  show_before_update        = true
  show_in_announcement_post = true
}

resource "incident_custom_field_option" "example" {
  custom_field_id = incident_custom_field.affected_teams.id
  value           = {{ quote .Value }}
}
`))

func customFieldOptionDefault() client.CustomFieldOptionV1 {
	return client.CustomFieldOptionV1{
		Value: "Payments",
	}
}

func testAccIncidentCustomFieldOptionResourceConfig(override *client.CustomFieldOptionV1) string {
	model := customFieldOptionDefault()

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
	if err := customFieldOptionTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
