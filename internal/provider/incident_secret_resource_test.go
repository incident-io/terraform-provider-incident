package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccIncidentSecretResource covers the whole life of a secret: created with a value,
// rotated, renamed, and imported.
//
// The API key needs the "secrets_manage" role, which grants the view, create, update,
// rotate and delete scopes the resource uses.
//
// value_wo is a write-only attribute, so this needs Terraform 1.11 or above. There's
// nothing to assert about the value itself - it's never returned - so a rotation is
// observed through the version and the masked last four characters, which is all
// incident.io will say about it.
func TestAccIncidentSecretResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentSecretResourceConfig(secretTestConfig{
					Description: "Auth token for the PagerDuty outgoing webhook",
					Value:       "sk_live_abc123",
					Version:     1,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "description", "Auth token for the PagerDuty outgoing webhook"),
					resource.TestCheckResourceAttr("incident_secret.test", "version", "1"),
					resource.TestCheckResourceAttr("incident_secret.test", "last_four_chars", "c123"),
					resource.TestCheckResourceAttr("incident_secret.test", "value_wo_version", "1"),
					resource.TestCheckResourceAttr("incident_secret.test", "owning_team_ids.#", "0"),
					resource.TestCheckResourceAttrSet("incident_secret.test", "id"),
					// A write-only attribute is never persisted, whatever the config said.
					resource.TestCheckNoResourceAttr("incident_secret.test", "value_wo"),
					// Both lookups find the same secret, and neither can report its value.
					resource.TestCheckResourceAttrPair(
						"data.incident_secret.by_id", "id",
						"incident_secret.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.incident_secret.by_name", "id",
						"incident_secret.test", "id",
					),
					resource.TestCheckResourceAttr("data.incident_secret.by_name", "last_four_chars", "c123"),
				),
			},
			{
				ResourceName:      "incident_secret.test",
				ImportState:       true,
				ImportStateVerify: true,
				// An imported secret has no value_wo_version: the value it holds didn't come
				// from this configuration, so there's no version of it to record.
				ImportStateVerifyIgnore: []string{"value_wo_version"},
			},
			{
				// Rotating: a new value, and the version bumped to say so.
				Config: testAccIncidentSecretResourceConfig(secretTestConfig{
					Description: "Auth token for the PagerDuty outgoing webhook",
					Value:       "sk_live_def456",
					Version:     2,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "version", "2"),
					resource.TestCheckResourceAttr("incident_secret.test", "last_four_chars", "f456"),
					resource.TestCheckResourceAttr("incident_secret.test", "value_wo_version", "2"),
				),
			},
			{
				// A new value without a new version rotates nothing. Terraform can't see a
				// write-only attribute change, so this is a no-op plan - which is exactly why
				// value_wo_version exists.
				Config: testAccIncidentSecretResourceConfig(secretTestConfig{
					Description: "Auth token for the PagerDuty outgoing webhook",
					Value:       "sk_live_ghi789",
					Version:     2,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "version", "2"),
					resource.TestCheckResourceAttr("incident_secret.test", "last_four_chars", "f456"),
				),
			},
			{
				// Metadata alone: the description changes, the value stays where it was.
				Config: testAccIncidentSecretResourceConfig(secretTestConfig{
					Description: "Rotated by hand, sorry",
					Value:       "sk_live_ghi789",
					Version:     2,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "description", "Rotated by hand, sorry"),
					resource.TestCheckResourceAttr("incident_secret.test", "version", "2"),
					resource.TestCheckResourceAttr("incident_secret.test", "last_four_chars", "f456"),
				),
			},
			{
				// Dropping the description clears it, rather than leaving the old one in
				// place: the API reads an omitted description as "leave unchanged", so the
				// provider has to send an empty one.
				Config: testAccIncidentSecretResourceConfig(secretTestConfig{
					Value:   "sk_live_ghi789",
					Version: 2,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_secret.test", "description"),
					resource.TestCheckResourceAttr("incident_secret.test", "version", "2"),
				),
			},
		},
	})
}

