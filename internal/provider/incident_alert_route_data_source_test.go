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

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakeAlertRoutesAPI is an in-memory stand-in for the two read endpoints the data source
// uses. It seeds routes directly, serves the list newest first with a cursor as the real
// endpoint does, and can play an organisation the v3 API refuses.
type fakeAlertRoutesAPI struct {
	t *testing.T

	mu        sync.Mutex
	routes    []client.AlertRouteV3
	listCount int
	nextID    int

	// pageSize caps the page the list endpoint serves, whatever page_size asks for, so a
	// test can make the data source walk the cursor with a handful of routes.
	pageSize int

	// unavailable makes every endpoint answer as the real one does for an organisation
	// still on the previous alert grouping engine.
	unavailable bool
}

func startFakeAlertRoutesAPI(t *testing.T) (*fakeAlertRoutesAPI, string) {
	t.Helper()

	fake := &fakeAlertRoutesAPI{t: t, pageSize: alertRouteListPageSize}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v3/alert_routes", fake.list)
	mux.HandleFunc("GET /v3/alert_routes/{id}", fake.show)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return fake, server.URL
}

// seed adds a route directly, assigning it the next ID.
func (f *fakeAlertRoutesAPI) seed(route client.AlertRouteV3) client.AlertRouteV3 {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	// IDs that sort in creation order, as ULIDs do.
	route.Id = fmt.Sprintf("01FAKE%020d", f.nextID)
	f.routes = append(f.routes, route)

	return route
}

// listCalls counts the list requests so far, and forgets them.
func (f *fakeAlertRoutesAPI) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	count := f.listCount
	f.listCount = 0

	return count
}

func (f *fakeAlertRoutesAPI) writeUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprintf(w, `{"type":"forbidden","status":403,"errors":[{"code":%q}]}`, alertRouteAPINotYetAvailableCode)
}

func (f *fakeAlertRoutesAPI) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.listCount++
	if f.unavailable {
		f.writeUnavailable(w)
		return
	}

	pageSize := f.pageSize
	if requested, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && requested < pageSize {
		pageSize = requested
	}

	// Newest first, as the real endpoint orders by ID descending.
	ordered := append([]client.AlertRouteV3{}, f.routes...)
	slices.Reverse(ordered)
	if after := r.URL.Query().Get("after"); after != "" {
		ordered = lo.Filter(ordered, func(route client.AlertRouteV3, _ int) bool {
			return route.Id < after
		})
	}

	page := lo.Map(ordered[:min(pageSize, len(ordered))], func(route client.AlertRouteV3, _ int) client.AlertRouteSlimV3 {
		return client.AlertRouteSlimV3{Id: route.Id, Name: route.Name, Enabled: route.Enabled}
	})
	meta := client.PaginationMetaResultV3{PageSize: int64(pageSize)}
	// The real endpoint offers a cursor whenever the page filled, whether or not another
	// page exists.
	if len(page) == pageSize && len(page) > 0 {
		meta.After = lo.ToPtr(page[len(page)-1].Id)
	}

	writeJSON(f.t, w, client.AlertRoutesListResultV3{AlertRoutes: page, PaginationMeta: meta})
}

func (f *fakeAlertRoutesAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.unavailable {
		f.writeUnavailable(w)
		return
	}

	id := r.PathValue("id")
	route, found := lo.Find(f.routes, func(route client.AlertRouteV3) bool { return route.Id == id })
	if !found {
		writeNotFound(w, "alert route")
		return
	}

	writeJSON(f.t, w, client.AlertRoutesShowResultV3{AlertRoute: route})
}

func literalV3(literal string) client.EngineParamBindingV3 {
	return client.EngineParamBindingV3{Value: &client.EngineParamBindingValueV3{Literal: lo.ToPtr(literal)}}
}

