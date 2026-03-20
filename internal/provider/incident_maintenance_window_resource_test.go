package provider

import (
	"bytes"
	"os"
	"regexp"
	"testing"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func testAccMaintenanceWindowLeadUserID() string {
	if id := os.Getenv("TF_ACC_LEAD_USER_ID"); id != "" {
		return id
	}
	// Default to a known user ID in the integration test workspace.
	return "01JPDX7TN2Y3FS8DG3N1G8G0ZX"
}

func TestAccIncidentMaintenanceWindowResource(t *testing.T) {
	model := maintenanceWindowDefault()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccMaintenanceWindowResourceConfig(model),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(
						"incident_maintenance_window.example", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "name", model.Name),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "start_at", model.StartAt),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "end_at", model.EndAt),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "show_in_sidebar", "true"),
					resource.TestCheckResourceAttrSet(
						"incident_maintenance_window.example", "lead_id"),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "alert_condition_groups.0.conditions.0.subject", "alert.title"),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "alert_condition_groups.0.conditions.0.operation", "contains"),
				),
			},
			// Ensure no drift after refresh
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Import
			{
				ResourceName:      "incident_maintenance_window.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and read
			{
				Config: testAccMaintenanceWindowResourceConfig(maintenanceWindowModel{
					Name:          "Updated Maintenance Window",
					StartAt:       model.StartAt,
					EndAt:         model.EndAt,
					LeadUserID:    model.LeadUserID,
					ShowInSidebar: false,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "name", "Updated Maintenance Window"),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "show_in_sidebar", "false"),
				),
			},
		},
	})
}

func TestAccIncidentMaintenanceWindowResourceWithOptionalFields(t *testing.T) {
	model := maintenanceWindowDefault()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with optional fields
			{
				Config: testAccMaintenanceWindowResourceConfigWithOptionalFields(model),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "name", model.Name),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "resolve_on_end", "true"),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "reroute_on_end", "false"),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "notification_message", "Planned maintenance in progress"),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "notify_start_minutes_before", "15"),
					resource.TestCheckResourceAttr(
						"incident_maintenance_window.example", "notify_end_minutes_before", "5"),
				),
			},
			// Ensure no drift after refresh
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Import
			{
				ResourceName:      "incident_maintenance_window.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccIncidentMaintenanceWindowResourceInvalidTimestamp(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "incident_maintenance_window" "invalid" {
  name     = "invalid-timestamp-test"
  start_at = "2026-13-01T02:00:00Z"
  end_at   = "2026-04-01T06:00:00Z"
  lead_id  = "01EXAMPLE"

  alert_condition_groups = [
    {
      conditions = [
        {
          subject   = "alert.title"
          operation = "contains"
          param_bindings = [
            {
              value = {
                literal = "test"
              }
            }
          ]
        }
      ]
    }
  ]
}`,
				ExpectError: regexp.MustCompile("Invalid Timestamp Format"),
			},
		},
	})
}

type maintenanceWindowModel struct {
	Name          string
	StartAt       string
	EndAt         string
	LeadUserID    string
	ShowInSidebar bool
}

var maintenanceWindowTemplate = template.Must(template.New("incident_maintenance_window").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_maintenance_window" "example" {
  name            = {{ quote .Name }}
  start_at        = {{ quote .StartAt }}
  end_at          = {{ quote .EndAt }}
  lead_id         = {{ quote .LeadUserID }}
  show_in_sidebar = {{ .ShowInSidebar }}

  alert_condition_groups = [
    {
      conditions = [
        {
          subject   = "alert.title"
          operation = "contains"
          param_bindings = [
            {
              value = {
                literal = "test"
              }
            }
          ]
        }
      ]
    }
  ]
}
`))

var maintenanceWindowWithOptionalFieldsTemplate = template.Must(template.New("incident_maintenance_window_optional").Funcs(sprig.TxtFuncMap()).Parse(`
resource "incident_maintenance_window" "example" {
  name            = {{ quote .Name }}
  start_at        = {{ quote .StartAt }}
  end_at          = {{ quote .EndAt }}
  lead_id         = {{ quote .LeadUserID }}
  show_in_sidebar = true

  resolve_on_end  = true
  reroute_on_end  = false

  notification_message        = "Planned maintenance in progress"
  notify_start_minutes_before = 15
  notify_end_minutes_before   = 5

  alert_condition_groups = [
    {
      conditions = [
        {
          subject   = "alert.title"
          operation = "contains"
          param_bindings = [
            {
              value = {
                literal = "test"
              }
            }
          ]
        }
      ]
    }
  ]
}
`))

func maintenanceWindowDefault() maintenanceWindowModel {
	// Use dates in the future to avoid issues with validation
	startAt := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC().Format(time.RFC3339)
	endAt := time.Now().Add(28 * time.Hour).Truncate(time.Second).UTC().Format(time.RFC3339)

	return maintenanceWindowModel{
		Name:          "Test Maintenance Window",
		StartAt:       startAt,
		EndAt:         endAt,
		LeadUserID:    testAccMaintenanceWindowLeadUserID(),
		ShowInSidebar: true,
	}
}

func testAccMaintenanceWindowResourceConfig(model maintenanceWindowModel) string {
	var buf bytes.Buffer
	if err := maintenanceWindowTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}

func testAccMaintenanceWindowResourceConfigWithOptionalFields(model maintenanceWindowModel) string {
	var buf bytes.Buffer
	if err := maintenanceWindowWithOptionalFieldsTemplate.Execute(&buf, model); err != nil {
		panic(err)
	}

	return buf.String()
}
