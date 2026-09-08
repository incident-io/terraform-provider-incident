package provider

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestAccIncidentStatusResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentStatusResourceConfig(nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_status.example", "name", incidentStatusDefault().Name),
					resource.TestCheckResourceAttr(
						"incident_status.example", "description", incidentStatusDefault().Description),
					resource.TestCheckResourceAttr(
						"incident_status.example", "category", string(incidentStatusDefault().Category)),
					resource.TestCheckResourceAttr(
						"incident_status.example", "rank", fmt.Sprintf("%d", incidentStatusDefault().Rank)),
				),
			},
			// Import. An imported status carries no rank: there's no config yet to say
			// whether Terraform manages its order, so the provider doesn't claim it.
			{
				ResourceName:            "incident_status.example",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"rank"},
			},
			// Update and read
			{
				Config: testAccIncidentStatusResourceConfig(&client.IncidentStatusV1{
					Name: StableSuffix("Clean-up"),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_status.example", "name", StableSuffix("Clean-up")),
					resource.TestCheckResourceAttr(
						"incident_status.example", "rank", fmt.Sprintf("%d", incidentStatusDefault().Rank)),
				),
			},
			// Move it to a different rank
			{
				Config: testAccIncidentStatusResourceConfig(&client.IncidentStatusV1{
					Name: StableSuffix("Clean-up"),
					Rank: stableStatusRank(5),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_status.example", "rank", fmt.Sprintf("%d", stableStatusRank(5))),
				),
			},
		},
	})
}

// TestAccIncidentStatusResourceWithoutRank covers the config that leaves ordering to us.
// rank has to stay absent from state rather than being adopted from the API: adopting it
// would plan a change against an unchanged status, and would overwrite whatever order
// someone had set in the dashboard.
func TestAccIncidentStatusResourceWithoutRank(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentStatusResourceConfig(&client.IncidentStatusV1{
					Rank: -1, // omits the attribute, see the template
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_status.example", "rank"),
				),
			},
		},
	})
}

// TestAccIncidentStatusResourceOrdering is the bug rank exists to fix: several statuses
// in one category, applied together. Terraform creates them concurrently, so without a
// rank each one asked for "the next after the last", they raced for the same rank, and
// the apply failed — or, when it didn't, left an order that varied run to run.
//
// The second step inserts a status between two of them without touching either, which is
// what sparse ranks buy: nothing has to be renumbered to make room.
func TestAccIncidentStatusResourceOrdering(t *testing.T) {
	first := statusOrdering{Key: "first", Name: "Ordered first", Rank: stableStatusRank(1)}
	second := statusOrdering{Key: "second", Name: "Ordered second", Rank: stableStatusRank(3)}
	third := statusOrdering{Key: "third", Name: "Ordered third", Rank: stableStatusRank(5)}
	inserted := statusOrdering{Key: "inserted", Name: "Ordered between", Rank: stableStatusRank(2)}

	checkRanks := func(statuses ...statusOrdering) resource.TestCheckFunc {
		checks := []resource.TestCheckFunc{}
		for _, status := range statuses {
			checks = append(checks, resource.TestCheckResourceAttr(
				fmt.Sprintf("incident_status.%s", status.Key), "rank",
				fmt.Sprintf("%d", status.Rank)))
		}

		return resource.ComposeAggregateTestCheckFunc(checks...)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentStatusOrderingConfig(first, second, third),
				Check:  checkRanks(first, second, third),
			},
			{
				Config: testAccIncidentStatusOrderingConfig(first, second, third, inserted),
				Check:  checkRanks(first, second, third, inserted),
			},
		},
	})
}

var incidentStatusTemplate = template.Must(template.New("incident_status").Funcs(testTemplateFuncs()).Parse(`
resource "incident_status" "example" {
  name         = {{ quote .Name }}
  description  = {{ quote .Description }}
  category     = {{ quote .Category }}
{{ if gt .Rank 0 }}
  rank         = {{ toJson .Rank }}
{{ end }}
}
`))

type statusOrdering struct {
	Key  string
	Name string
	Rank int64
}

var incidentStatusOrderingTemplate = template.Must(template.New("incident_status_ordering").Funcs(testTemplateFuncs()).Parse(`
{{ range . }}
resource "incident_status" "{{ .Key }}" {
  name        = {{ stableSuffix .Name | quote }}
  description = "Where this sits is what the config says, not whichever create landed first."
  category    = "live"
  rank        = {{ toJson .Rank }}
}
{{ end }}
`))

// stableStatusRank derives ranks from the test run, because ranks must be unique within a
// category and the CLI legs of a CI run create statuses concurrently. Each run gets a
// block of ten, so a test can order several statuses without straying into another run's
// block. Real statuses sit at low ranks, so we stay well clear of them.
func stableStatusRank(offset int64) int64 {
	n, err := strconv.ParseInt(testRunShortID, 16, 64)
	if err != nil {
		panic(err)
	}

	return 1000 + (n%1000)*10 + offset
}

func incidentStatusDefault() client.IncidentStatusV1 {
	return client.IncidentStatusV1{
		Name:        StableSuffix("Clean up"),
		Description: "We're cleaning up",
		Category:    client.IncidentStatusV1CategoryLive,
		Rank:        stableStatusRank(0),
	}
}

func testAccIncidentStatusResourceConfig(override *client.IncidentStatusV1) string {
	model := incidentStatusDefault()

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
	if err := incidentStatusTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccIncidentStatusOrderingConfig(statuses ...statusOrdering) string {
	var buf bytes.Buffer
	if err := incidentStatusOrderingTemplate.Execute(&buf, statuses); err != nil {
		panic(err)
	}

	return buf.String()
}
