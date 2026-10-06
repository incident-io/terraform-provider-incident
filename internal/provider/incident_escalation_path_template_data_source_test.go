package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakeEscalationPathTemplatesAPI is an in-memory stand-in for the two read endpoints the
// data source uses. It seeds templates directly, serves the list newest first with a
// cursor as the real endpoint does, and matches search as a case-insensitive substring,
// so a search for a name also returns the names it is a prefix of.
type fakeEscalationPathTemplatesAPI struct {
	t *testing.T

	mu        sync.Mutex
	templates []client.EscalationPathTemplateV2
	listCount int
	nextID    int

	// pageSize caps the page the list endpoint serves, whatever page_size asks for, so a
	// test can make the data source walk the cursor with a handful of templates.
	pageSize int
}

func startFakeEscalationPathTemplatesAPI(t *testing.T) (*fakeEscalationPathTemplatesAPI, string) {
	t.Helper()

	fake := &fakeEscalationPathTemplatesAPI{t: t, pageSize: escalationPathTemplateListPageSize}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/escalation_path_templates", fake.list)
	mux.HandleFunc("GET /v2/escalation_path_templates/{id}", fake.show)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return fake, server.URL
}

// seed adds a template directly, assigning it the next ID.
func (f *fakeEscalationPathTemplatesAPI) seed(template client.EscalationPathTemplateV2) client.EscalationPathTemplateV2 {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	// IDs that sort in creation order, as ULIDs do.
	template.Id = fmt.Sprintf("01FAKE%020d", f.nextID)
	f.templates = append(f.templates, template)

	return template
}

// listCalls counts the list requests so far, and forgets them.
func (f *fakeEscalationPathTemplatesAPI) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	count := f.listCount
	f.listCount = 0

	return count
}

func (f *fakeEscalationPathTemplatesAPI) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.listCount++

	pageSize := f.pageSize
	if requested, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && requested < pageSize {
		pageSize = requested
	}

	// Newest first, as the real endpoint orders by ID descending.
	ordered := append([]client.EscalationPathTemplateV2{}, f.templates...)
	slices.Reverse(ordered)
	if search := strings.ToLower(r.URL.Query().Get("search")); search != "" {
		ordered = lo.Filter(ordered, func(template client.EscalationPathTemplateV2, _ int) bool {
			return strings.Contains(strings.ToLower(template.Name), search)
		})
	}
	if after := r.URL.Query().Get("after"); after != "" {
		ordered = lo.Filter(ordered, func(template client.EscalationPathTemplateV2, _ int) bool {
			return template.Id < after
		})
	}

	page := ordered[:min(pageSize, len(ordered))]
	meta := client.PaginationMetaResultV2{PageSize: int64(pageSize)}
	// The real endpoint offers a cursor whenever the page filled, whether or not another
	// page exists.
	if len(page) == pageSize && len(page) > 0 {
		meta.After = lo.ToPtr(page[len(page)-1].Id)
	}

	writeJSON(f.t, w, client.EscalationPathTemplatesListResultV2{EscalationPathTemplates: page, PaginationMeta: meta})
}

func (f *fakeEscalationPathTemplatesAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := r.PathValue("id")
	template, found := lo.Find(f.templates, func(template client.EscalationPathTemplateV2) bool { return template.Id == id })
	if !found {
		writeNotFound(w, "escalation path template")
		return
	}

	writeJSON(f.t, w, client.EscalationPathTemplatesShowResultV2{EscalationPathTemplate: template})
}

func boundTarget(param string, targetType client.EscalationPathTargetWithBindingV2Type) client.EscalationPathTargetWithBindingV2 {
	return client.EscalationPathTargetWithBindingV2{
		Type:    targetType,
		Urgency: client.EscalationPathTargetWithBindingV2UrgencyHigh,
		Binding: &client.EngineParamBindingV2{Value: &client.EngineParamBindingValueV2{Reference: lo.ToPtr(param)}},
	}
}

