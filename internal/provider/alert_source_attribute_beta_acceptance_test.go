package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// accAlertSourceAttributeBeta covers the basic lifecycle of one attribute binding:
// create, import by its composite id, and update.
func accAlertSourceAttributeBeta(t *testing.T) {
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
