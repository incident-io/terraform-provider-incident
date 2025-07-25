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
	boolPtr := func(b bool) *bool { return &b }

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read (backwards compatibility - no required field)
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
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "required", "false"),
				),
			},
			// Import
			{
				ResourceName:      "incident_alert_attribute.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update to include required=true
			{
				Config: testAccAlertAttributeResourceConfig(alertAttributeElement{
					Name:     "Severity",
					Type:     "String",
					Array:    false,
					Required: boolPtr(true),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "name", "Severity"),
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "required", "true"),
				),
			},
			// Update to required=false
			{
				Config: testAccAlertAttributeResourceConfig(alertAttributeElement{
					Name:     "UpdatedSeverity",
					Type:     "String",
					Array:    false,
					Required: boolPtr(false),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "name", "UpdatedSeverity"),
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "required", "false"),
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

func TestAccAlertAttributeResourceWithRequired(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read with required=true
			{
				Config: testAccAlertAttributeResourceConfig(alertAttributeElement{
					Name:     "RequiredAttribute",
					Type:     "String",
					Array:    false,
					Required: boolPtr(true),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "name", "RequiredAttribute"),
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "type", "String"),
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "array", "false"),
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "required", "true"),
				),
			},
			// Import state should work
			{
				ResourceName:      "incident_alert_attribute.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

var alertAttributeTemplate = template.Must(template.New("incident_alert_attribute").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_alert_attribute" "example" {
  name  = {{ quote .Name }}
  type  = {{ quote .Type }}
  array = {{ .Array }}{{- if .Required }}
  required = {{ .Required }}{{- end }}
}
`))

type alertAttributeElement struct {
	Name     string
	Type     string
	Array    bool
	Required *bool // pointer to handle optional field
}

func testAccAlertAttributeResourceConfig(attribute alertAttributeElement) string {
	var buf bytes.Buffer
	if err := alertAttributeTemplate.Execute(&buf, attribute); err != nil {
		panic(err)
	}

	return buf.String()
}