// fullEscalationPathTemplate uses every block the data source reports: a branch on
// working hours, bound and concrete targets, a repeat config and a description.
func fullEscalationPathTemplate() client.EscalationPathTemplateV2 {
	schedule := boundTarget("schedule", client.EscalationPathTargetWithBindingV2TypeSchedule)
	schedule.ScheduleMode = lo.ToPtr(client.EscalationPathTargetWithBindingV2ScheduleModeCurrentlyOnCall)

	return client.EscalationPathTemplateV2{
		Name:        "Team on-call",
		Description: lo.ToPtr("Page the team's schedule in hours, otherwise its channel then its lead."),
		Params: []client.EngineParamV2{
			{Name: "schedule", Label: "Schedule", Type: `CatalogEntry["Schedule"]`, Description: "The team's schedule"},
			{Name: "lead", Label: "Team lead", Type: `CatalogEntry["User"]`, Optional: true},
		},
		Path: []client.EscalationPathTemplateNodeV2{{
			Id:   "in-hours",
			Type: client.EscalationPathTemplateNodeV2TypeIfElse,
			IfElse: &client.EscalationPathTemplateNodeIfElseV2{
				Conditions: []client.ConditionV2{{
					Subject:   client.ConditionSubjectV2{Reference: `escalation.working_hours["UK"]`},
					Operation: client.ConditionOperationV2{Value: "is_active"},
				}},
				ThenPath: []client.EscalationPathTemplateNodeV2{{
					Id:   "page-schedule",
					Type: client.EscalationPathTemplateNodeV2TypeLevel,
					Level: &client.EscalationPathNodeLevelWithBindingV2{
						Targets:          []client.EscalationPathTargetWithBindingV2{schedule},
						TimeToAckSeconds: lo.ToPtr(int64(300)),
					},
				}},
				ElsePath: []client.EscalationPathTemplateNodeV2{
					{
						Id:   "notify-channel",
						Type: client.EscalationPathTemplateNodeV2TypeNotifyChannel,
						NotifyChannel: &client.EscalationPathNodeNotifyChannelWithBindingV2{
							Targets: []client.EscalationPathTargetWithBindingV2{{
								Id:      lo.ToPtr("C0ONCALL00000"),
								Type:    client.EscalationPathTargetWithBindingV2TypeSlackChannel,
								Urgency: client.EscalationPathTargetWithBindingV2UrgencyLow,
							}},
						},
					},
					{
						Id:   "page-lead",
						Type: client.EscalationPathTemplateNodeV2TypeLevel,
						Level: &client.EscalationPathNodeLevelWithBindingV2{
							Targets: []client.EscalationPathTargetWithBindingV2{boundTarget("lead", client.EscalationPathTargetWithBindingV2TypeUser)},
						},
					},
				},
			},
		}},
		WorkingHours: &[]client.WeekdayIntervalConfigV2{{
			Id:       "UK",
			Name:     "UK",
			Timezone: "Europe/London",
			WeekdayIntervals: []client.WeekdayIntervalV2{
				{StartTime: "09:00", EndTime: "17:00", Weekday: client.WeekdayIntervalV2WeekdayMonday},
			},
		}},
		RepeatConfig: &client.EscalationPathRepeatConfigV2{
			RepeatAfterSeconds:    600,
			DelayRepeatOnActivity: true,
		},
	}
}

// bareEscalationPathTemplate is one level paging one parameter: the optional blocks the
// API leaves out.
func bareEscalationPathTemplate(name string) client.EscalationPathTemplateV2 {
	return client.EscalationPathTemplateV2{
		Name:   name,
		Params: []client.EngineParamV2{{Name: "user", Label: "User", Type: `CatalogEntry["User"]`}},
		Path: []client.EscalationPathTemplateNodeV2{{
			Id:   "page",
			Type: client.EscalationPathTemplateNodeV2TypeLevel,
			Level: &client.EscalationPathNodeLevelWithBindingV2{
				Targets: []client.EscalationPathTargetWithBindingV2{boundTarget("user", client.EscalationPathTargetWithBindingV2TypeUser)},
			},
		}},
	}
}

