package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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
					Name: "Godawful",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_severity.example", "name", "Godawful"),
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
					Rank: -1,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_severity.example", "name", incidentSeverityDefault().Name),
				),
			},
		},
	})
}

var incidentSeverityTemplate = template.Must(template.New("incident_severity").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_severity" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
{{ if gt .Rank 0 }}
  rank         = {{ toJson .Rank }}
{{ end }}
}
`))

func incidentSeverityDefault() client.SeverityV2 {
	return client.SeverityV2{
		Name:        "Major",
		Description: "Issues causing significant impact. Immediate response is usually required.",
		Rank:        7,
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
