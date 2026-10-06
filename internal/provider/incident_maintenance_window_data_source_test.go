package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakeMaintenanceWindowsAPI is an in-memory stand-in for the two read endpoints the data
// source uses. It seeds windows directly and serves the list newest first with a cursor,
// as the real endpoint does.
type fakeMaintenanceWindowsAPI struct {
	t *testing.T

	mu        sync.Mutex
	windows   []client.MaintenanceWindowV1
	listCount int
	nextID    int

	// pageSize caps the page the list endpoint serves, whatever page_size asks for, so a
	// test can make the data source walk the cursor with a handful of windows.
	pageSize int
}

func startFakeMaintenanceWindowsAPI(t *testing.T) (*fakeMaintenanceWindowsAPI, string) {
	t.Helper()

	fake := &fakeMaintenanceWindowsAPI{t: t, pageSize: maintenanceWindowListPageSize}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/maintenance_windows", fake.list)
	mux.HandleFunc("GET /v1/maintenance_windows/{id}", fake.show)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return fake, server.URL
}

// seed adds a window directly, assigning it the next ID.
func (f *fakeMaintenanceWindowsAPI) seed(window client.MaintenanceWindowV1) client.MaintenanceWindowV1 {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	// IDs that sort in creation order, as ULIDs do.
	window.Id = fmt.Sprintf("01FAKE%020d", f.nextID)
	f.windows = append(f.windows, window)

	return window
}

// listCalls counts the list requests so far, and forgets them.
func (f *fakeMaintenanceWindowsAPI) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	count := f.listCount
	f.listCount = 0

	return count
}

func (f *fakeMaintenanceWindowsAPI) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.listCount++

	pageSize := f.pageSize
	if requested, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && requested < pageSize {
		pageSize = requested
	}

	// Newest first, as the real endpoint orders by ID descending.
	ordered := append([]client.MaintenanceWindowV1{}, f.windows...)
	slices.Reverse(ordered)
	if after := r.URL.Query().Get("after"); after != "" {
		ordered = lo.Filter(ordered, func(window client.MaintenanceWindowV1, _ int) bool {
			return window.Id < after
		})
	}

	page := ordered[:min(pageSize, len(ordered))]
	meta := client.PaginationMetaResultV1{PageSize: int64(pageSize)}
	// The real endpoint offers a cursor whenever the page filled, whether or not another
	// page exists.
	if len(page) == pageSize && len(page) > 0 {
		meta.After = lo.ToPtr(page[len(page)-1].Id)
	}

	writeJSON(f.t, w, client.MaintenanceWindowsListResultV1{MaintenanceWindows: page, PaginationMeta: meta})
}

func (f *fakeMaintenanceWindowsAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := r.PathValue("id")
	window, found := lo.Find(f.windows, func(window client.MaintenanceWindowV1) bool { return window.Id == id })
	if !found {
		writeNotFound(w, "maintenance window")
		return
	}

	writeJSON(f.t, w, client.MaintenanceWindowsShowResultV1{MaintenanceWindow: window})
}

func literalV2(literal string) client.EngineParamBindingV2 {
	return client.EngineParamBindingV2{Value: &client.EngineParamBindingValueV2{Literal: lo.ToPtr(literal)}}
}

var (
	maintenanceWindowStart = time.Date(2026, time.November, 7, 22, 0, 0, 0, time.UTC)
	maintenanceWindowEnd   = maintenanceWindowStart.Add(4 * time.Hour)
)

// fullMaintenanceWindow sets every attribute the data source reports.
func fullMaintenanceWindow() client.MaintenanceWindowV1 {
	return client.MaintenanceWindowV1{
		Name:    "Database migration",
		StartAt: maintenanceWindowStart,
		EndAt:   maintenanceWindowEnd,
		Lead:    client.ActorV2{User: &client.UserV2{Id: "01LEAD00000000000000000000", Name: "Lisa Karlin Curtis"}},
		AlertConditionGroups: []client.ConditionGroupV2{{Conditions: []client.ConditionV2{{
			Subject:       client.ConditionSubjectV2{Reference: "alert.title", Label: "Title"},
			Operation:     client.ConditionOperationV2{Value: "contains", Label: "contains"},
			ParamBindings: []client.EngineParamBindingV2{literalV2("database")},
		}}}},
		ShowInSidebar: true,
		ResolveOnEnd:  true,
		RerouteOnEnd:  false,
		EscalationTargets: &[]client.MaintenanceWindowEscalationTargetV1{
			{EscalationPaths: lo.ToPtr(literalV2("01ESCALATIONPATH0000000000"))},
			{Users: lo.ToPtr(literalV2("01USER00000000000000000000"))},
		},
		NotifyChannels: &[]client.MaintenanceWindowNotifyChannelV1{{
			ChannelId:   "C0DATABASE000",
			ChannelName: lo.ToPtr("database"),
			ChannelType: "public",
		}},
		NotifyStartMinutesBefore: lo.ToPtr(int64(15)),
		NotifyEndMinutesBefore:   lo.ToPtr(int64(5)),
		NotificationMessage:      lo.ToPtr("Scheduled downtime for the database migration"),
		IncidentId:               lo.ToPtr("01INCIDENT0000000000000000"),
	}
}

