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
	"github.com/samber/lo"
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
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "required", string(customFieldDefault().Required)),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "show_before_closure", fmt.Sprintf("%v", customFieldDefault().ShowBeforeClosure)),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "show_before_creation", fmt.Sprintf("%v", customFieldDefault().ShowBeforeCreation)),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "show_before_update", fmt.Sprintf("%v", customFieldDefault().ShowBeforeUpdate)),
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "show_in_announcement_post", fmt.Sprintf("%v", *customFieldDefault().ShowInAnnouncementPost)),
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
				Config: testAccIncidentCustomFieldResourceConfig(&client.CustomFieldV1{
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
	required                  = {{ quote .Required }}
	show_before_creation      = {{ toJson .ShowBeforeCreation }}
	show_before_closure       = {{ toJson .ShowBeforeClosure }}
	show_before_update        = {{ toJson .ShowBeforeUpdate }}
	show_in_announcement_post = {{ toJson .ShowInAnnouncementPost }}
}
`))

func customFieldDefault() client.CustomFieldV1 {
	return client.CustomFieldV1{
		Name:                   "Affected Teams",
		Description:            "The teams that are affected by this incident",
		FieldType:              client.CustomFieldV1FieldType("multi_select"),
		Required:               client.CustomFieldV1RequiredAlways,
		ShowBeforeCreation:     true,
		ShowBeforeClosure:      true,
		ShowBeforeUpdate:       true,
		ShowInAnnouncementPost: lo.ToPtr(false),
	}
}

func testAccIncidentCustomFieldResourceConfig(override *client.CustomFieldV1) string {
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
