package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// announcementTemplateTestField is one field in a test config. Unset attributes are left
// out of the config.
type announcementTemplateTestField struct {
	FieldType string
	Emoji     string
	Markdown  string
}

type announcementTemplateTestConfig struct {
	Name        string
	Fields      []announcementTemplateTestField
	Actions     []string
	WithLookups bool
}

func testAccIncidentAnnouncementTemplateResourceConfig(config announcementTemplateTestConfig) string {
	if config.Name == "" {
		config.Name = StableSuffix("TF test announcement template")
	}

	return testRunTemplate("incident_announcement_template", `
resource "incident_announcement_template" "test" {
  name = {{ .Name | quote }}

  fields = [
{{ range .Fields }}    {
      field_type = {{ .FieldType | quote }}
{{ if .Emoji }}      emoji      = {{ .Emoji | quote }}
{{ end }}{{ if .Markdown }}      rich_text = {
        type     = "markdown"
        contents = {{ .Markdown | quote }}
      }
{{ end }}    },
{{ end }}  ]

  actions = [
{{ range .Actions }}    { action_type = {{ . | quote }} },
{{ end }}  ]
}
{{ if .WithLookups }}
data "incident_announcement_template" "by_id" {
  id = incident_announcement_template.test.id
}

data "incident_announcement_template" "by_name" {
  name = incident_announcement_template.test.name
}
{{ end }}
`, config)
}

const (
	announcementFieldSeverity = "announcement_post_fields_severity"
	announcementFieldStatus   = "announcement_post_fields_status"
	announcementFieldRichText = "announcement_post_fields_rich_text"

	announcementActionJoinCall  = "announcement_post_actions_join_call"
	announcementActionHomepage  = "announcement_post_actions_homepage"
	announcementActionSubscribe = "announcement_post_actions_subscribe"
)

// announcementTemplateMarkdown carries formatting and a variable: the two things a rich
// text field most needs to keep through a read and a write-back.
const announcementTemplateMarkdown = "If you work on **payments**, please join {{incident.reference}}"

// testAnnouncementTemplateLifecycleSteps is the life of a template: created with fields and
// actions and looked up both ways, imported, reordered, and finally left with nothing on it.
// The same steps run against a fake API as a unit test and against a real account.
func testAnnouncementTemplateLifecycleSteps() []resource.TestStep {
	const address = "incident_announcement_template.test"

	return []resource.TestStep{
		{
			Config: testAccIncidentAnnouncementTemplateResourceConfig(announcementTemplateTestConfig{
				Fields: []announcementTemplateTestField{
					{FieldType: announcementFieldSeverity, Emoji: "fire"},
					{FieldType: announcementFieldRichText, Markdown: announcementTemplateMarkdown},
				},
				Actions:     []string{announcementActionJoinCall, announcementActionHomepage},
				WithLookups: true,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(address, "id"),
				resource.TestCheckResourceAttr(address, "is_default", "false"),
				resource.TestCheckResourceAttr(address, "fields.#", "2"),
				resource.TestCheckResourceAttr(address, "fields.0.field_type", announcementFieldSeverity),
				resource.TestCheckResourceAttr(address, "fields.0.emoji", "fire"),
				resource.TestCheckNoResourceAttr(address, "fields.0.rich_text"),
				resource.TestCheckResourceAttr(address, "fields.1.rich_text.type", "markdown"),
				resource.TestCheckResourceAttr(address, "fields.1.rich_text.contents", announcementTemplateMarkdown),
				resource.TestCheckResourceAttr(address, "actions.#", "2"),
				resource.TestCheckResourceAttr(address, "actions.1.action_type", announcementActionHomepage),
				resource.TestCheckResourceAttr(address, "owning_team_ids.#", "0"),
				resource.TestCheckResourceAttrPair("data.incident_announcement_template.by_id", "id", address, "id"),
				resource.TestCheckResourceAttrPair("data.incident_announcement_template.by_name", "id", address, "id"),
				resource.TestCheckResourceAttr("data.incident_announcement_template.by_name", "fields.1.rich_text.contents", announcementTemplateMarkdown),
				resource.TestCheckResourceAttr("data.incident_announcement_template.by_id", "actions.0.action_type", announcementActionJoinCall),
			),
		},
		{
			ResourceName:      address,
			ImportState:       true,
			ImportStateVerify: true,
		},
		{
			// The order of the lists is the order on the post, so reordering is a change.
			Config: testAccIncidentAnnouncementTemplateResourceConfig(announcementTemplateTestConfig{
				Fields: []announcementTemplateTestField{
					{FieldType: announcementFieldRichText, Markdown: announcementTemplateMarkdown},
					{FieldType: announcementFieldStatus},
					{FieldType: announcementFieldSeverity, Emoji: "fire"},
				},
				Actions: []string{announcementActionSubscribe},
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(address, "fields.#", "3"),
				resource.TestCheckResourceAttr(address, "fields.0.field_type", announcementFieldRichText),
				resource.TestCheckResourceAttr(address, "fields.1.field_type", announcementFieldStatus),
				resource.TestCheckNoResourceAttr(address, "fields.1.emoji"),
				resource.TestCheckResourceAttr(address, "fields.2.emoji", "fire"),
				resource.TestCheckResourceAttr(address, "actions.#", "1"),
				resource.TestCheckResourceAttr(address, "actions.0.action_type", announcementActionSubscribe),
			),
		},
		{
			Config: testAccIncidentAnnouncementTemplateResourceConfig(announcementTemplateTestConfig{}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(address, "fields.#", "0"),
				resource.TestCheckResourceAttr(address, "actions.#", "0"),
			),
		},
	}
}

// TestIncidentAnnouncementTemplateResourceLifecycle runs the lifecycle against a fake API,
// so it needs no account.
func TestIncidentAnnouncementTemplateResourceLifecycle(t *testing.T) {
	_, url := startFakeAnnouncementsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testAnnouncementTemplateLifecycleSteps(),
	})
}

// TestIncidentAnnouncementTemplateDataSourceLookups checks the lookup attributes are
// validated, and that the seeded default template can be found by name.
func TestIncidentAnnouncementTemplateDataSourceLookups(t *testing.T) {
	fake, url := startFakeAnnouncementsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_announcement_template" "missing" {
  name = "Nobody's template"
}
`,
				ExpectError: wrapRe(`no announcement template found with name "Nobody's template"`),
			},
			{
				Config: `
data "incident_announcement_template" "both" {
  id   = "01FAKE"
  name = "Default"
}
`,
				ExpectError: regexp.MustCompile(`Ambiguous lookup`),
			},
			{
				Config: `
data "incident_announcement_template" "default" {
  name = "Default"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_announcement_template.default", "id", fake.defaultTemplate().Id),
					resource.TestCheckResourceAttr("data.incident_announcement_template.default", "is_default", "true"),
				),
			},
		},
	})
}

// TestAccIncidentAnnouncementTemplateResource runs the lifecycle against a real account.
func TestAccIncidentAnnouncementTemplateResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testAnnouncementTemplateLifecycleSteps(),
	})
}
