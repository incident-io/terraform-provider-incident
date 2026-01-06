package provider

import (
	"bytes"
	"os"
	"regexp"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAlertSourceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccAlertSourceResourceConfig("test-source", "datadog"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "datadog"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "id"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "secret_token"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing
			{
				Config: testAccAlertSourceResourceConfig("updated-source", "datadog"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "updated-source"),
				),
			},
			// Test full configuration with template
			{
				Config: testAccAlertSourceResourceConfigFull(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "full-test-source"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "template.title.literal"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "template.description.literal"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "template.attributes.#", "1"),
					resource.TestCheckResourceAttrPair("incident_alert_source.test", "template.attributes.0.alert_attribute_id", "incident_alert_attribute.test", "id"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "template.attributes.0.binding.value.reference", `expressions["severity_expr"]`),
				),
			},
		},
	})
}

func testRunTemplate(tmplName, source string, args any) string {
	tmpl := template.Must(template.New(tmplName).Funcs(sprig.TxtFuncMap()).Parse(source))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, args)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func testAccAlertSourceResourceConfig(name string, sourceType string) string {
	return testRunTemplate("incident_alert_source", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = {{ quote .SourceType }}

  template = {
    expressions = [],
    title = {
      literal = {{ quote .Title }}
    },
    description = {
      literal = {{ quote .Description }}
    },
    attributes = []
  }
}
`, struct {
		Name, SourceType, Title, Description string
	}{
		Name:        name,
		SourceType:  sourceType,
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

func testAccAlertSourceResourceConfigWithJira(name string, projectIDs []string) string {
	return testRunTemplate("incident_alert_source_jira", `
resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "jira"

  template = {
    expressions = [],
    title = {
      literal = {{ quote .Title }}
    },
    description = {
      literal = {{ quote .Description }}
    },
    attributes = []
  }

  jira_options = {
    project_ids = [
      {{ range .ProjectIDs }}
      {{ quote . }},
      {{ end }}
    ]
  }
}
`, struct {
		Name        string
		Title       string
		Description string
		ProjectIDs  []string
	}{
		Name:        name,
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
		ProjectIDs:  projectIDs,
	})
}

func testAccAlertSourceResourceConfigFull() string {
	return testRunTemplate("incident_alert_source_full", `
resource "incident_alert_attribute" "test" {
  name = "test-attribute"
  type = "String"
  array = false
}

