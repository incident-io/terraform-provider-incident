package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentRoleDataSource(t *testing.T) {
	defaultCF := incidentRoleDefault()

	// Searching by name
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentRoleDataSourceConfig(incidentRoleDataSourceFixture{
					Name:         defaultCF.Name,
					Description:  defaultCF.Description,
					Instructions: defaultCF.Instructions,
					Shortform:    defaultCF.Shortform,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.incident_role.by_name", "name", defaultCF.Name),
					resource.TestCheckResourceAttr(
						"data.incident_role.by_name", "description", defaultCF.Description),
				),
			},
		},
	})
}

var incidentRoleDataSourceTemplate = template.Must(template.New("incident_role_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_role" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
  instructions = {{ quote .Instructions }}
  shortform    = {{ quote .Shortform }}
}
`))

type incidentRoleDataSourceFixture struct {
	Name         string
	Description  string
	Instructions string
	Shortform    string
}

func testAccIncidentRoleDataSourceConfig(payload incidentRoleDataSourceFixture) string {
	var buf bytes.Buffer
	if err := incidentRoleDataSourceTemplate.Execute(&buf, payload); err != nil {
		panic(err)
	}

	return buf.String()
}