// fullAlertRoute uses every block the data source reports, so a schema that misses one
// fails here rather than on a customer's route.
func fullAlertRoute() client.AlertRouteV3 {
	return client.AlertRouteV3{
		Name:      "Production incidents",
		Enabled:   true,
		IsPrivate: false,
		AlertSources: []client.AlertRouteAlertSourceV3{{
			AlertSourceId: "01SOURCE000000000000000000",
			ConditionGroups: []client.ConditionGroupV3{{Conditions: []client.ConditionV3{{
				Subject:       client.ConditionSubjectV3{Reference: "alert.priority", Label: "Priority"},
				Operation:     client.ConditionOperationV3{Value: "one_of", Label: "is one of"},
				ParamBindings: []client.EngineParamBindingV3{{ArrayValue: &[]client.EngineParamBindingValueV3{{Literal: lo.ToPtr("01PRIORITY0000000000000000")}}}},
			}}}},
		}},
		ConditionGroups: []client.ConditionGroupV3{{Conditions: []client.ConditionV3{{
			Subject:       client.ConditionSubjectV3{Reference: "alert.title", Label: "Title"},
			Operation:     client.ConditionOperationV3{Value: "contains", Label: "contains"},
			ParamBindings: []client.EngineParamBindingV3{literalV3("database")},
		}}}},
		EscalationConfig: client.AlertRouteEscalationConfigV3{
			AutoCancelEscalations: true,
			EscalationTargets: []client.AlertRouteEscalationTargetV3{
				{EscalationPaths: lo.ToPtr(literalV3("01ESCALATIONPATH0000000000"))},
			},
			WhenAlertJoinsGroup: &client.AlertRouteWhenAlertJoinsGroupV3{
				Mode:               client.AlertRouteWhenAlertJoinsGroupV3ModeOnEachNewAlert,
				GracePeriodSeconds: lo.ToPtr(int32(60)),
			},
		},
		GroupingConfig: client.AlertGroupingConfigV3{Default: client.GroupingSettingsV3{
			Enabled:       true,
			AiEnabled:     lo.ToPtr(false),
			GroupingKeys:  &[]client.GroupingKeyV3{{Reference: "alert.title"}},
			WindowSeconds: lo.ToPtr(int32(1800)),
			WindowType:    lo.ToPtr(client.GroupingSettingsV3WindowTypeRolling),
		}},
		MessageConfig: client.AlertMessageConfigV3{
			Destinations: []client.AlertMessageDestinationV3{{
				ConditionGroups: []client.ConditionGroupV3{},
				SlackTargets: &client.AlertRouteChannelTargetV3{
					Binding:            literalV3("C0ALERTS00000"),
					ChannelVisibility:  "public",
					GroupAlertsSummary: lo.ToPtr(true),
				},
			}},
			Template: lo.ToPtr(literalV3("01MESSAGETEMPLATE000000000")),
		},
		IncidentConfig: client.AlertRouteIncidentConfigV3{
			Enabled:            true,
			AutoDeclineEnabled: lo.ToPtr(true),
			ConditionGroups:    &[]client.ConditionGroupV3{},
			Template: &client.AlertRouteIncidentTemplateV3{
				Name:    client.AlertRouteAutoGeneratedTemplateBindingV3{Binding: lo.ToPtr(literalV3("Database alert"))},
				Summary: &client.AlertRouteAutoGeneratedTemplateBindingV3{Autogenerated: true},
				Severity: &client.AlertRouteSeverityBindingV3{
					Binding:       lo.ToPtr(literalV3("01SEVERITY0000000000000000")),
					MergeStrategy: client.AlertRouteSeverityBindingV3MergeStrategyMax,
				},
				IncidentType: &client.AlertRouteTemplateBindingV3{Binding: lo.ToPtr(literalV3("01INCIDENTTYPE000000000000"))},
				CustomFields: &[]client.AlertRouteCustomFieldBindingV3{{
					CustomFieldId: "01CUSTOMFIELD0000000000000",
					Binding:       literalV3("01OPTION000000000000000000"),
					MergeStrategy: client.AlertRouteCustomFieldBindingV3MergeStrategyFirstWins,
				}},
			},
		},
		OwningTeamIds: &[]string{"01TEAM00000000000000000000"},
	}
}