resource "incident_alert_source" "test" {
  name        = "full-test-source"
  source_type = "datadog"

  template = {
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = [{
      alert_attribute_id = incident_alert_attribute.test.id
      binding = {
        value = {
          reference = "expressions[\"severity_expr\"]"
        }
      }
    }]

    expressions = [{
      label = "Severity"
      reference = "severity_expr"
      root_reference = "payload"
      operations = [{
        operation_type = "parse"
        parse = {
          source = "$.metadata.severity"
          returns = {
            type  = "String"
            array = false
          }
        }
      }]
    }]
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

// TestAccAlertSourceResource_Jira checks that the jira_options work.
//
// NOTE: this only runs if TF_ACC_JIRA is in your environment, since it requires
// the Jira integration to be installed in the target account.
func TestAccAlertSourceResource_Jira(t *testing.T) {
	if os.Getenv("TF_ACC_JIRA") == "" {
		t.Skip("TF_ACC_JIRA is not set: skipping Jira-specific test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{

			// Add a Jira one
			{
				// This is the default project in our dev account
				Config: testAccAlertSourceResourceConfigWithJira("jira-source", []string{"46a0db2b-17d4-48c1-961e-563d87797b5c/10000"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "jira-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "jira_options.project_ids.#", "1"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "jira_options.project_ids.0", "46a0db2b-17d4-48c1-961e-563d87797b5c/10000"),
				),
			},
		},
	})
}

// TestAccAlertSourceResource_ValidationErrors checks that we return helpful
// validation errors when possible.
func TestAccAlertSourceResource_ValidationErrors(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Test
				Config: testRunTemplate("incident_alert_source_invalid", `
resource "incident_alert_source" "test" {
  name = "Not Jira, but with Jira options"
  source_type = "datadog"

  template = {
    expressions = [],
    title = {
      literal = {{ quote .Title }}
    },
    description = {
      literal = {{ quote .Description }}
    },
    attributes = []
  }

  jira_options = {
    project_ids = ["my-project"]
  }
}
`, struct{ Title, Description string }{
					Title:       testAlertSourceTitle,
					Description: testAlertSourceDescription,
				}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("jira_options can only be set when source_type is jira"),
			},
			{
				// Test missing required template fields
				Config: testRunTemplate("incident_alert_source_invalid", `
resource "incident_alert_source" "test" {
  name        = "test-source"
  source_type = "datadog"
  template = {
    # Missing required title
    description = {
      literal = {{ quote .Description }}
    }
  }
}
`, struct{ Description string }{Description: testAlertSourceDescription}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("required"),
			},
			{
				// Test visible_to_teams without is_private=true
				Config:      testAccAlertSourceResourceConfigVisibleToTeamsWithoutPrivate(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("visible_to_teams can only be set when is_private is true"),
			},
			{
				// Test is_private=true without visible_to_teams
				Config:      testAccAlertSourceResourceConfigPrivateWithoutTeams(),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("visible_to_teams must be set when is_private is true"),
			},
		},
	})
}

const (
	testAlertSourceTitle       = `{"content":[{"content":[{"attrs":{"label":"Payload → Title","missing":false,"name":"title"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`
	testAlertSourceDescription = `{"content":[{"content":[{"attrs":{"label":"Payload → Description","missing":false,"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`
)

func TestAccAlertSourceResource_DynamicAttributes(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Test dynamic attributes
			{
				Config: testAccAlertSourceResourceConfigDynamicAttributes(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.dynamic_alert_source", "name", "tf-dynamic-alert-source"),
					resource.TestCheckResourceAttr("incident_alert_source.dynamic_alert_source", "source_type", "http"),
					// Verify we have 2 attributes
					resource.TestCheckResourceAttr("incident_alert_source.dynamic_alert_source", "template.attributes.#", "2"),
				),
			},
		},
	})
}

