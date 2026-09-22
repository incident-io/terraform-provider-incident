package provider

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// payConfigRule is one rule in a test config. Weekly rules set Weekdays, StartTime and
// EndTime; one-off rules set Name, StartAt and EndAt.
type payConfigRule struct {
	Weekdays  []string
	StartTime string
	EndTime   string
	Name      string
	StartAt   string
	EndAt     string
	RateCents int64
}

// payConfigTestConfig is the config each step renders. A nil rule list leaves the
// attribute out of the config altogether, which is different from an empty one.
type payConfigTestConfig struct {
	Name          string
	Timezone      string
	Currency      string
	BaseRateCents int64
	RateTimeUnit  string
	WeeklyRules   []payConfigRule
	OneOffRules   []payConfigRule
	// OmitWeeklyRules and OmitOneOffRules drop the attribute rather than setting a list.
	OmitWeeklyRules bool
	OmitOneOffRules bool
	// WithDataSources adds a lookup by id and by name alongside the resource.
	WithDataSources bool
}

func testAccIncidentPayConfigResourceConfig(config payConfigTestConfig) string {
	if config.Name == "" {
		config.Name = StableSuffix("TF test pay config")
	}
	if config.Timezone == "" {
		config.Timezone = "Europe/London"
	}
	if config.Currency == "" {
		config.Currency = "GBP"
	}
	if config.RateTimeUnit == "" {
		config.RateTimeUnit = "hour"
	}

	return testRunTemplate("incident_pay_config", `
resource "incident_pay_config" "test" {
  name            = {{ .Name | quote }}
  timezone        = {{ .Timezone | quote }}
  currency        = {{ .Currency | quote }}
  base_rate_cents = {{ .BaseRateCents }}
  rate_time_unit  = {{ .RateTimeUnit | quote }}
{{ if not .OmitWeeklyRules }}
  weekly_rules = [
{{ range .WeeklyRules }}    {
      weekdays   = [{{ range $i, $d := .Weekdays }}{{ if $i }}, {{ end }}{{ $d | quote }}{{ end }}]
      start_time = {{ .StartTime | quote }}
      end_time   = {{ .EndTime | quote }}
      rate_cents = {{ .RateCents }}
    },
{{ end }}  ]
{{ end }}
{{ if not .OmitOneOffRules }}
  one_off_rules = [
{{ range .OneOffRules }}    {
      name       = {{ .Name | quote }}
      start_at   = {{ .StartAt | quote }}
      end_at     = {{ .EndAt | quote }}
      rate_cents = {{ .RateCents }}
    },
{{ end }}  ]
{{ end }}
}
{{ if .WithDataSources }}
data "incident_pay_config" "by_id" {
  id = incident_pay_config.test.id
}

data "incident_pay_config" "by_name" {
  name = incident_pay_config.test.name
}
{{ end }}
`, config)
}

var (
	weekendRule = payConfigRule{Weekdays: []string{"saturday", "sunday"}, StartTime: "00:00", EndTime: "00:00", RateCents: 1500}
	// A rule's window is within one day, so the evening rate stops at midnight: the
	// morning after would be a rule of its own.
	nightRule  = payConfigRule{Weekdays: []string{"monday", "tuesday", "wednesday", "thursday", "friday"}, StartTime: "18:00", EndTime: "00:00", RateCents: 1000}
	fridayRule = payConfigRule{Weekdays: []string{"friday"}, StartTime: "12:00", EndTime: "18:00", RateCents: 800}

	christmasRule = payConfigRule{Name: "Christmas Day", StartAt: "2026-12-25T00:00:00Z", EndAt: "2026-12-26T00:00:00Z", RateCents: 3000}
	boxingRule    = payConfigRule{Name: "Boxing Day", StartAt: "2026-12-26T00:00:00Z", EndAt: "2026-12-27T00:00:00Z", RateCents: 2500}
	festiveRule   = payConfigRule{Name: "Festive break", StartAt: "2026-12-25T00:00:00Z", EndAt: "2026-12-27T00:00:00Z", RateCents: 2800}

	// The same two holidays with the boundary between them moved to midday, so Christmas
	// Day grows into the window Boxing Day is giving up.
	christmasLongRule = payConfigRule{Name: "Christmas Day", StartAt: "2026-12-25T00:00:00Z", EndAt: "2026-12-26T12:00:00Z", RateCents: 3000}
	boxingLateRule    = payConfigRule{Name: "Boxing Day", StartAt: "2026-12-26T12:00:00Z", EndAt: "2026-12-27T00:00:00Z", RateCents: 2500}
)

