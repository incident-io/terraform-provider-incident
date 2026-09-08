package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"
)

// TestAccIncidentAPIKeyResource covers the whole life of an API key: created with a token,
// rotated, edited without being rotated, and imported.
//
// The API key running these tests needs the "api_keys_manage" role to reach the endpoints
// at all, and - because a key can only grant roles whose scopes are a subset of its own -
// every role these configs assign: "viewer" and "catalog_viewer".
func TestAccIncidentAPIKeyResource(t *testing.T) {
	// The token is only returned when incident.io issues one, so the way to tell a rotation
	// happened is to watch the stored value change. These hold it between steps.
	var createdToken, rotatedToken string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAPIKeyResourceConfig(apiKeyTestConfig{
					Comments: "Requested in #ask-infra",
					Roles:    []string{"viewer"},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_api_key.test", "comments", "Requested in #ask-infra"),
					resource.TestCheckResourceAttr("incident_api_key.test", "role_names.#", "1"),
					resource.TestCheckTypeSetElemAttr("incident_api_key.test", "role_names.*", "viewer"),
					resource.TestCheckResourceAttr("incident_api_key.test", "team_ids.#", "0"),
					resource.TestCheckResourceAttr("incident_api_key.test", "team_role_names.#", "0"),
					resource.TestCheckResourceAttrSet("incident_api_key.test", "id"),
					resource.TestCheckResourceAttrSet("incident_api_key.test", "created_at"),
					resource.TestCheckResourceAttrSet("incident_api_key.test", "token_last_issued_at"),

					// Creating a key issues a token, which is the only time this one is
					// available. The default grace period applies even though nothing has
					// rotated yet.
					resource.TestCheckResourceAttrSet("incident_api_key.test", "token"),
					resource.TestCheckResourceAttr("incident_api_key.test", "rotation_grace_period_minutes", "30"),
					captureAPIKeyAttr("incident_api_key.test", "token", &createdToken),

					// Both lookups find the same key, and neither can report its token.
					resource.TestCheckResourceAttrPair(
						"data.incident_api_key.by_id", "id",
						"incident_api_key.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.incident_api_key.by_name", "id",
						"incident_api_key.test", "id",
					),
					resource.TestCheckTypeSetElemAttr("data.incident_api_key.by_name", "role_names.*", "viewer"),
				),
			},
			{
				ResourceName:      "incident_api_key.test",
				ImportState:       true,
				ImportStateVerify: true,
				// An imported key has no token: the one it was issued with went to whoever
				// created it, and no read can recover it.
				ImportStateVerifyIgnore: []string{"token"},
			},
			{
				// Editing a key without touching token_version must not rotate it. The token
				// outlives its permissions, so narrowing the roles leaves it working.
				Config: testAccIncidentAPIKeyResourceConfig(apiKeyTestConfig{
					Comments: "Now also reads the catalog",
					Roles:    []string{"viewer", "catalog_viewer"},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_api_key.test", "comments", "Now also reads the catalog"),
					resource.TestCheckResourceAttr("incident_api_key.test", "role_names.#", "2"),
					resource.TestCheckTypeSetElemAttr("incident_api_key.test", "role_names.*", "catalog_viewer"),
					checkAPIKeyAttrEquals("incident_api_key.test", "token", &createdToken),
				),
			},
			{
				// Rotating: the version bumped is what asks for it, and the token that comes
				// back is a different one.
				Config: testAccIncidentAPIKeyResourceConfig(apiKeyTestConfig{
					Comments:     "Now also reads the catalog",
					Roles:        []string{"viewer", "catalog_viewer"},
					TokenVersion: 2,
					GracePeriod:  lo.ToPtr(int64(0)),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_api_key.test", "token_version", "2"),
					resource.TestCheckResourceAttr("incident_api_key.test", "rotation_grace_period_minutes", "0"),
					resource.TestCheckResourceAttrSet("incident_api_key.test", "token"),
					checkAPIKeyAttrDiffers("incident_api_key.test", "token", &createdToken),
					captureAPIKeyAttr("incident_api_key.test", "token", &rotatedToken),
				),
			},
			{
				// The same version rotates nothing, however many times it's applied.
				Config: testAccIncidentAPIKeyResourceConfig(apiKeyTestConfig{
					Comments:     "Steady as she goes",
					Roles:        []string{"viewer", "catalog_viewer"},
					TokenVersion: 2,
					GracePeriod:  lo.ToPtr(int64(0)),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_api_key.test", "comments", "Steady as she goes"),
					checkAPIKeyAttrEquals("incident_api_key.test", "token", &rotatedToken),
				),
			},
			{
				// Dropping the comments clears them, rather than leaving the old ones in
				// place: the API reads an omitted field as "leave unchanged", so the provider
				// has to send an empty one. Dropping token_version hands the token back
				// without rotating it.
				Config: testAccIncidentAPIKeyResourceConfig(apiKeyTestConfig{
					Roles: []string{"viewer"},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_api_key.test", "comments"),
					resource.TestCheckResourceAttr("incident_api_key.test", "role_names.#", "1"),
					resource.TestCheckNoResourceAttr("incident_api_key.test", "token_version"),
					checkAPIKeyAttrEquals("incident_api_key.test", "token", &rotatedToken),
				),
			},
			{
				// Clearing every account role leaves a key that can do nothing, which is a
				// state the API allows and the empty-set default has to round-trip.
				Config: testAccIncidentAPIKeyResourceConfig(apiKeyTestConfig{}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_api_key.test", "role_names.#", "0"),
				),
			},
		},
	})
}

// TestAccIncidentAPIKeyResourceTeamScoped covers a key scoped to a team rather than the
// whole account: no account-level roles at all, which is why role_names has to be allowed
// to be empty.
//
// The test API key needs "schedules_editor" as a team role for this to be grantable.
func TestAccIncidentAPIKeyResourceTeamScoped(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAPIKeyTeamScopedConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_api_key.test", "role_names.#", "0"),
					resource.TestCheckResourceAttr("incident_api_key.test", "team_ids.#", "1"),
					resource.TestCheckResourceAttrPair(
						"incident_api_key.test", "team_ids.0",
						"incident_catalog_entry.api_key_team", "id",
					),
					resource.TestCheckTypeSetElemAttr("incident_api_key.test", "team_role_names.*", "schedules_editor"),
					resource.TestCheckResourceAttr("data.incident_api_key.by_id", "team_ids.#", "1"),
				),
			},
		},
	})
}