func testAccAlertSourceResourceConfigDynamicAttributes() string {
	return testRunTemplate("incident_alert_source_dynamic_attributes", `
# Create alert attributes directly
resource "incident_alert_attribute" "team" {
  name  = "team-tf-attr"
  type  = "String"
  array = false
}

resource "incident_alert_attribute" "feature" {
  name  = "feature-tf-attr"
  type  = "String"
  array = false
}

locals {
	with_conds = true
}

# Use those attributes in an alert source
resource "incident_alert_source" "dynamic_alert_source" {
  name        = "tf-dynamic-alert-source"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }

    # Use a simple attribute list without dynamic references
    attributes = local.with_conds ? [
      {
        alert_attribute_id = incident_alert_attribute.team.id
        binding = {
          value = {
            literal = "team-value"
          }
        }
      },
      {
        alert_attribute_id = incident_alert_attribute.feature.id
        binding = {
          value = {
            literal = "feature-value"
          }
        }
      }
    ] : []
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

func testAccAlertSourceResourceConfigVisibleToTeamsWithoutPrivate() string {
	return testRunTemplate("incident_alert_source_visible_to_teams_without_private", `
resource "incident_alert_source" "test" {
  name        = "test-source-invalid"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
    visible_to_teams = {
      array_value = [{ literal = "some-team-id" }]
    }
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

func testAccAlertSourceResourceConfigPrivateWithoutTeams() string {
	return testRunTemplate("incident_alert_source_private_without_teams", `
resource "incident_alert_source" "test" {
  name        = "test-source-invalid"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
    is_private = true
  }
}
`, struct {
		Title, Description string
	}{
		Title:       testAlertSourceTitle,
		Description: testAlertSourceDescription,
	})
}

// TestAccAlertSourceResource_Private checks that privacy settings work correctly.
func TestAccAlertSourceResource_Private(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a private alert source
			{
				Config: testAccAlertSourceResourceConfigPrivate(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "private-alert-source"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "source_type", "http"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "template.is_private", "true"),
					resource.TestCheckResourceAttrSet("incident_alert_source.test", "template.visible_to_teams.array_value.0.literal"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccAlertSourceResourceConfigPrivate() string {
	return testRunTemplate("incident_alert_source_private", `
# Look up the Team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# Create a team catalog entry for this test
resource "incident_catalog_entry" "test_team" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-privacy-test"
  name            = "Terraform Alert Source Privacy Test Team"
  attribute_values = []
}

resource "incident_alert_source" "test" {
  name        = "private-alert-source"
  source_type = "http"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
    is_private = true
    visible_to_teams = {
      array_value = [{ literal = incident_catalog_entry.test_team.id }]
    }
  }
}
`, struct {
		Title, Description, TeamTypeName string
	}{
		Title:        testAlertSourceTitle,
		Description:  testAlertSourceDescription,
		TeamTypeName: teamTypeName(),
	})
}

// TestAccAlertSourceResource_OwningTeamIDs checks that owning_team_ids work correctly.
func TestAccAlertSourceResourceOwningTeamIDs(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create without owning_team_ids
			{
				Config: testAccAlertSourceResourceConfig("test-source-no-teams", "datadog"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source-no-teams"),
					resource.TestCheckNoResourceAttr("incident_alert_source.test", "owning_team_ids"),
				),
			},
			// Update to add owning_team_ids
			{
				Config: testAccAlertSourceResourceConfigWithOwningTeamIDs("test-source-with-teams"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source-with-teams"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "owning_team_ids.#", "1"),
					resource.TestCheckResourceAttrPair("incident_alert_source.test", "owning_team_ids.0", "incident_catalog_entry.owner_team", "id"),
				),
			},
			// Update to change the team
			{
				Config: testAccAlertSourceResourceConfigWithDifferentOwningTeamIDs("test-source-updated-teams"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.test", "name", "test-source-updated-teams"),
					resource.TestCheckResourceAttr("incident_alert_source.test", "owning_team_ids.#", "2"),
				),
			},
		},
	})
}

func testAccAlertSourceResourceConfigWithOwningTeamIDs(name string) string {
	return testRunTemplate("incident_alert_source_with_owning_teams", `
# Look up the Team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# Create a team catalog entry for this test
resource "incident_catalog_entry" "owner_team" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-owning-team-test"
  name            = "Terraform Alert Source Owning Team Test"
  attribute_values = []
}

resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "datadog"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
  }

  owning_team_ids = [incident_catalog_entry.owner_team.id]
}
`, struct {
		Name, Title, Description, TeamTypeName string
	}{
		Name:         name,
		Title:        testAlertSourceTitle,
		Description:  testAlertSourceDescription,
		TeamTypeName: teamTypeName(),
	})
}

func testAccAlertSourceResourceConfigWithDifferentOwningTeamIDs(name string) string {
	return testRunTemplate("incident_alert_source_with_different_owning_teams", `
# Look up the Team catalog type
data "incident_catalog_type" "team" {
  name = {{ quote .TeamTypeName }}
}

# Create team catalog entries for this test
resource "incident_catalog_entry" "owner_team" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-owning-team-test"
  name            = "Terraform Alert Source Owning Team Test"
  attribute_values = []
}

resource "incident_catalog_entry" "owner_team_2" {
  catalog_type_id = data.incident_catalog_type.team.id
  external_id     = "tf-alert-source-owning-team-test-2"
  name            = "Terraform Alert Source Owning Team Test 2"
  attribute_values = []
}

resource "incident_alert_source" "test" {
  name        = {{ quote .Name }}
  source_type = "datadog"

  template = {
    expressions = []
    title = {
      literal = {{ quote .Title }}
    }
    description = {
      literal = {{ quote .Description }}
    }
    attributes = []
  }

  owning_team_ids = [
    incident_catalog_entry.owner_team.id,
    incident_catalog_entry.owner_team_2.id
  ]
}
`, struct {
		Name, Title, Description, TeamTypeName string
	}{
		Name:         name,
		Title:        testAlertSourceTitle,
		Description:  testAlertSourceDescription,
		TeamTypeName: teamTypeName(),
	})
}
