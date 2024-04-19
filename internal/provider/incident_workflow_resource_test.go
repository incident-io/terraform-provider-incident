package provider

import (
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestAccIncidentWorkflowResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and check state
			{
				Config: overrideAndMarshalModel(incidentWorkflowDefault(), nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "name", incidentWorkflowDefault().Name),
				),
			},
			// Import
			{
				ResourceName:      "incident_workflow.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and check new state
			{
				Config: overrideAndMarshalModel(incidentWorkflowDefault(), &client.Workflow{
					Name: "My New Name",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_workflow.example", "name", "My New Name"),
				),
			},
			// (Clean-up)
		},
	})
}

var incidentWorkflowTemplate = template.Must(template.New("incident_workflow").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_workflow" "example" {
  name               = {{ quote .Name }}
  trigger            = "incident.updated"
  terraform_repo_url = "https://github.com/incident-io/test"
}
`))

func incidentWorkflowDefault() *client.Workflow {
	return &client.Workflow{
		Name: "My Test Workflow",
	}
}