// TestAccIncidentSecretResourceMetadataOnly manages a secret whose value Terraform never
// owns: no value_wo at all, which is how you adopt a secret something else rotates.
func TestAccIncidentSecretResourceMetadataOnly(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				// Creating one still needs a value: a secret is created with its first
				// version, so there's no such thing as a secret without a value.
				Config: testAccIncidentSecretResourceConfig(secretTestConfig{
					Name:    "metadata-only",
					Value:   "sk_live_abc123",
					Version: 1,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "version", "1"),
				),
			},
			{
				// With both value attributes gone, the secret's value is left alone and only
				// its metadata is managed.
				Config: testAccIncidentSecretResourceConfig(secretTestConfig{
					Name:        "metadata-only",
					Description: "Rotated nightly by our key-rotation job",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "description", "Rotated nightly by our key-rotation job"),
					resource.TestCheckResourceAttr("incident_secret.test", "version", "1"),
					resource.TestCheckResourceAttr("incident_secret.test", "last_four_chars", "c123"),
					resource.TestCheckNoResourceAttr("incident_secret.test", "value_wo_version"),
				),
			},
		},
	})
}

// TestAccIncidentSecretResourceVersionWithoutValue checks the plan-time error for a
// rotation that has nothing to rotate to.
func TestAccIncidentSecretResourceVersionWithoutValue(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("incident_secret_version_only", `
resource "incident_secret" "test" {
  name             = {{ stableSuffix "TF version only" | quote }}
  value_wo_version = 1
}
`, nil),
				ExpectError: regexp.MustCompile(`value_wo_version is set but value_wo is not`),
			},
		},
	})
}

// TestAccIncidentSecretResourceOwningTeams assigns a secret to a team and then takes it
// away again. Removing the attribute is the interesting half: the API reads an omitted
// owning_team_ids as "leave unchanged", so the provider has to send an empty list for a
// config that dropped the team to mean anything.
func TestAccIncidentSecretResourceOwningTeams(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentSecretOwningTeamsConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "owning_team_ids.#", "1"),
					resource.TestCheckResourceAttrPair(
						"incident_secret.test", "owning_team_ids.0",
						"incident_catalog_entry.secret_team", "id",
					),
					resource.TestCheckResourceAttr("data.incident_secret.by_id", "owning_team_ids.#", "1"),
				),
			},
			{
				Config: testAccIncidentSecretOwningTeamsConfig(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_secret.test", "owning_team_ids.#", "0"),
				),
			},
		},
	})
}

func testAccIncidentSecretOwningTeamsConfig(owned bool) string {
	return testRunTemplate("incident_secret_owning_teams", `
data "incident_catalog_type" "team" {
  name = {{ .TeamTypeName | quote }}
}

resource "incident_catalog_entry" "secret_team" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = {{ .TeamName | quote }}
  name               = {{ .TeamName | quote }}
  attribute_values   = []
  managed_attributes = []
}

resource "incident_secret" "test" {
  name             = {{ .Name | quote }}
  value_wo         = "sk_live_abc123"
  value_wo_version = 1
  {{ if .Owned }}owning_team_ids = [incident_catalog_entry.secret_team.id]{{ end }}
}

data "incident_secret" "by_id" {
  id = incident_secret.test.id
}
`, struct {
		TeamTypeName string
		TeamName     string
		Name         string
		Owned        bool
	}{
		TeamTypeName: teamTypeName(),
		TeamName:     StableSuffix("tf-acceptance-test-secret-team"),
		Name:         StableSuffix("TF test secret owning teams"),
		Owned:        owned,
	})
}

// secretTestConfig is the config each step renders. Name is a suffix so one test's
// secrets don't collide with another's, on top of the per-run suffix: a secret's name is
// unique across the organisation, and these run against a shared account.
type secretTestConfig struct {
	Name        string
	Description string
	Value       string
	Version     int64
}

func testAccIncidentSecretResourceConfig(config secretTestConfig) string {
	name := "TF test secret"
	if config.Name != "" {
		name = fmt.Sprintf("TF test secret %s", config.Name)
	}
	config.Name = StableSuffix(name)

	return testRunTemplate("incident_secret", `
resource "incident_secret" "test" {
  name = {{ .Name | quote }}
  {{ if .Description }}description = {{ .Description | quote }}{{ end }}
  {{ if .Value }}value_wo = {{ .Value | quote }}{{ end }}
  {{ if .Version }}value_wo_version = {{ .Version }}{{ end }}
}

data "incident_secret" "by_id" {
  id = incident_secret.test.id
}

data "incident_secret" "by_name" {
  name = incident_secret.test.name
}
`, config)
}