// TestIncidentEscalationPathTemplateDataSourceByID reads a template through the show
// endpoint, and checks the optional blocks come back null rather than empty when the API
// leaves them out.
func TestIncidentEscalationPathTemplateDataSourceByID(t *testing.T) {
	fake, url := startFakeEscalationPathTemplatesAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	full := fake.seed(fullEscalationPathTemplate())
	bare := fake.seed(bareEscalationPathTemplate("Page one person"))

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "incident_escalation_path_template" "full" {
  id = %q
}

data "incident_escalation_path_template" "bare" {
  id = %q
}
`, full.Id, bare.Id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "id", full.Id),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "name", "Team on-call"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "description", "Page the team's schedule in hours, otherwise its channel then its lead."),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "params.%", "2"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "params.schedule.label", "Schedule"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "params.schedule.type", `CatalogEntry["Schedule"]`),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "params.schedule.description", "The team's schedule"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "params.schedule.optional", "false"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "params.lead.optional", "true"),
					// With no config to take names from, the root sequence and each branch get
					// the fallback names.
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "start", "main"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.%", "3"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main.nodes.0.id", "in-hours"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main.nodes.0.branch.if.working_hours_active", "UK"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main.nodes.0.branch.then", "main_then"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main.nodes.0.branch.else", "main_else"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_then.nodes.0.id", "page-schedule"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_then.nodes.0.level.time_to_ack_seconds", "300"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_then.nodes.0.level.targets.0.type", "schedule"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_then.nodes.0.level.targets.0.urgency", "high"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_then.nodes.0.level.targets.0.schedule_mode", "currently_on_call"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_then.nodes.0.level.targets.0.binding.value.reference", "schedule"),
					resource.TestCheckNoResourceAttr("data.incident_escalation_path_template.full", "sequences.main_then.nodes.0.level.targets.0.id"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_else.nodes.0.notify_channel.targets.0.id", "C0ONCALL00000"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_else.nodes.0.notify_channel.targets.0.type", "slack_channel"),
					resource.TestCheckNoResourceAttr("data.incident_escalation_path_template.full", "sequences.main_else.nodes.0.notify_channel.targets.0.binding.value.reference"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "sequences.main_else.nodes.1.level.targets.0.binding.value.reference", "lead"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "working_hours.0.id", "UK"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "working_hours.0.timezone", "Europe/London"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "working_hours.0.weekday_intervals.0.weekday", "monday"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "repeat_config.repeat_after_seconds", "600"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.full", "repeat_config.delay_repeat_on_activity", "true"),
					resource.TestCheckNoResourceAttr("data.incident_escalation_path_template.full", "unlock_in_dashboard"),

					resource.TestCheckResourceAttr("data.incident_escalation_path_template.bare", "id", bare.Id),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.bare", "name", "Page one person"),
					resource.TestCheckNoResourceAttr("data.incident_escalation_path_template.bare", "description"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.bare", "start", "main"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.bare", "sequences.%", "1"),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.bare", "sequences.main.nodes.0.level.targets.0.binding.value.reference", "user"),
					resource.TestCheckNoResourceAttr("data.incident_escalation_path_template.bare", "working_hours.#"),
					resource.TestCheckNoResourceAttr("data.incident_escalation_path_template.bare", "repeat_config.repeat_after_seconds"),
					func(*terraform.State) error {
						if got := fake.listCalls(); got != 0 {
							return fmt.Errorf("a lookup by id should not list, got %d list calls", got)
						}

						return nil
					},
				),
			},
			{
				Config: `
data "incident_escalation_path_template" "missing" {
  id = "01FAKE00000000000000000000"
}
`,
				ExpectError: regexp.MustCompile(`Unable to read escalation path template`),
			},
		},
	})
}

// TestIncidentEscalationPathTemplateDataSourcePagesToFindAName is the case a lookup by
// ID never hits: the list's search matches more names than the one asked for, so each
// page is filtered to the exact name, and a name on a later page is only found by walking
// the cursor. Two templates sharing a name are reported rather than picked between.
func TestIncidentEscalationPathTemplateDataSourcePagesToFindAName(t *testing.T) {
	fake, url := startFakeEscalationPathTemplatesAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	// One per page. The list is newest first, so the oldest template is on the last one,
	// behind the one whose name the search also matches.
	oldest := fake.seed(fullEscalationPathTemplate())
	fake.seed(bareEscalationPathTemplate("Team on-call (legacy)"))
	fake.seed(bareEscalationPathTemplate("Platform on-call"))
	fake.seed(bareEscalationPathTemplate("Platform on-call"))
	fake.pageSize = 1

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_escalation_path_template" "missing" {
  name = "Nobody's template"
}
`,
				ExpectError: wrapRe(`no escalation path template found with name "Nobody's template"`),
			},
			{
				Config: `
data "incident_escalation_path_template" "duplicated" {
  name = "Platform on-call"
}
`,
				ExpectError: wrapRe(`found 2 escalation path templates named "Platform on-call"; look it up by id instead`),
			},
			{
				Config: `
data "incident_escalation_path_template" "both" {
  id   = "01FAKE"
  name = "Team on-call"
}
`,
				ExpectError: regexp.MustCompile(`Ambiguous lookup`),
			},
			{
				Config: `
data "incident_escalation_path_template" "neither" {
}
`,
				ExpectError: regexp.MustCompile(`Missing lookup`),
			},
			{
				// Last, so the config the harness tears down with is one that works.
				Config: `
data "incident_escalation_path_template" "oldest" {
  name = "Team on-call"
}
`,
				PreConfig: func() { fake.listCalls() },
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.oldest", "id", oldest.Id),
					resource.TestCheckResourceAttr("data.incident_escalation_path_template.oldest", "repeat_config.repeat_after_seconds", "600"),
					func(*terraform.State) error {
						// The search matches two templates, one per page, then the empty page
						// ends the walk. Terraform reads a data source once per plan and once
						// per apply here.
						if got := fake.listCalls(); got%3 != 0 || got == 0 {
							return fmt.Errorf("want list calls in rounds of 3 (2 pages and the empty page after), got %d", got)
						}

						return nil
					},
				),
			},
		},
	})
}