// testPayConfigLifecycleSteps is the whole life of a pay config: created with rules,
// imported, its attributes changed without touching the rules, its rules changed without
// touching it, rules removed from the middle and the end, one-off rules replaced by one
// that overlaps both, and finally left with no rules at all.
//
// The steps are shared by the hermetic unit test, which asserts the exact writes each
// change makes against a fake API, and the acceptance test, which runs them against a
// real account. wroteExactly is nil for the latter: what the real API was asked is not
// something a test can see.
func testPayConfigLifecycleSteps(wroteExactly func(...string) resource.TestCheckFunc) []resource.TestStep {
	if wroteExactly == nil {
		wroteExactly = func(...string) resource.TestCheckFunc {
			return func(*terraform.State) error { return nil }
		}
	}

	// The night rule after a pay rise, derived from the rule itself so that the only
	// difference between the two is the one the steps below are about.
	nightRuleRaised := nightRule
	nightRuleRaised.RateCents = 1100

	// The IDs each position had after the first apply. A change to a rule must keep its
	// position's ID: that is what the plan promised, and what makes a change an update
	// rather than a replacement.
	ids := map[string]string{}
	remember := func(attributes ...string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			rs, ok := s.RootModule().Resources["incident_pay_config.test"]
			if !ok {
				return fmt.Errorf("incident_pay_config.test not in state")
			}
			for _, attribute := range attributes {
				ids[attribute] = rs.Primary.Attributes[attribute]
			}

			return nil
		}
	}
	stillHas := func(attribute string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			rs := s.RootModule().Resources["incident_pay_config.test"]
			if got, want := rs.Primary.Attributes[attribute], ids[attribute]; got != want {
				return fmt.Errorf("%s: got %q, want the ID it had after the first apply, %q", attribute, got, want)
			}

			return nil
		}
	}

	return []resource.TestStep{
		{
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents:   500,
				WeeklyRules:     []payConfigRule{weekendRule, nightRule},
				OneOffRules:     []payConfigRule{christmasRule, boxingRule},
				WithDataSources: true,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("incident_pay_config.test", "id"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "timezone", "Europe/London"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "currency", "GBP"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "base_rate_cents", "500"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "rate_time_unit", "hour"),
				resource.TestCheckNoResourceAttr("incident_pay_config.test", "published_at"),
				resource.TestCheckResourceAttrSet("incident_pay_config.test", "created_at"),
				// The rules come back in the order they were written, each with an ID.
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.#", "2"),
				resource.TestCheckResourceAttrSet("incident_pay_config.test", "weekly_rules.0.id"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.0.rate_cents", "1500"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.0.weekdays.#", "2"),
				resource.TestCheckTypeSetElemAttr("incident_pay_config.test", "weekly_rules.0.weekdays.*", "saturday"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.1.start_time", "18:00"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.1.end_time", "00:00"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.#", "2"),
				resource.TestCheckResourceAttrSet("incident_pay_config.test", "one_off_rules.0.id"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.0.name", "Christmas Day"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.0.start_at", "2026-12-25T00:00:00Z"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.1.rate_cents", "2500"),
				// Both lookups find the same config, rules and all.
				resource.TestCheckResourceAttrPair("data.incident_pay_config.by_id", "id", "incident_pay_config.test", "id"),
				resource.TestCheckResourceAttrPair("data.incident_pay_config.by_name", "id", "incident_pay_config.test", "id"),
				resource.TestCheckResourceAttr("data.incident_pay_config.by_name", "weekly_rules.#", "2"),
				resource.TestCheckResourceAttrPair("data.incident_pay_config.by_name", "weekly_rules.1.id", "incident_pay_config.test", "weekly_rules.1.id"),
				resource.TestCheckResourceAttr("data.incident_pay_config.by_id", "one_off_rules.1.name", "Boxing Day"),
				remember("weekly_rules.0.id", "weekly_rules.1.id", "one_off_rules.0.id", "one_off_rules.1.id"),
				wroteExactly("POST /v2/pay_configs"),
			),
		},
		{
			ResourceName:      "incident_pay_config.test",
			ImportState:       true,
			ImportStateVerify: true,
		},
		{
			// The config's own attributes, and nothing else: no rule endpoint is touched.
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents: 600,
				RateTimeUnit:  "day",
				WeeklyRules:   []payConfigRule{weekendRule, nightRule},
				OneOffRules:   []payConfigRule{christmasRule, boxingRule},
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("incident_pay_config.test", "base_rate_cents", "600"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "rate_time_unit", "day"),
				stillHas("weekly_rules.0.id"),
				stillHas("weekly_rules.1.id"),
				wroteExactly("PUT /v2/pay_configs/{id}"),
			),
		},
		{
			// One weekly rule changes and another is added. The config itself is not
			// updated, so this works on a published config with only the rule scopes.
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents: 600,
				RateTimeUnit:  "day",
				WeeklyRules:   []payConfigRule{weekendRule, nightRuleRaised, fridayRule},
				OneOffRules:   []payConfigRule{christmasRule, boxingRule},
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.#", "3"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.1.rate_cents", "1100"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.2.rate_cents", "800"),
				stillHas("weekly_rules.0.id"),
				stillHas("weekly_rules.1.id"),
				remember("weekly_rules.2.id"),
				wroteExactly(
					"PUT /v2/pay_configs/{id}/weekly_rules/{weekly_rules.1.id}",
					"POST /v2/pay_configs/{id}/weekly_rules",
				),
			),
		},
		{
			// The first weekly rule goes. Positions shift down, so the rule at each position
			// is rewritten in place and the last one is removed: the evaluation order is
			// what the config says, and rewriting positions is the only way to keep it so.
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents: 600,
				RateTimeUnit:  "day",
				WeeklyRules:   []payConfigRule{nightRuleRaised, fridayRule},
				OneOffRules:   []payConfigRule{christmasRule, boxingRule},
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.#", "2"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.0.rate_cents", "1100"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.0.start_time", "18:00"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.1.rate_cents", "800"),
				stillHas("weekly_rules.0.id"),
				stillHas("weekly_rules.1.id"),
				wroteExactly(
					"DELETE /v2/pay_configs/{id}/weekly_rules/{weekly_rules.2.id}",
					"PUT /v2/pay_configs/{id}/weekly_rules/{weekly_rules.0.id}",
					"PUT /v2/pay_configs/{id}/weekly_rules/{weekly_rules.1.id}",
				),
			),
		},
		{
			// Both holidays keep their position, but Christmas Day takes the half-day that
			// Boxing Day is giving up. Written in position order, Christmas Day would land
			// on a window Boxing Day still holds and the API would reject it, so the rule
			// that is getting out of the way goes first.
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents: 600,
				RateTimeUnit:  "day",
				WeeklyRules:   []payConfigRule{nightRuleRaised, fridayRule},
				OneOffRules:   []payConfigRule{christmasLongRule, boxingLateRule},
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.0.end_at", "2026-12-26T12:00:00Z"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.1.start_at", "2026-12-26T12:00:00Z"),
				stillHas("one_off_rules.0.id"),
				stillHas("one_off_rules.1.id"),
				wroteExactly(
					"PUT /v2/pay_configs/{id}/one_off_rules/{one_off_rules.1.id}",
					"PUT /v2/pay_configs/{id}/one_off_rules/{one_off_rules.0.id}",
				),
			),
		},
		{
			// Two holidays become one break covering both. The new window overlaps the old
			// Boxing Day rule, which the API rejects on every write, so the removal has to
			// land before the update does.
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents: 600,
				RateTimeUnit:  "day",
				WeeklyRules:   []payConfigRule{nightRuleRaised, fridayRule},
				OneOffRules:   []payConfigRule{festiveRule},
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.#", "1"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.0.name", "Festive break"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.0.end_at", "2026-12-27T00:00:00Z"),
				stillHas("one_off_rules.0.id"),
				wroteExactly(
					"DELETE /v2/pay_configs/{id}/one_off_rules/{one_off_rules.1.id}",
					"PUT /v2/pay_configs/{id}/one_off_rules/{one_off_rules.0.id}",
				),
			),
		},
		{
			// No rules at all: a flat rate.
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents: 600,
				RateTimeUnit:  "day",
				WeeklyRules:   []payConfigRule{},
				OneOffRules:   []payConfigRule{},
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("incident_pay_config.test", "weekly_rules.#", "0"),
				resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.#", "0"),
				wroteExactly(
					"DELETE /v2/pay_configs/{id}/weekly_rules/{weekly_rules.0.id}",
					"DELETE /v2/pay_configs/{id}/weekly_rules/{weekly_rules.1.id}",
					"DELETE /v2/pay_configs/{id}/one_off_rules/{one_off_rules.0.id}",
				),
			),
		},
		{
			// Leaving both lists out means the same as setting them empty, so there is
			// nothing to plan.
			Config: testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
				BaseRateCents:   600,
				RateTimeUnit:    "day",
				OmitWeeklyRules: true,
				OmitOneOffRules: true,
			}),
			PlanOnly: true,
		},
	}
}

