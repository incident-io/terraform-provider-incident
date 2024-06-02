package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentCustomFieldOptionDataSource(t *testing.T) {
	// Searching by value and custom_field_type
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentCustomFieldOptionDataSourceConfig(customFieldOptionDataSourceFixture{
					ResourceValue: customFieldOptionDefault().Value,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_custom_field_option.example", "value", customFieldOptionDefault().Value),
					resource.TestCheckResourceAttr(
						"data.incident_custom_field_option.by_value_and_custom_field_id", "value", customFieldOptionDefault().Value),
				),
			},
		},
	})
}

var customFieldOptionDataSourceTemplate = template.Must(template.New("incident_custom_field_option_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_custom_field" "affected_teams" {
  name        = "Affected Teams"
  description = "The teams that are affected by this incident."
  field_type  = "multi_select"
}

resource "incident_custom_field_option" "example" {
  custom_field_id = incident_custom_field.affected_teams.id
  value           = {{ quote .Value }}
}

data "incident_custom_field_option" "by_value_and_custom_field_id" {
  value  					= incident_custom_field_option.example.value
  custom_field_id = incident_custom_field_option.example.custom_field_id
}
`))

type customFieldOptionDataSourceFixture struct {
	ResourceValue string
}

func testAccIncidentCustomFieldOptionDataSourceConfig(payload customFieldOptionDataSourceFixture) string {
	var buf bytes.Buffer
	if err := customFieldOptionDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(err)
	}

	return buf.String()
}
