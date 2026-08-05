package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentSeverityResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentSeverityResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_severity.example", "name", incidentSeverityDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_severity.example", "description", incidentSeverityDefault().Description),
					resource.TestCheckResourceAttr(
						"incident_severity.example", "rank", fmt.Sprintf("%d", incidentSeverityDefault().Rank)),
				),
			},
			// Import
			{
				ResourceName:      "incident_severity.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccIncidentSeverityResourceConfig(&client.SeverityV2{
					Name: StableSuffix("Godawful"),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_severity.example", "name", StableSuffix("Godawful")),
				),
			},
		},
	})
}

func TestAccIncidentSeverityResourceWithoutRank(t *testing.T) {
	// Verify the computed rank is set without issue.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentSeverityResourceConfig(&client.SeverityV2{
					Name: StableSuffix("Pretty bad"),
					Rank: -1,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_severity.example", "name", StableSuffix("Pretty bad")),
				),
			},
		},
	})
}

var incidentSeverityTemplate = template.Must(template.New("incident_severity").Funcs(testTemplateFuncs()).Parse(`
resource "incident_severity" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
{{ if gt .Rank 0 }}
  rank         = {{ toJson .Rank }}
{{ end }}
}
`))

// stableRank derives a rank from the test run, because ranks must be unique within the
// org and the CLI legs of a CI run create severities concurrently. Real severities sit
// at low ranks, so we stay well clear of them.
func stableRank() int64 {
	n, err := strconv.ParseInt(testRunShortID, 16, 64)
	if err != nil {
		panic(err)
	}

	return 100 + n%1000
}

func incidentSeverityDefault() client.SeverityV2 {
	return client.SeverityV2{
		Name:        StableSuffix("P0"),
		Description: "All work stops until this issue is resolved.",
		Rank:        stableRank(),
	}
}

func testAccIncidentSeverityResourceConfig(override *client.SeverityV2) string {
	model := incidentSeverityDefault()

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
	if err := incidentSeverityTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