// TestIncidentPayConfigResourceLifecycle drives the resource through Terraform against a
// fake of the API, so it runs without an account and can assert the exact writes each
// change makes. The reconciliation is the part of this resource worth pinning: a change
// to one rule must touch that rule and nothing else, removals must land before additions,
// and a rule must keep its position's ID.
func TestIncidentPayConfigResourceLifecycle(t *testing.T) {
	fake, url := startFakePayConfigsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	// wroteExactly checks the writes since the previous step, in order. Placeholders in
	// braces name an attribute of the resource in state, so an expectation can refer to
	// an ID the fake minted. The state is the one after the apply, so a rule the step
	// removed is named by the position it had before: the caller says which.
	previous := map[string]string{}
	wroteExactly := func(want ...string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			rs, ok := s.RootModule().Resources["incident_pay_config.test"]
			if !ok {
				return fmt.Errorf("incident_pay_config.test not in state")
			}

			// A removed rule's ID is no longer in state, so placeholders resolve against
			// the previous step's attributes first and this step's second.
			current := rs.Primary.Attributes
			resolve := func(expectation string) string {
				return regexp.MustCompile(`\{([^}]+)\}`).ReplaceAllStringFunc(expectation, func(match string) string {
					key := strings.Trim(match, "{}")
					if value, ok := previous[key]; ok {
						return value
					}

					return current[key]
				})
			}
			want = lo.Map(want, func(expectation string, _ int) string { return resolve(expectation) })

			got := fake.writes()
			for key, value := range current {
				previous[key] = value
			}

			if !slices.Equal(got, want) {
				return fmt.Errorf("writes since the last step:\n  got  %q\n  want %q", got, want)
			}

			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testPayConfigLifecycleSteps(wroteExactly),
	})
}

