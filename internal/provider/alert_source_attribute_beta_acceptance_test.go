package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccAlertSourceAttributeBeta covers the basic lifecycle of one attribute binding:
// create, import by its composite id, and update.
func TestAccAlertSourceAttributeBeta(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceAttributeBetaConfig("test-attribute-binding", "production"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("incident_alert_source_attribute_beta.test", "alert_source_id", "incident_alert_source_beta.test", "id"),
					resource.TestCheckResourceAttrPair("incident_alert_source_attribute_beta.test", "alert_attribute_id", "incident_alert_attribute.test", "id"),
					resource.TestCheckResourceAttr("incident_alert_source_attribute_beta.test", "value_literal", "production"),
					resource.TestCheckResourceAttrSet("incident_alert_source_attribute_beta.test", "merge_strategy"),
				),
			},
			{
				ResourceName:      "incident_alert_source_attribute_beta.test",
				ImportState:       true,
				ImportStateVerify: true,
				// There is no id: the pair of ids is the identity, so ImportStateVerify needs
				// telling which attribute to correlate the imported and pre-import state by.
				ImportStateVerifyIdentifierAttribute: "alert_attribute_id",
				ImportStateIdFunc:                    importAlertSourceAttributeBetaStateIDFunc("incident_alert_source_attribute_beta.test"),
			},
			{
				Config: testAccAlertSourceAttributeBetaConfig("test-attribute-binding", "staging"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_attribute_beta.test", "value_literal", "staging"),
				),
			},
		},
	})
}

// TestAccAlertSourceAttributeBetaNavigate covers a navigate operation end to end, which
// nothing else does. The API addresses a catalog entry's attributes as
// catalog_attribute["<id>"], so the provider wraps the bare attribute ID `to` takes and
// unwraps it on read: send the ID unwrapped and the apply fails with "Reference not found in
// scope", and skip the unwrapping and every plan shows a diff against the wrapped form the
// API returns. Both halves are asserted here — the apply, and `to` reading back as the bare
// ID it was written as.
func TestAccAlertSourceAttributeBetaNavigate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceAttributeBetaNavigateConfig("test-navigate"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The wrapping round-trips: state holds the bare attribute ID.
					resource.TestCheckResourceAttrPair(
						"incident_alert_source_attribute_beta.test", "expression.operation.1.navigate.to",
						"incident_catalog_type_attribute.owner", "id"),
					resource.TestCheckResourceAttr(
						"incident_alert_source_attribute_beta.test", "expression.start_from", "payload"),
				),
			},
			// A second apply of the same config must be a no-op. resource.Test fails on a
			// non-empty plan after each step, so this is the drift guard for the read path.
			{
				Config:   testAccAlertSourceAttributeBetaNavigateConfig("test-navigate"),
				PlanOnly: true,
			},
		},
	})
}

// testAccAlertSourceAttributeBetaNavigateConfig builds a Service type whose Owner attribute
// points at a Team type, then an expression that parses a service out of the payload and
// navigates to its owners. Owner is an array because the pipeline ends with first {}, which
// the API rejects over a scalar.
func testAccAlertSourceAttributeBetaNavigateConfig(name string) string {
	return testRunTemplate("incident_alert_source_attribute_beta_navigate", `
resource "incident_catalog_type" "team" {
  name            = "{{ .Name }} team"
  description     = "Used in terraform acceptance tests"
  source_repo_url = ""
}

resource "incident_catalog_type" "service" {
  name            = "{{ .Name }} service"
  description     = "Used in terraform acceptance tests"
  source_repo_url = ""
}

resource "incident_catalog_type_attribute" "owner" {
  catalog_type_id = incident_catalog_type.service.id
  name            = "Owner"
  type            = incident_catalog_type.team.type_name
  array           = true
}

resource "incident_alert_source_beta" "test" {
  name        = {{ quote .Name }}
  source_type = "http"

  title       = { literal = "a title" }
  description = { literal = "a description" }
}

resource "incident_alert_attribute" "test" {
  name  = {{ quote .Name }}
  type  = incident_catalog_type.team.attribute_type
  array = false
}

resource "incident_alert_source_attribute_beta" "test" {
  alert_source_id    = incident_alert_source_beta.test.id
  alert_attribute_id = incident_alert_attribute.test.id

  expression {
    start_from = "payload"

    operation {
      parse = {
        function = "$.service"
        as       = incident_catalog_type.service.attribute_type
      }
    }

    // The operation under test: follow the parsed service's Owner attribute.
    operation {
      navigate = {
        to = incident_catalog_type_attribute.owner.id
      }
    }

    operation { first = {} }
  }
}
`, struct {
		Name string
	}{
		Name: StableSuffix(name),
	})
}

func testAccAlertSourceAttributeBetaConfig(name, value string) string {
	return testRunTemplate("incident_alert_source_attribute_beta", `
resource "incident_alert_source_beta" "test" {
  name        = {{ quote .Name }}
  source_type = "http"

  title       = { literal = "a title" }
  description = { literal = "a description" }
}

resource "incident_alert_attribute" "test" {
  name  = {{ quote .Name }}
  type  = "String"
  array = false
}

resource "incident_alert_source_attribute_beta" "test" {
  alert_source_id    = incident_alert_source_beta.test.id
  alert_attribute_id = incident_alert_attribute.test.id

  value_literal = {{ quote .Value }}
}
`, struct {
		Name  string
		Value string
	}{
		Name:  StableSuffix(name),
		Value: value,
	})
}

// importAlertSourceAttributeBetaStateIDFunc returns a function that generates the composite
// import ID: an attribute binding has no id of its own, so the pair of ids that identify it
// has to be read out of state.
func importAlertSourceAttributeBetaStateIDFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", nil
		}
		return rs.Primary.Attributes["alert_source_id"] + ":" + rs.Primary.Attributes["alert_attribute_id"], nil
	}
}