// bareAlertRoute groups nothing, creates no incidents and posts nowhere: the optional
// blocks the API leaves out.
func bareAlertRoute(name string) client.AlertRouteV3 {
	return client.AlertRouteV3{
		Name:         name,
		Enabled:      false,
		IsPrivate:    true,
		AlertSources: []client.AlertRouteAlertSourceV3{{AlertSourceId: "01SOURCE000000000000000000", ConditionGroups: []client.ConditionGroupV3{}}},
		EscalationConfig: client.AlertRouteEscalationConfigV3{
			EscalationTargets: []client.AlertRouteEscalationTargetV3{{Users: lo.ToPtr(literalV3("01USER00000000000000000000"))}},
		},
		GroupingConfig: client.AlertGroupingConfigV3{Default: client.GroupingSettingsV3{Enabled: false}},
		IncidentConfig: client.AlertRouteIncidentConfigV3{Enabled: false},
	}
}

// TestIncidentAlertRouteDataSourceByID reads a route through the show endpoint, and checks
// the optional blocks come back null rather than empty when the API leaves them out.
func TestIncidentAlertRouteDataSourceByID(t *testing.T) {
	fake, url := startFakeAlertRoutesAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	full := fake.seed(fullAlertRoute())
	bare := fake.seed(bareAlertRoute("Private pager"))

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "incident_alert_route" "full" {
  id = %q
}

data "incident_alert_route" "bare" {
  id = %q
}
`, full.Id, bare.Id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "id", full.Id),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "name", "Production incidents"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "enabled", "true"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "is_private", "false"),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_route.full", "alert_sources.*", map[string]string{
						"alert_source_id":                                                        "01SOURCE000000000000000000",
						"condition_groups.0.conditions.0.subject":                                "alert.priority",
						"condition_groups.0.conditions.0.operation":                              "one_of",
						"condition_groups.0.conditions.0.param_bindings.0.array_value.0.literal": "01PRIORITY0000000000000000",
					}),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "condition_groups.0.conditions.0.param_bindings.0.value.literal", "database"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "escalation_config.auto_cancel_escalations", "true"),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_route.full", "escalation_config.escalation_targets.*", map[string]string{
						"escalation_paths.value.literal": "01ESCALATIONPATH0000000000",
					}),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "escalation_config.when_alert_joins_group.mode", "on_each_new_alert"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "escalation_config.when_alert_joins_group.grace_period_seconds", "60"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "grouping_config.default.enabled", "true"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "grouping_config.default.ai_enabled", "false"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "grouping_config.default.window_seconds", "1800"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "grouping_config.default.window_type", "rolling"),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_route.full", "grouping_config.default.grouping_keys.*", map[string]string{
						"reference": "alert.title",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_route.full", "message_config.destinations.*", map[string]string{
						"slack_targets.binding.value.literal": "C0ALERTS00000",
						"slack_targets.channel_visibility":    "public",
						"slack_targets.group_alerts_summary":  "true",
					}),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "message_config.template.value.literal", "01MESSAGETEMPLATE000000000"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.enabled", "true"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.auto_decline_enabled", "true"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.template.name.autogenerated", "false"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.template.name.value.literal", "Database alert"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.template.summary.autogenerated", "true"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.template.severity.merge_strategy", "max"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.template.severity.binding.value.literal", "01SEVERITY0000000000000000"),
					resource.TestCheckResourceAttr("data.incident_alert_route.full", "incident_config.template.incident_type.value.literal", "01INCIDENTTYPE000000000000"),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_route.full", "incident_config.template.custom_fields.*", map[string]string{
						"custom_field_id":       "01CUSTOMFIELD0000000000000",
						"binding.value.literal": "01OPTION000000000000000000",
						"merge_strategy":        "first-wins",
					}),
					resource.TestCheckTypeSetElemAttr("data.incident_alert_route.full", "owning_team_ids.*", "01TEAM00000000000000000000"),

					resource.TestCheckResourceAttr("data.incident_alert_route.bare", "id", bare.Id),
					resource.TestCheckResourceAttr("data.incident_alert_route.bare", "name", "Private pager"),
					resource.TestCheckResourceAttr("data.incident_alert_route.bare", "enabled", "false"),
					resource.TestCheckResourceAttr("data.incident_alert_route.bare", "is_private", "true"),
					resource.TestCheckTypeSetElemNestedAttrs("data.incident_alert_route.bare", "escalation_config.escalation_targets.*", map[string]string{
						"users.value.literal": "01USER00000000000000000000",
					}),
					resource.TestCheckNoResourceAttr("data.incident_alert_route.bare", "escalation_config.when_alert_joins_group.mode"),
					resource.TestCheckResourceAttr("data.incident_alert_route.bare", "grouping_config.default.enabled", "false"),
					resource.TestCheckNoResourceAttr("data.incident_alert_route.bare", "grouping_config.default.window_seconds"),
					resource.TestCheckNoResourceAttr("data.incident_alert_route.bare", "message_config.template.value.literal"),
					resource.TestCheckResourceAttr("data.incident_alert_route.bare", "incident_config.enabled", "false"),
					resource.TestCheckNoResourceAttr("data.incident_alert_route.bare", "incident_config.auto_decline_enabled"),
					resource.TestCheckNoResourceAttr("data.incident_alert_route.bare", "incident_config.template.name.autogenerated"),
					resource.TestCheckNoResourceAttr("data.incident_alert_route.bare", "owning_team_ids.#"),
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
data "incident_alert_route" "missing" {
  id = "01FAKE00000000000000000000"
}
`,
				ExpectError: regexp.MustCompile(`Unable to read alert route`),
			},
		},
	})
}

