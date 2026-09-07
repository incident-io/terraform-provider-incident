package provider

import (
	"bytes"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentSeverityDataSource(t *testing.T) {
	severity := incidentSeverityDefault()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentSeverityDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.incident_severity.by_id", "id", "incident_severity.example", "id"),
					resource.TestCheckResourceAttr(
						"data.incident_severity.by_id", "name", severity.Name),
					resource.TestCheckResourceAttr(
						"data.incident_severity.by_id", "description", severity.Description),
					resource.TestCheckResourceAttrPair(
						"data.incident_severity.by_id", "rank", "incident_severity.example", "rank"),
					resource.TestCheckResourceAttrPair(
						"data.incident_severity.by_name", "id", "incident_severity.example", "id"),
					resource.TestCheckResourceAttr(
						"data.incident_severity.by_name", "description", severity.Description),
					resource.TestCheckResourceAttrPair(
						"data.incident_severity.by_name", "rank", "incident_severity.example", "rank"),
				),
			},
		},
	})
}

func TestAccIncidentSeverityDataSourceAmbiguousLookup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_severity" "both" {
  id   = "01FCNDV6P870EA6S7TK1DSYDG0"
  name = "Minor"
}
`,
				ExpectError: regexp.MustCompile("Ambiguous lookup"),
			},
		},
	})
}

var incidentSeverityDataSourceTemplate = template.Must(template.New("incident_severity_data_source").Funcs(testTemplateFuncs()).Parse(`
resource "incident_severity" "example" {
  name        = {{ quote .Name }}
  description = {{ quote .Description }}
  rank        = {{ toJson .Rank }}
}
data "incident_severity" "by_id" {
  id = incident_severity.example.id
}
data "incident_severity" "by_name" {
  name = incident_severity.example.name
}
`))

func testAccIncidentSeverityDataSourceConfig() string {
	var buf bytes.Buffer
	if err := incidentSeverityDataSourceTemplate.Execute(&buf, incidentSeverityDefault()); err != nil {
		panic(err)
	}

	return buf.String()
}