// TestIncidentPayConfigResourceOffsetTimestamps writes a one-off rule with a UTC offset,
// which the API reports back in UTC. The two are the same instant, so there must be no
// diff: neither straight after the apply nor on the next plan.
func TestIncidentPayConfigResourceOffsetTimestamps(t *testing.T) {
	_, url := startFakePayConfigsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	config := testAccIncidentPayConfigResourceConfig(payConfigTestConfig{
		BaseRateCents: 500,
		OneOffRules: []payConfigRule{{
			Name:      "New Year's Day",
			StartAt:   "2027-01-01T00:00:00+01:00",
			EndAt:     "2027-01-02T00:00:00+01:00",
			RateCents: 2000,
		}},
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					// State keeps the configured form: semantic equality accepted the API's.
					resource.TestCheckResourceAttr("incident_pay_config.test", "one_off_rules.0.start_at", "2027-01-01T00:00:00+01:00"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// TestIncidentPayConfigResourceGoneFromAPI checks a config deleted behind Terraform's
// back is dropped from state and planned for recreation, rather than failing the refresh.
func TestIncidentPayConfigResourceGoneFromAPI(t *testing.T) {
	fake, url := startFakePayConfigsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	config := testAccIncidentPayConfigResourceConfig(payConfigTestConfig{BaseRateCents: 500})

	var firstID string
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					firstID = s.RootModule().Resources["incident_pay_config.test"].Primary.ID

					return nil
				},
			},
			{
				PreConfig: func() {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					fake.configs = nil
				},
				Config: config,
				Check: func(s *terraform.State) error {
					if id := s.RootModule().Resources["incident_pay_config.test"].Primary.ID; id == firstID {
						return fmt.Errorf("want a new config to have been created, still have %s", id)
					}

					return nil
				},
			},
		},
	})
}

