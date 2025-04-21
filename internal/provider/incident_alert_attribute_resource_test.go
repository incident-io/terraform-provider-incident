package provider

import (
	"bytes"
	"regexp"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAlertAttributeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccAlertAttributeResourceConfig(alertAttributeElement{
					Name:  "Severity",
					Type:  "String",
					Array: false,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "name", "Severity"),
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "type", "String"),
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "array", "false"),
				),
			},
			// Import
			{
				ResourceName:      "incident_alert_attribute.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccAlertAttributeResourceConfig(alertAttributeElement{
					Name:  "UpdatedSeverity",
					Type:  "String",
					Array: false,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "name", "UpdatedSeverity"),
				),
			},
			// Block use of `Priority` as an attribute name. This relies on our API giving you a
			// 422 when you try and create an attribute with this name as part of our work to migrate
			// priority to becoming a regular alert attribute.
			{
				Config: testAccAlertAttributeResourceConfig(alertAttributeElement{
					Name:  "Priority",
					Type:  "String",
					Array: false,
				}),
				ExpectError: regexp.MustCompile("cannot have an attribute named 'Priority'"),
			},
			{
				Config: testAccAlertAttributeResourceConfig(alertAttributeElement{
					Name:  "UpdatedSeverity",
					Type:  "String",
					Array: false,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "name", "UpdatedSeverity"),
				),
			},
		},
	})
}

var alertAttributeTemplate = template.Must(template.New("incident_alert_attribute").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_alert_attribute" "example" {
  name  = {{ quote .Name }}
  type  = {{ quote .Type }}
  array = {{ .Array }}
}
`))

type alertAttributeElement struct {
	Name  string
	Type  string
	Array bool
}

func testAccAlertAttributeResourceConfig(attribute alertAttributeElement) string {
	var buf bytes.Buffer
	if err := alertAttributeTemplate.Execute(&buf, attribute); err != nil {
		panic(err)
	}

	return buf.String()
}