// bareMaintenanceWindow sets only what the API requires: the optional attributes it
// leaves out.
func bareMaintenanceWindow(name string) client.MaintenanceWindowV1 {
	return client.MaintenanceWindowV1{
		Name:                 name,
		StartAt:              maintenanceWindowStart,
		EndAt:                maintenanceWindowEnd,
		Lead:                 client.ActorV2{User: &client.UserV2{Id: "01LEAD00000000000000000000", Name: "Lisa Karlin Curtis"}},
		AlertConditionGroups: []client.ConditionGroupV2{},
	}
}

// TestIncidentMaintenanceWindowDataSourceByID reads a window through the show endpoint,
// and checks the optional attributes come back null rather than empty when the API
// leaves them out.
func TestIncidentMaintenanceWindowDataSourceByID(t *testing.T) {
	fake, url := startFakeMaintenanceWindowsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	full := fake.seed(fullMaintenanceWindow())
	bare := fake.seed(bareMaintenanceWindow("Quiet hours"))

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "incident_maintenance_window" "full" {
  id = %q
}

data "incident_maintenance_window" "bare" {
  id = %q
}
`, full.Id, bare.Id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "id", full.Id),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "name", "Database migration"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "start_at", "2026-11-07T22:00:00Z"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "end_at", "2026-11-08T02:00:00Z"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "lead_id", "01LEAD00000000000000000000"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "alert_condition_groups.0.conditions.0.subject", "alert.title"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "alert_condition_groups.0.conditions.0.operation", "contains"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "alert_condition_groups.0.conditions.0.param_bindings.0.value.literal", "database"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "show_in_sidebar", "true"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "resolve_on_end", "true"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "reroute_on_end", "false"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "escalation_targets.#", "2"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "escalation_targets.0.escalation_paths.value.literal", "01ESCALATIONPATH0000000000"),
					resource.TestCheckNoResourceAttr("data.incident_maintenance_window.full", "escalation_targets.0.users.value.literal"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "escalation_targets.1.users.value.literal", "01USER00000000000000000000"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "notify_channels.0.channel_id", "C0DATABASE000"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "notify_channels.0.channel_name", "database"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "notify_channels.0.channel_type", "public"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "notify_start_minutes_before", "15"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "notify_end_minutes_before", "5"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "notification_message", "Scheduled downtime for the database migration"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.full", "incident_id", "01INCIDENT0000000000000000"),
					resource.TestCheckNoResourceAttr("data.incident_maintenance_window.full", "force_destroy"),

					resource.TestCheckResourceAttr("data.incident_maintenance_window.bare", "id", bare.Id),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.bare", "name", "Quiet hours"),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.bare", "show_in_sidebar", "false"),
					resource.TestCheckNoResourceAttr("data.incident_maintenance_window.bare", "escalation_targets.#"),
					resource.TestCheckNoResourceAttr("data.incident_maintenance_window.bare", "notify_channels.#"),
					resource.TestCheckNoResourceAttr("data.incident_maintenance_window.bare", "notify_start_minutes_before"),
					resource.TestCheckNoResourceAttr("data.incident_maintenance_window.bare", "notification_message"),
					resource.TestCheckNoResourceAttr("data.incident_maintenance_window.bare", "incident_id"),
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
data "incident_maintenance_window" "missing" {
  id = "01FAKE00000000000000000000"
}
`,
				ExpectError: regexp.MustCompile(`Unable to read maintenance window`),
			},
		},
	})
}

// TestIncidentMaintenanceWindowDataSourcePagesToFindAName is the case a lookup by ID
// never hits: the list is paginated and carries no name filter, so a name on a later page
// is only found by walking the cursor, and a name on no page has to end the walk. Two
// windows sharing a name are reported rather than picked between.
func TestIncidentMaintenanceWindowDataSourcePagesToFindAName(t *testing.T) {
	fake, url := startFakeMaintenanceWindowsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	// One per page. The list is newest first, so the oldest window is on the last one.
	oldest := fake.seed(fullMaintenanceWindow())
	fake.seed(bareMaintenanceWindow("Quiet hours"))
	fake.seed(bareMaintenanceWindow("Quiet hours"))
	fake.pageSize = 1

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_maintenance_window" "missing" {
  name = "Nobody's window"
}
`,
				ExpectError: wrapRe(`no maintenance window found with name "Nobody's window"`),
			},
			{
				Config: `
data "incident_maintenance_window" "duplicated" {
  name = "Quiet hours"
}
`,
				ExpectError: wrapRe(`found 2 maintenance windows named "Quiet hours"; look it up by id instead`),
			},
			{
				Config: `
data "incident_maintenance_window" "both" {
  id   = "01FAKE"
  name = "Database migration"
}
`,
				ExpectError: regexp.MustCompile(`Ambiguous lookup`),
			},
			{
				Config: `
data "incident_maintenance_window" "neither" {
}
`,
				ExpectError: regexp.MustCompile(`Missing lookup`),
			},
			{
				// Last, so the config the harness tears down with is one that works.
				Config: `
data "incident_maintenance_window" "oldest" {
  name = "Database migration"
}
`,
				PreConfig: func() { fake.listCalls() },
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_maintenance_window.oldest", "id", oldest.Id),
					resource.TestCheckResourceAttr("data.incident_maintenance_window.oldest", "incident_id", "01INCIDENT0000000000000000"),
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