// TestIncidentPayConfigDataSourcePagesToFindAName is the case a lookup by ID never hits:
// the list is paginated and carries no name filter, so a name on a later page is only
// found by walking the cursor, and a name on no page has to end the walk.
func TestIncidentPayConfigDataSourcePagesToFindAName(t *testing.T) {
	fake, url := startFakePayConfigsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	// One per page. The list is newest first, so the oldest config is on the last page.
	oldest := fake.seed("Platform on-call")
	fake.seed("Data on-call")
	fake.seed("Support on-call")
	fake.pageSize = 1

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_pay_config" "missing" {
  name = "Nobody's on-call"
}
`,
				ExpectError: wrapRe(`no pay config found with name "Nobody's on-call"`),
			},
			{
				Config: `
data "incident_pay_config" "both" {
  id   = "01FAKE"
  name = "Platform on-call"
}
`,
				ExpectError: regexp.MustCompile(`Ambiguous lookup`),
			},
			{
				Config: `
data "incident_pay_config" "neither" {
}
`,
				ExpectError: regexp.MustCompile(`Missing lookup`),
			},
			{
				// Last, so the config the harness tears down with is one that works.
				Config: `
data "incident_pay_config" "oldest" {
  name = "Platform on-call"
}
`,
				PreConfig: func() { fake.listCalls() },
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_pay_config.oldest", "id", oldest.Id),
					resource.TestCheckResourceAttr("data.incident_pay_config.oldest", "currency", "GBP"),
					func(*terraform.State) error {
						// Three full pages, then the empty one that ends the walk. Terraform
						// reads a data source once per plan and once per apply here.
						if got := fake.listCalls(); got%4 != 0 || got == 0 {
							return fmt.Errorf("want list calls in rounds of 4 (3 pages and the empty page after), got %d", got)
						}

						return nil
					},
				),
			},
		},
	})
}

// TestAccIncidentPayConfigResource runs the lifecycle against a real account. The API key
// needs the pay_configs_editor role, which carries every scope a pay config takes,
// including update_published. A key without it skips rather than fails: the role is not
// one a key has by default, and CI's key is shared with every other acceptance test.
func TestAccIncidentPayConfigResource(t *testing.T) {
	// The role is read before resource.Test, so honour TF_ACC ourselves rather than
	// calling the API during a unit test run, then initialise testClient.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}
	testAccPreCheck(t)
	testAccRequirePayConfigsEditor(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testPayConfigLifecycleSteps(nil),
	})
}

// testAccRequirePayConfigsEditor skips unless the API key can manage pay configs. The
// identity endpoint reports the key's roles, which is cheaper and less destructive than
// finding out by writing a config the key isn't allowed to write.
func testAccRequirePayConfigsEditor(t *testing.T) {
	identity, err := testClient.UtilitiesV1IdentityWithResponse(t.Context())
	if err != nil {
		t.Fatalf("reading the API key's identity: %s", err)
	}
	if identity.JSON200 == nil {
		t.Fatalf("reading the API key's identity: %s", string(identity.Body))
	}

	if !slices.Contains(identity.JSON200.Identity.Roles, client.IdentityV1RolesPayConfigsEditor) {
		t.Skip("the API key does not have the pay_configs_editor role")
	}
}
