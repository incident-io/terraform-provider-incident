package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// TestAccAlertSource covers the basic lifecycle: create, read back unchanged,
// import, and update.
func TestAccAlertSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceConfig("test-source", "a title", "a description"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", StableSuffix("test-source")),
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "http"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "title.literal", "a title"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "description.literal", "a description"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "id"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "secret_token"),
				),
			},
			{
				ResourceName:      "incident_alert_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccAlertSourceConfig("test-source-renamed", "a title", "a description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", StableSuffix("test-source-renamed")),
				),
			},
		},
	})
}

func testAccAlertSourceConfig(name, title, description string) string {
	return testRunTemplate("incident_alert_source", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "http"

  title       = { literal = {{ quote .Title }} }
  description = { literal = {{ quote .Description }} }
}
`, struct {
		Name        string
		Title       string
		Description string
	}{
		Name:        StableSuffix(name),
		Title:       title,
		Description: description,
	})
}

func testAccAlertSourceHeartbeatConfig(name string, intervalSeconds int, disabled *bool) string {
	disabledLine := ""
	if disabled != nil {
		disabledLine = fmt.Sprintf("  disabled = %t", *disabled)
	}

	return testRunTemplate("incident_alert_source_heartbeat", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "heartbeat"

  heartbeat_options = {
    interval_seconds = {{ .IntervalSeconds }}
  }
{{ .DisabledLine }}
}
`, struct {
		Name            string
		IntervalSeconds int
		DisabledLine    string
	}{
		Name:            StableSuffix(name),
		IntervalSeconds: intervalSeconds,
		DisabledLine:    disabledLine,
	})
}

// TestAccAlertSourceHeartbeatDisabled covers disabled on a heartbeat source.
//
// It never gets one paused: the API refuses to disable a heartbeat until it has received
// its first ping, and a source the test just created has received nothing. So the pause is
// the error case, which is worth an acceptance test of its own - the source is created
// before the pause is attempted, and it has to end up in state anyway or Terraform would
// lose track of a heartbeat that exists and is monitoring.
func TestAccAlertSourceHeartbeatDisabled(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccAlertSourceHeartbeatConfig("heartbeat-paused", 60, lo.ToPtr(true)),
				ExpectError: regexp.MustCompile(`(?s)Unable to (pause|update) alert source.*alert_source.disabled`),
			},
			// The source from the failed step is in state, so this adopts it rather than
			// leaving it behind: a heartbeat nobody is managing keeps monitoring, and the
			// test's cleanup wouldn't know to delete it.
			{
				Config: testAccAlertSourceHeartbeatConfig("heartbeat-paused", 60, lo.ToPtr(false)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "heartbeat"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "disabled", "false"),
				),
			},
		},
	})
}

// TestAccAlertSourcePriority covers priority = { expression_ref = ... }, which is the
// beta resource's whole reason for existing on this field: incident_alert_source has to
// smuggle priority through a binding on the built-in Priority alert attribute, and here it
// is a field.
//
// Two expressions, because the payload is opaque JSON: an expression reaches into it with
// parse, and a condition can only ask whether payload as a whole is set, so the branches
// condition tests the first expression's result rather than payload.severity.
func TestAccAlertSourcePriority(t *testing.T) {
	// The priority is resolved before resource.Test, so honour TF_ACC ourselves rather
	// than calling the API during a unit test run, then initialise testClient.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}
	testAccPreCheck(t)

	priorityID := testAccAlertPriority(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourcePriorityConfig(priorityID),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.priority", "priority.expression_ref", "priority_lookup"),
					resource.TestCheckResourceAttr("incident_alert_source.priority", "named_expression.#", "2"),
					resource.TestCheckResourceAttrSet("incident_alert_source.priority", "id"),
				),
			},
			// named_expression is ignored because it is a list, and an import cannot know
			// the order the config wrote it in. The API sorts a source's expressions by
			// reference before serializing them, and the read sorts whatever it has no
			// prior for, so an import lands on priority_lookup, severity where the config
			// says severity, priority_lookup.
			//
			// priority is not ignored: a reference reads back as the expression_ref sugar,
			// so the binding this test is about round-trips.
			{
				ResourceName:            "incident_alert_source.priority",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"named_expression"},
			},
		},
	})
}

func testAccAlertSourcePriorityConfig(priorityID string) string {
	return testRunTemplate("incident_alert_source_priority", `
resource "incident_alert_source" "priority" {
  name        = {{ stableSuffix "beta-priority" | quote }}
  source_type = "http"

  title       = { literal = "a title" }
  description = { literal = "a description" }

  named_expression {
    name       = "severity"
    start_from = "payload"

    operation {
      parse = {
        function = "$.severity"
        as       = "String"
      }
    }
  }

  named_expression {
    name       = "priority_lookup"
    start_from = "."

    operation {
      branches {
        as = "CatalogEntry[\"AlertPriority\"]"

        if {
          conditions = [{
            subject   = "expressions[\"severity\"]"
            operation = "one_of"
            params    = [{ values = ["CRITICAL"] }]
          }]
          result = { value_literal = {{ quote .PriorityID }} }
        }
      }
    }

    fallback {
      result = { value_literal = {{ quote .PriorityID }} }
    }
  }

  priority = {
    expression_ref = "priority_lookup"
  }
}
`, struct{ PriorityID string }{PriorityID: priorityID})
}

// testAccAlertPriority returns an alert priority from the test account.
//
// The config used to reach for the plural catalog entries data source and index into
// it, which is the shape that data source is being removed for: whichever entry came
// back first is not a thing a configuration can name. Resolving it here instead keeps
// the test working against any organisation - which priorities an account has is its
// own business, and this test only cares that the binding round-trips - without a
// configuration that depends on the API's ordering.
func testAccAlertPriority(t *testing.T) string {
	t.Helper()

	// No params argument here: everything after the context is a request editor, so a
	// nil would be a nil editor that the client then calls.
	types, err := testClient.CatalogV3ListTypesWithResponse(t.Context())
	if err != nil {
		t.Fatalf("listing catalog types: %s", err)
	}
	if types.JSON200 == nil {
		t.Fatalf("listing catalog types: %s", string(types.Body))
	}

	priorityType, found := lo.Find(types.JSON200.CatalogTypes, func(catalogType client.CatalogTypeV3) bool {
		return catalogType.TypeName == "AlertPriority"
	})
	if !found {
		t.Skip("the test account has no AlertPriority catalog type")
	}

	entries, err := testClient.CatalogV3ListEntriesWithResponse(t.Context(), &client.CatalogV3ListEntriesParams{
		CatalogTypeId: priorityType.Id,
		PageSize:      250,
	})
	if err != nil {
		t.Fatalf("listing alert priorities: %s", err)
	}
	if entries.JSON200 == nil {
		t.Fatalf("listing alert priorities: %s", string(entries.Body))
	}
	if len(entries.JSON200.CatalogEntries) == 0 {
		t.Skip("the test account has no alert priorities")
	}

	return entries.JSON200.CatalogEntries[0].Id
}
