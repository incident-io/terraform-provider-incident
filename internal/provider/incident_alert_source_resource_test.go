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
		},
	})
}

const (
	testAlertSourceTitle       = `{"content":[{"content":[{"attrs":{"label":"Payload → Title","missing":false,"name":"title"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`
	testAlertSourceDescription = `{"content":[{"content":[{"attrs":{"label":"Payload → Description","missing":false,"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`
)
