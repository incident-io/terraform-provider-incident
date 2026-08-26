package provider

import (
	"bytes"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentStatusDataSource(t *testing.T) {
	defaultStatus := incidentStatusDefault()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Look the status up both by ID and by name.
			{
				Config: testAccIncidentStatusDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.incident_status.by_id", "name", defaultStatus.Name),
					resource.TestCheckResourceAttr(
						"data.incident_status.by_id", "description", defaultStatus.Description),
					resource.TestCheckResourceAttr(
						"data.incident_status.by_id", "category", string(defaultStatus.Category)),
					resource.TestCheckResourceAttrPair(
						"data.incident_status.by_name", "id", "incident_status.example", "id"),
					resource.TestCheckResourceAttr(
						"data.incident_status.by_name", "description", defaultStatus.Description),
					resource.TestCheckResourceAttr(
						"data.incident_status.by_name", "category", string(defaultStatus.Category)),
				),
			},
		},
	})
}

// Both lookup attributes are Optional and Computed, so setting both has to be
// rejected explicitly rather than by the schema.
func TestAccIncidentStatusDataSourceAmbiguousLookup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_status" "both" {
  id   = "01HPYFHF7NA1TDPY9WQNPPP1XT"
  name = "Triage"
}
`,
				ExpectError: regexp.MustCompile("Ambiguous lookup"),
			},
		},
	})
}

var incidentStatusDataSourceTemplate = template.Must(template.New("incident_status_data_source").Funcs(testTemplateFuncs()).Parse(`
resource "incident_status" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
  category     = {{ quote .Category }}
}
data "incident_status" "by_id" {
  id = incident_status.example.id
}
data "incident_status" "by_name" {
  name = incident_status.example.name
}
`))

func testAccIncidentStatusDataSourceConfig() string {
	var buf bytes.Buffer
	if err := incidentStatusDataSourceTemplate.Execute(&buf, incidentStatusDefault()); err != nil {
		panic(err)
	}

	return buf.String()
}
