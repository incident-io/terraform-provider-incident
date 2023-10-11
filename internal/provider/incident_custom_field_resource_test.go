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

func TestAccIncidentCustomFieldResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCustomFieldResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "name", customFieldDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "description", customFieldDefault().Description),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "field_type", string(customFieldDefault().FieldType)),
				),
			},
			// Import
			{
				ResourceName:      "incident_custom_field.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentCustomFieldResourceConfig(&client.CustomFieldV2{
					Name: "Unlucky Teams",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "name", "Unlucky Teams"),
				),
			},
		},
	})
}

var customFieldTemplate = template.Must(template.New("incident_custom_field").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_custom_field" "example" {
  name                      = {{ quote .Name }}
  description               = {{ quote .Description }}
  field_type                = {{ quote .FieldType }}
}
`))

func customFieldDefault() client.CustomFieldV2 {
	return client.CustomFieldV2{
		Name:        "Affected Teams",
		Description: "The teams that are affected by this incident",
		FieldType:   client.CustomFieldV2FieldType("multi_select"),
	}
}

func testAccIncidentCustomFieldResourceConfig(override *client.CustomFieldV2) string {
	model := customFieldDefault()

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
	if err := customFieldTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
