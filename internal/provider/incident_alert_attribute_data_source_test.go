package provider

import (
	"bytes"
	"fmt"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentAlertAttributeDataSource(t *testing.T) {
	defaultAlertAttribute := alertAttributeDefault()

	// Searching by name
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// CRUD
			{
				Config: testAccIncidentAlertAttributeDataSourceConfig(incidentAlertAttributeDataSourceFixture{
					Name:  defaultAlertAttribute.Name,
					Type:  defaultAlertAttribute.Type,
					Array: defaultAlertAttribute.Array,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_alert_attribute.example", "name", defaultAlertAttribute.Name),
					resource.TestCheckResourceAttr(
						"data.incident_alert_attribute.by_name", "name", defaultAlertAttribute.Name),
					resource.TestCheckResourceAttr(
						"data.incident_alert_attribute.by_name", "type", defaultAlertAttribute.Type),
					resource.TestCheckResourceAttr(
						"data.incident_alert_attribute.by_name", "array", fmt.Sprintf("%t", defaultAlertAttribute.Array)),
				),
			},
		},
	})
}

var incidentAlertAttributeDataSourceTemplate = template.Must(template.New("incident_alert_attribute_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_alert_attribute" "example" {
  name  = {{ quote .Name }}
  type  = {{ quote .Type }}
  array = {{ .Array }}
}
data "incident_alert_attribute" "by_name" {
  name         = incident_alert_attribute.example.name
}
`))

func alertAttributeDefault() client.AlertAttributeV2 {
	return client.AlertAttributeV2{
		Name:  "Severity",
		Type:  "String",
		Array: false,
	}
}

type incidentAlertAttributeDataSourceFixture struct {
	Name  string
	Type  string
	Array bool
}

func testAccIncidentAlertAttributeDataSourceConfig(payload incidentAlertAttributeDataSourceFixture) string {
	var buf bytes.Buffer
	if err := incidentAlertAttributeDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(err)
	}

	return buf.String()
}
