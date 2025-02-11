package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentCustomFieldDataSource(t *testing.T) {
	defaultCF := customFieldDefault()

	// Searching by name
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCustomFieldDataSourceConfig(customFieldDataSourceFixture{
					ResourceName:        defaultCF.Name,
					ResourceFieldType:   defaultCF.FieldType,
					ResourceDescription: defaultCF.Description,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field.example", "name", defaultCF.Name),
					resource.TestCheckResourceAttr(
						"data.incident_custom_field.by_name", "name", defaultCF.Name),
					resource.TestCheckResourceAttr(
						"data.incident_custom_field.by_name", "field_type", string(defaultCF.FieldType)),
					resource.TestCheckResourceAttr(
						"data.incident_custom_field.by_name", "description", defaultCF.Description),
				),
			},
		},
	})
}

var customFieldDataSourceTemplate = template.Must(template.New("incident_custom_field_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_custom_field" "example" {
  name        = {{ quote .ResourceName }}
  description = {{ quote .ResourceDescription }}
  field_type  = {{ quote .ResourceFieldType }}
}
data "incident_custom_field" "by_name" {
  name = incident_custom_field.example.name
}
`))

type customFieldDataSourceFixture struct {
	ResourceName        string
	ResourceDescription string
	ResourceFieldType   client.CustomFieldV2FieldType
}

func testAccIncidentCustomFieldDataSourceConfig(payload customFieldDataSourceFixture) string {
	var buf bytes.Buffer
	if err := customFieldDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(err)
	}

	return buf.String()
}

func customFieldDefault() client.CustomFieldV2 {
	return client.CustomFieldV2{
		Name:        "Affected Teams",
		Description: "The teams that are affected by this incident",
		FieldType:   client.CustomFieldV2FieldType("multi_select"),
	}
}
