package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type announcementRuleTestConfig struct {
	Name                             string
	SlackChannelIDs                  []string
	Mode                             string
	ConditionsNoLongerApplyBehaviour string
	// WithTemplate points the rule at a template of its own. Without it the rule leaves
	// template_id out of its config.
	WithTemplate bool
	// SeverityID is the severity the rule's condition matches. When empty the config
	// creates a severity to match, which only a real account can serve.
	SeverityID  string
	WithLookups bool
}

func testAccIncidentAnnouncementRuleResourceConfig(config announcementRuleTestConfig) string {
	if config.Name == "" {
		config.Name = StableSuffix("TF test announcement rule")
	}
	if config.Mode == "" {
		config.Mode = "include_triage"
	}

	return testRunTemplate("incident_announcement_rule", `
{{ if not .SeverityID }}
resource "incident_severity" "announced" {
  name        = {{ printf "%s severity" .Name | quote }}
  description = "Incidents at this severity are announced."
  rank        = 100
}
{{ end }}
{{ if .WithTemplate }}
resource "incident_announcement_template" "rule" {
  name    = {{ printf "%s template" .Name | quote }}
  fields  = [{ field_type = "announcement_post_fields_severity" }]
  actions = []
}
{{ end }}
resource "incident_announcement_rule" "test" {
  name                = {{ .Name | quote }}
  slack_channel_ids   = [{{ range $i, $id := .SlackChannelIDs }}{{ if $i }}, {{ end }}{{ $id | quote }}{{ end }}]
  mode                = {{ .Mode | quote }}
  update_sharing_mode = "thread"
{{ if .ConditionsNoLongerApplyBehaviour }}
  conditions_no_longer_apply_behaviour = {{ .ConditionsNoLongerApplyBehaviour | quote }}
{{ end }}{{ if .WithTemplate }}
  template_id = incident_announcement_template.rule.id
{{ end }}
  condition_groups = [
    {
      conditions = [
        {
          subject        = "incident.severity"
          operation      = "one_of"
          param_bindings = [{ array_value = [{ literal = {{ if .SeverityID }}{{ .SeverityID | quote }}{{ else }}incident_severity.announced.id{{ end }} }] }]
        }
      ]
    }
  ]
}
{{ if .WithLookups }}
data "incident_announcement_rule" "by_id" {
  id = incident_announcement_rule.test.id
}

data "incident_announcement_rule" "by_name" {
  name = incident_announcement_rule.test.name
}
{{ end }}
`, config)
}

// testAnnouncementRuleLifecycleSteps is the life of a rule: created with a template of its
// own and looked up both ways, imported, then changed. channelIDs are the Slack channels it
// posts into, and severityID the severity its condition matches (see
// announcementRuleTestConfig).
func testAnnouncementRuleLifecycleSteps(channelIDs []string, severityID string) []resource.TestStep {
	const address = "incident_announcement_rule.test"
	name := StableSuffix("TF test announcement rule")

	return []resource.TestStep{
		{
			Config: testAccIncidentAnnouncementRuleResourceConfig(announcementRuleTestConfig{
				Name:            name,
				SlackChannelIDs: channelIDs,
				WithTemplate:    true,
				SeverityID:      severityID,
				WithLookups:     true,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(address, "id"),
				resource.TestCheckResourceAttr(address, "slack_channel_ids.#", fmt.Sprint(len(channelIDs))),
				resource.TestCheckTypeSetElemAttr(address, "slack_channel_ids.*", channelIDs[0]),
				resource.TestCheckResourceAttr(address, "microsoft_teams_channel_ids.#", "0"),
				resource.TestCheckResourceAttr(address, "mode", "include_triage"),
				resource.TestCheckResourceAttr(address, "update_sharing_mode", "thread"),
				// The API's defaults for what the config leaves out.
				resource.TestCheckResourceAttr(address, "private_incident_scope", "none"),
				resource.TestCheckResourceAttr(address, "conditions_no_longer_apply_behaviour", "leave_in_place"),
				resource.TestCheckResourceAttrPair(address, "template_id", "incident_announcement_template.rule", "id"),
				resource.TestCheckResourceAttr(address, "condition_groups.0.conditions.0.subject", "incident.severity"),
				resource.TestCheckResourceAttr(address, "condition_groups.0.conditions.0.operation", "one_of"),
				resource.TestCheckResourceAttrSet(address, "created_at"),
				resource.TestCheckResourceAttrPair("data.incident_announcement_rule.by_id", "id", address, "id"),
				resource.TestCheckResourceAttrPair("data.incident_announcement_rule.by_name", "id", address, "id"),
				resource.TestCheckResourceAttrPair("data.incident_announcement_rule.by_name", "template_id", address, "template_id"),
				resource.TestCheckResourceAttr("data.incident_announcement_rule.by_id", "condition_groups.0.conditions.0.subject", "incident.severity"),
			),
		},
		{
			ResourceName:      address,
			ImportState:       true,
			ImportStateVerify: true,
		},
		{
			// Leaving template_id out of the config keeps the rule on the template it has,
			// rather than moving it back to the default.
			Config: testAccIncidentAnnouncementRuleResourceConfig(announcementRuleTestConfig{
				Name:                             name,
				SlackChannelIDs:                  channelIDs[:1],
				Mode:                             "live_and_closed",
				ConditionsNoLongerApplyBehaviour: "remove",
				WithTemplate:                     true,
				SeverityID:                       severityID,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(address, "mode", "live_and_closed"),
				resource.TestCheckResourceAttr(address, "conditions_no_longer_apply_behaviour", "remove"),
				resource.TestCheckResourceAttr(address, "slack_channel_ids.#", "1"),
				resource.TestCheckResourceAttrPair(address, "template_id", "incident_announcement_template.rule", "id"),
			),
		},
	}
}

// TestIncidentAnnouncementRuleResourceLifecycle runs the lifecycle against a fake API, so
// it needs no account.
func TestIncidentAnnouncementRuleResourceLifecycle(t *testing.T) {
	_, url := startFakeAnnouncementsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testAnnouncementRuleLifecycleSteps([]string{"C0FAKEONE", "C0FAKETWO"}, "01FAKESEVERITY"),
	})
}