// TestAccIncidentAPIKeyResourceTeamPairing checks the plan-time errors for team scoping,
// which the API requires to be set as a pair: teams with no roles to grant for them, or
// roles with no teams to grant them for, are both rejected before an apply is attempted.
func TestAccIncidentAPIKeyResourceTeamPairing(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("incident_api_key_teams_only", `
resource "incident_api_key" "test" {
  name     = {{ stableSuffix "TF test api key teams only" | quote }}
  team_ids = ["01G0J1EXE7AXZ2C93K61WBPYEH"]
}
`, nil),
				ExpectError: regexp.MustCompile(`needs at least one team role`),
			},
			{
				Config: testRunTemplate("incident_api_key_team_roles_only", `
resource "incident_api_key" "test" {
  name            = {{ stableSuffix "TF test api key team roles only" | quote }}
  team_role_names = ["schedules_editor"]
}
`, nil),
				ExpectError: regexp.MustCompile(`it needs team_ids saying which`),
			},
		},
	})
}

func testAccIncidentAPIKeyTeamScopedConfig() string {
	return testRunTemplate("incident_api_key_team_scoped", `
data "incident_catalog_type" "team" {
  name = {{ .TeamTypeName | quote }}
}

resource "incident_catalog_entry" "api_key_team" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = {{ .TeamName | quote }}
  name               = {{ .TeamName | quote }}
  attribute_values   = []
  managed_attributes = []
}

resource "incident_api_key" "test" {
  name            = {{ .Name | quote }}
  team_ids        = [incident_catalog_entry.api_key_team.id]
  team_role_names = ["schedules_editor"]
}

data "incident_api_key" "by_id" {
  id = incident_api_key.test.id
}
`, struct {
		TeamTypeName string
		TeamName     string
		Name         string
	}{
		TeamTypeName: teamTypeName(),
		TeamName:     StableSuffix("tf-acceptance-test-api-key-team"),
		Name:         StableSuffix("TF test api key team scoped"),
	})
}