// TestIncidentAlertRouteDataSourcePagesToFindAName is the case a lookup by ID never hits:
// the list is paginated and carries no name filter, so a name on a later page is only
// found by walking the cursor, and a name on no page has to end the walk. Two routes
// sharing a name are reported rather than picked between.
func TestIncidentAlertRouteDataSourcePagesToFindAName(t *testing.T) {
	fake, url := startFakeAlertRoutesAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	// One per page. The list is newest first, so the oldest route is on the last one.
	oldest := fake.seed(fullAlertRoute())
	fake.seed(bareAlertRoute("Staging alerts"))
	fake.seed(bareAlertRoute("Staging alerts"))
	fake.pageSize = 1

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_alert_route" "missing" {
  name = "Nobody's alerts"
}
`,
				ExpectError: wrapRe(`no alert route found with name "Nobody's alerts"`),
			},
			{
				Config: `
data "incident_alert_route" "duplicated" {
  name = "Staging alerts"
}
`,
				ExpectError: wrapRe(`found 2 alert routes named "Staging alerts"; look it up by id instead`),
			},
			{
				Config: `
data "incident_alert_route" "both" {
  id   = "01FAKE"
  name = "Production incidents"
}
`,
				ExpectError: regexp.MustCompile(`Ambiguous lookup`),
			},
			{
				Config: `
data "incident_alert_route" "neither" {
}
`,
				ExpectError: regexp.MustCompile(`Missing lookup`),
			},
			{
				// Last, so the config the harness tears down with is one that works.
				Config: `
data "incident_alert_route" "oldest" {
  name = "Production incidents"
}
`,
				PreConfig: func() { fake.listCalls() },
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_alert_route.oldest", "id", oldest.Id),
					resource.TestCheckResourceAttr("data.incident_alert_route.oldest", "grouping_config.default.window_seconds", "1800"),
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

// TestIncidentAlertRouteDataSourceUnavailable is an organisation still on the previous
// alert grouping engine, whose routes the v3 API refuses. The data source has no older
// shape to fall back to, so it says why rather than reporting a bare 403.
func TestIncidentAlertRouteDataSourceUnavailable(t *testing.T) {
	fake, url := startFakeAlertRoutesAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	route := fake.seed(bareAlertRoute("Staging alerts"))
	fake.unavailable = true

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "incident_alert_route" "by_id" {
  id = %q
}
`, route.Id),
				ExpectError: wrapRe(`has not moved to the new alert grouping engine`),
			},
			{
				Config: `
data "incident_alert_route" "by_name" {
  name = "Staging alerts"
}
`,
				ExpectError: wrapRe(`has not moved to the new alert grouping engine`),
			},
		},
	})
}