// TestIncidentAnnouncementRuleResourceDefaultTemplate checks a rule that names no template
// is given the organisation's default, and that nothing then wants to change.
func TestIncidentAnnouncementRuleResourceDefaultTemplate(t *testing.T) {
	fake, url := startFakeAnnouncementsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	config := testAccIncidentAnnouncementRuleResourceConfig(announcementRuleTestConfig{
		SlackChannelIDs: []string{"C0FAKEONE"},
		SeverityID:      "01FAKESEVERITY",
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.TestCheckResourceAttr(
					"incident_announcement_rule.test", "template_id", fake.defaultTemplate().Id),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// TestIncidentAnnouncementRuleResourceGoneFromAPI checks a rule deleted behind Terraform's
// back is dropped from state and recreated, rather than failing the refresh.
func TestIncidentAnnouncementRuleResourceGoneFromAPI(t *testing.T) {
	fake, url := startFakeAnnouncementsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	config := testAccIncidentAnnouncementRuleResourceConfig(announcementRuleTestConfig{
		SlackChannelIDs: []string{"C0FAKEONE"},
		SeverityID:      "01FAKESEVERITY",
	})

	var firstID string
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					firstID = s.RootModule().Resources["incident_announcement_rule.test"].Primary.ID

					return nil
				},
			},
			{
				PreConfig: func() {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					fake.rules = nil
				},
				Config: config,
				Check: func(s *terraform.State) error {
					if id := s.RootModule().Resources["incident_announcement_rule.test"].Primary.ID; id == firstID {
						return fmt.Errorf("want a new rule to have been created, still have %s", id)
					}

					return nil
				},
			},
		},
	})
}

// TestIncidentAnnouncementRuleDataSourcePagesToFindAName covers what a lookup by ID never
// does: the list is paginated with no name filter, so a name on a later page is only found
// by walking the cursor, and a name on no page has to end the walk.
func TestIncidentAnnouncementRuleDataSourcePagesToFindAName(t *testing.T) {
	fake, url := startFakeAnnouncementsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	// One per page. The list is newest first, so the oldest rule is on the last page.
	oldest := fake.seedRule("Major incidents")
	fake.seedRule("Payments")
	fake.seedRule("Security")
	fake.pageSize = 1

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_announcement_rule" "missing" {
  name = "Nobody's rule"
}
`,
				ExpectError: wrapRe(`no announcement rule found with name "Nobody's rule"`),
			},
			{
				Config: `
data "incident_announcement_rule" "neither" {
}
`,
				ExpectError: regexp.MustCompile(`Missing lookup`),
			},
			{
				// Last, so the config the harness tears down with is one that works.
				Config: `
data "incident_announcement_rule" "oldest" {
  name = "Major incidents"
}
`,
				PreConfig: func() { fake.listCalls() },
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_announcement_rule.oldest", "id", oldest.Id),
					resource.TestCheckTypeSetElemAttr("data.incident_announcement_rule.oldest", "slack_channel_ids.*", "C0SEEDED"),
					func(*terraform.State) error {
						// Three full pages, then the empty one that ends the walk, once per read.
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

// TestAccIncidentAnnouncementRuleResource runs the lifecycle against a real account. A rule
// needs a Slack channel incident.io can post into, which the test can't create, so it
// skips unless TF_ACC_CHANNEL_ID names one.
func TestAccIncidentAnnouncementRuleResource(t *testing.T) {
	channelID := os.Getenv("TF_ACC_CHANNEL_ID")
	if os.Getenv("TF_ACC") != "" && channelID == "" {
		t.Skip("TF_ACC_CHANNEL_ID not set, skipping announcement rule acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testAnnouncementRuleLifecycleSteps([]string{channelID}, ""),
	})
}
