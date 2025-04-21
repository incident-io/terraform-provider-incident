package provider

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentRoleDataSource(t *testing.T) {
	defaultIncidentRole := incidentRoleDefault()

	// Searching by name
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentRoleDataSourceConfig(incidentRoleDataSourceFixture{
					Name:         defaultIncidentRole.Name,
					Description:  defaultIncidentRole.Description,
					Instructions: defaultIncidentRole.Instructions,
					Shortform:    defaultIncidentRole.Shortform,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_incident_role.example", "name", defaultIncidentRole.Name),
					resource.TestCheckResourceAttr(
						"data.incident_incident_role.by_id", "name", defaultIncidentRole.Name),
					resource.TestCheckResourceAttr(
						"data.incident_incident_role.by_id", "description", defaultIncidentRole.Description),
					resource.TestCheckResourceAttr(
						"data.incident_incident_role.by_id", "instructions", defaultIncidentRole.Instructions),
					resource.TestCheckResourceAttr(
						"data.incident_incident_role.by_id", "shortform", defaultIncidentRole.Shortform),
				),
			},
		},
	})
}

var incidentRoleDataSourceTemplate = template.Must(template.New("incident_role_data_source").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_incident_role" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
  instructions = {{ quote .Instructions }}
  shortform    = {{ quote .Shortform }}
}
data "incident_incident_role" "by_id" {
  id         = incident_incident_role.example.id
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