// apiKeyTestConfig is the config each step renders. Name is a suffix so one test's keys
// don't collide with another's, on top of the per-run suffix: these run against a shared
// account, and looking a key up by name needs the name to match exactly one.
type apiKeyTestConfig struct {
	Name         string
	Comments     string
	Roles        []string
	TokenVersion int64
	// GracePeriod is a pointer so a step can ask for zero, which is a meaningful value
	// here: retire the old token immediately.
	GracePeriod *int64
}

func testAccIncidentAPIKeyResourceConfig(config apiKeyTestConfig) string {
	name := "TF test api key"
	if config.Name != "" {
		name = fmt.Sprintf("TF test api key %s", config.Name)
	}
	config.Name = StableSuffix(name)

	return testRunTemplate("incident_api_key", `
resource "incident_api_key" "test" {
  name = {{ .Name | quote }}
  {{ if .Comments }}comments = {{ .Comments | quote }}{{ end }}
  role_names = [{{ range $index, $role := .Roles }}{{ if $index }}, {{ end }}{{ $role | quote }}{{ end }}]
  {{ if .TokenVersion }}token_version = {{ .TokenVersion }}{{ end }}
  {{ if .GracePeriod }}rotation_grace_period_minutes = {{ .GracePeriod }}{{ end }}
}

data "incident_api_key" "by_id" {
  id = incident_api_key.test.id
}

data "incident_api_key" "by_name" {
  name = incident_api_key.test.name
}
`, config)
}

// captureAPIKeyAttr stashes an attribute's value so a later step can compare against it.
// A token is only ever returned once, so watching the stored one is the only way to see
// whether a rotation happened.
func captureAPIKeyAttr(resourceName, attribute string, into *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		value, err := apiKeyAttrValue(state, resourceName, attribute)
		if err != nil {
			return err
		}

		*into = value

		return nil
	}
}

// checkAPIKeyAttrEquals asserts an attribute still holds a previously captured value.
func checkAPIKeyAttrEquals(resourceName, attribute string, want *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		value, err := apiKeyAttrValue(state, resourceName, attribute)
		if err != nil {
			return err
		}

		if value != *want {
			return fmt.Errorf("expected %s.%s to be unchanged, but it moved", resourceName, attribute)
		}

		return nil
	}
}

// checkAPIKeyAttrDiffers asserts an attribute no longer holds a previously captured value.
// The values are secrets, so a failure says that they matched rather than what they were.
func checkAPIKeyAttrDiffers(resourceName, attribute string, prior *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		value, err := apiKeyAttrValue(state, resourceName, attribute)
		if err != nil {
			return err
		}

		if value == *prior {
			return fmt.Errorf("expected %s.%s to have changed, but it is the same", resourceName, attribute)
		}

		return nil
	}
}

func apiKeyAttrValue(state *terraform.State, resourceName, attribute string) (string, error) {
	res, ok := state.RootModule().Resources[resourceName]
	if !ok {
		return "", fmt.Errorf("resource %s not found in state", resourceName)
	}

	value, ok := res.Primary.Attributes[attribute]
	if !ok {
		return "", fmt.Errorf("resource %s has no %s attribute", resourceName, attribute)
	}

	return value, nil
}
