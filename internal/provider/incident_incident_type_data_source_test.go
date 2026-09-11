package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// TestAccIncidentIncidentTypeDataSource looks an incident type up both ways round.
//
// Incident types can't be created from Terraform - they're configured in your
// settings - so rather than hardcode a name the test asks the account what it has,
// the same way the timestamp data source's test does.
func TestAccIncidentIncidentTypeDataSource(t *testing.T) {
	// The lookup below runs before resource.Test, so honour TF_ACC ourselves rather
	// than calling the API during a unit test run, then initialise testClient.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}
	testAccPreCheck(t)

	incidentType := testAccIncidentType(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("incident_incident_type_data_source", `
data "incident_incident_type" "by_id" {
  id = {{ quote .Id }}
}

data "incident_incident_type" "by_name" {
  name = {{ quote .Name }}
}
`, incidentType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.incident_incident_type.by_id", "name", incidentType.Name),
					resource.TestCheckResourceAttr(
						"data.incident_incident_type.by_name", "id", incidentType.Id),
					resource.TestCheckResourceAttrSet(
						"data.incident_incident_type.by_id", "create_in_triage"),
					// The two lookups are one resolution step apart and shouldn't
					// disagree about anything they both report.
					resource.TestCheckResourceAttrPair(
						"data.incident_incident_type.by_name", "is_default",
						"data.incident_incident_type.by_id", "is_default"),
					resource.TestCheckResourceAttrPair(
						"data.incident_incident_type.by_name", "private_incidents_only",
						"data.incident_incident_type.by_id", "private_incidents_only"),
				),
			},
		},
	})
}

// Both lookup attributes are Optional and Computed, so setting both - or neither -
// has to be rejected explicitly rather than by the schema.
func TestAccIncidentIncidentTypeDataSourceAmbiguousLookup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_incident_type" "both" {
  id   = "01FCNDV6P870EA6S7TK1DSYDG0"
  name = "Production Outage"
}
`,
				ExpectError: regexp.MustCompile("Ambiguous lookup"),
			},
			{
				Config: `
data "incident_incident_type" "neither" {
}
`,
				ExpectError: regexp.MustCompile("Missing lookup"),
			},
		},
	})
}

// testAccIncidentType returns an incident type with a unique name from the test
// account. Unlike timestamps, an account needn't have any - incident types are
// opt-in - so this skips rather than fails when there are none.
func testAccIncidentType(t *testing.T) client.IncidentTypeV1 {
	t.Helper()

	result, err := testClient.IncidentTypesV1ListWithResponse(t.Context())
	if err != nil {
		t.Fatalf("listing incident types: %s", err)
	}
	if result.JSON200 == nil {
		t.Fatalf("listing incident types: %s", string(result.Body))
	}

	count := map[string]int{}
	for _, incidentType := range result.JSON200.IncidentTypes {
		count[incidentType.Name]++
	}
	for _, incidentType := range result.JSON200.IncidentTypes {
		if count[incidentType.Name] == 1 {
			return incidentType
		}
	}

	t.Skip("no incident type with a unique name in the test account")
	return client.IncidentTypeV1{}
}
