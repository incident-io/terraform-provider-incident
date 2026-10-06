package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakeStatusPageStructuresAPI is an in-memory stand-in for the show and set structure
// endpoints and the managed-resources claim, so the structure resource can be driven
// through Terraform without an account. Like the real API it:
//
//   - rejects a component that isn't one of the organisation's, against its field
//   - keeps a group the payload names by ID, and rejects an ID that isn't on the page
//   - assigns an ID to a group the payload doesn't name
//   - keeps the display settings of a placement the payload leaves out, defaults a new
//     one's, and keeps the page's uptime mode when the payload has none
//   - keeps a kept group's description when omitted, and clears it on an empty string
//   - returns each component with its name, as the read shape does
//   - reports who manages a structure from the annotations its last claim carried
type fakeStatusPageStructuresAPI struct {
	t *testing.T

	mu         sync.Mutex
	components map[string]string // component ID -> name
	pages      map[string]fakeStatusPage
	// claims holds the annotations each page's last managed-resource claim sent.
	claims map[string]map[string]string
	nextID int
}

type fakeStatusPage struct {
	structure         client.StatusPageStructureV2
	displayUptimeMode client.StatusPagesShowStatusPageStructureResultV2DisplayUptimeMode
}

func startFakeStatusPageStructuresAPI(t *testing.T) (*fakeStatusPageStructuresAPI, string) {
	t.Helper()

	fake := &fakeStatusPageStructuresAPI{
		t:          t,
		components: map[string]string{},
		pages:      map[string]fakeStatusPage{},
		claims:     map[string]map[string]string{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/status_page_structures/{id}", fake.show)
	mux.HandleFunc("PUT /v2/status_page_structures/{id}", fake.set)
	mux.HandleFunc("POST /v2/managed_resources", fake.claim)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return fake, server.URL
}

// seedComponent adds a component the structure can place.
func (f *fakeStatusPageStructuresAPI) seedComponent(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	id := fmt.Sprintf("01FAKECOMPONENT%014d", f.nextID)
	f.components[id] = name

	return id
}

// seedPage adds a page with a structure already on it, as the dashboard would have left it.
func (f *fakeStatusPageStructuresAPI) seedPage(structure client.StatusPageStructureV2, displayUptimeMode client.StatusPagesShowStatusPageStructureResultV2DisplayUptimeMode) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	id := fmt.Sprintf("01FAKEPAGE%019d", f.nextID)
	f.pages[id] = fakeStatusPage{structure: structure, displayUptimeMode: displayUptimeMode}

	return id
}

// groupIDs returns the IDs of a page's groups, in order.
func (f *fakeStatusPageStructuresAPI) groupIDs(pageID string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var ids []string
	for _, item := range f.pages[pageID].structure.Items {
		if item.Group != nil {
			ids = append(ids, item.Group.Id)
		}
	}

	return ids
}

// managedBy reports who the fake thinks manages a page's structure, from its last claim.
func (f *fakeStatusPageStructuresAPI) managedBy(pageID string) client.ManagementMetaV2ManagedBy {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.management(pageID).ManagedBy
}

// management builds a page's management_meta. The caller holds the lock.
func (f *fakeStatusPageStructuresAPI) management(pageID string) client.ManagementMetaV2 {
	annotations := f.claims[pageID]
	if annotations == nil {
		annotations = map[string]string{}
	}

	managedBy := client.ManagementMetaV2ManagedByDashboard
	if _, ok := annotations["incident.io/terraform/version"]; ok {
		managedBy = client.ManagementMetaV2ManagedByTerraform
	}

	return client.ManagementMetaV2{Annotations: annotations, ManagedBy: managedBy}
}

func (f *fakeStatusPageStructuresAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	pageID := r.PathValue("id")
	page, ok := f.pages[pageID]
	if !ok {
		writeNotFound(w, "status page")
		return
	}

	writeJSON(f.t, w, client.StatusPagesShowStatusPageStructureResultV2{
		CurrentStructure:  page.structure,
		DisplayUptimeMode: page.displayUptimeMode,
		ManagementMeta:    f.management(pageID),
	})
}

func (f *fakeStatusPageStructuresAPI) set(w http.ResponseWriter, r *http.Request) {
	var payload client.StatusPagesSetStatusPageStructurePayloadV2
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.t.Fatalf("decoding set structure payload: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	pageID := r.PathValue("id")
	page, ok := f.pages[pageID]
	if !ok {
		writeNotFound(w, "status page")
		return
	}

	// What the page has now, so an omitted setting can keep it.
	existingGroups := map[string]client.StatusPageStructureGroupV2{}
	placements := map[string]client.StatusPageStructureComponentV2{}
	for _, item := range page.structure.Items {
		if item.Component != nil {
			placements[item.Component.ComponentId] = *item.Component
		}
		if item.Group != nil {
			existingGroups[item.Group.Id] = *item.Group
			for _, component := range item.Group.Components {
				placements[component.ComponentId] = component
			}
		}
	}

	component := func(field string, payload client.StatusPageStructureComponentPayloadV2) (client.StatusPageStructureComponentV2, bool) {
		name, ok := f.components[payload.ComponentId]
		if !ok {
			writeValidationError(w, field, "That component does not exist")
			return client.StatusPageStructureComponentV2{}, false
		}

		// A first placement is shown, with uptime.
		got := client.StatusPageStructureComponentV2{ComponentId: payload.ComponentId, Name: name, Hidden: false, DisplayUptime: true}
		if previous, placed := placements[payload.ComponentId]; placed {
			got.Hidden, got.DisplayUptime = previous.Hidden, previous.DisplayUptime
		}
		if payload.Hidden != nil {
			got.Hidden = *payload.Hidden
		}
		if payload.DisplayUptime != nil {
			got.DisplayUptime = *payload.DisplayUptime
		}

		return got, true
	}

	structure := client.StatusPageStructureV2{Items: []client.StatusPageStructureItemV2{}}
	for idx, item := range payload.Items {
		if item.Component != nil {
			got, ok := component(fmt.Sprintf("items.%d.component.component_id", idx), *item.Component)
			if !ok {
				return
			}
			structure.Items = append(structure.Items, client.StatusPageStructureItemV2{Component: &got})
			continue
		}

		group := client.StatusPageStructureGroupV2{Name: item.Group.Name, Components: []client.StatusPageStructureComponentV2{}}
		var previous *client.StatusPageStructureGroupV2
		if item.Group.Id != nil {
			kept, ok := existingGroups[*item.Group.Id]
			if !ok {
				writeValidationError(w, fmt.Sprintf("items.%d.group.id", idx), "That group is not on this page")
				return
			}
			previous = &kept
			group.Id = *item.Group.Id
		} else {
			f.nextID++
			group.Id = fmt.Sprintf("01FAKEGROUP%018d", f.nextID)
		}
		for componentIdx, payload := range item.Group.Components {
			got, ok := component(fmt.Sprintf("items.%d.group.components.%d.component_id", idx, componentIdx), payload)
			if !ok {
				return
			}
			group.Components = append(group.Components, got)
		}

		// A new group's defaults follow its members; a kept group keeps its settings.
		group.Hidden = lo.EveryBy(group.Components, func(c client.StatusPageStructureComponentV2) bool { return c.Hidden })
		group.DisplayAggregatedUptime = lo.SomeBy(group.Components, func(c client.StatusPageStructureComponentV2) bool { return !c.Hidden && c.DisplayUptime })
		if previous != nil {
			group.Hidden, group.DisplayAggregatedUptime, group.Description = previous.Hidden, previous.DisplayAggregatedUptime, previous.Description
		}
		if item.Group.Hidden != nil {
			group.Hidden = *item.Group.Hidden
		}
		if item.Group.DisplayAggregatedUptime != nil {
			group.DisplayAggregatedUptime = *item.Group.DisplayAggregatedUptime
		}
		if item.Group.Description != nil {
			group.Description = normaliseDescription(item.Group.Description)
		}

		structure.Items = append(structure.Items, client.StatusPageStructureItemV2{Group: &group})
	}

	page.structure = structure
	if payload.DisplayUptimeMode != nil {
		page.displayUptimeMode = client.StatusPagesShowStatusPageStructureResultV2DisplayUptimeMode(*payload.DisplayUptimeMode)
	}
	f.pages[pageID] = page

	writeJSON(f.t, w, client.StatusPagesSetStatusPageStructureResultV2{
		CurrentStructure:  structure,
		DisplayUptimeMode: client.StatusPagesSetStatusPageStructureResultV2DisplayUptimeMode(page.displayUptimeMode),
		ManagementMeta:    f.management(pageID),
	})
}

func (f *fakeStatusPageStructuresAPI) claim(w http.ResponseWriter, r *http.Request) {
	var payload client.ManagedResourcesCreateManagedResourcePayloadV2
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.t.Fatalf("decoding claim payload: %v", err)
	}
	if payload.ResourceType != client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageStructure {
		f.t.Fatalf("claim for unexpected resource type %q", payload.ResourceType)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.pages[payload.ResourceId]; !ok {
		writeNotFound(w, "status page")
		return
	}

	f.claims[payload.ResourceId] = payload.Annotations
	management := f.management(payload.ResourceId)

	writeJSONStatus(f.t, w, http.StatusCreated, client.ManagedResourcesCreateManagedResourceResultV2{
		ManagedResource: client.ManagedResourceV2{
			Annotations:  management.Annotations,
			ManagedBy:    client.ManagedResourceV2ManagedBy(management.ManagedBy),
			ResourceId:   payload.ResourceId,
			ResourceType: client.ManagedResourceV2ResourceTypeStatusPageStructure,
		},
	})
}

// writeValidationError answers as the API does for a bad field: a 422 naming the field.
func writeValidationError(w http.ResponseWriter, field, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_, _ = fmt.Fprintf(w, `{"type":"validation_error","status":422,"errors":[{"code":"invalid_value","message":%q,"source":{"pointer":%q}}]}`, message, field)
}

// structureTestConfig renders the resource with the given items, each either a component
// or a group. Settings left empty are left out of the config; a boolean one is "true" or "false".
//
// Components names incident_status_page_component resources to create alongside, for a
// real account where the components have to exist. An item places one by Expr.
type structureTestConfig struct {
	PageID            string
	UnlockInDashboard bool
	DisplayUptimeMode string
	Components        []string
	Items             []structureTestItem
}

type structureTestItem struct {
	Component *structureTestComponent
	Group     *structureTestGroup
}

type structureTestComponent struct {
	// ID is a literal component ID, quoted into the config. Expr is an HCL expression for
	// one instead, such as a component resource's id.
	ID            string
	Expr          string
	Hidden        string
	DisplayUptime string
}

type structureTestGroup struct {
	Name                    string
	Description             string
	Hidden                  string
	DisplayAggregatedUptime string
	Components              []structureTestComponent
}

func componentItem(id string) structureTestItem {
	return structureTestItem{Component: &structureTestComponent{ID: id}}
}

// componentRef places a component the config creates, by its resource name.
func componentRef(name string) structureTestItem {
	return structureTestItem{Component: &structureTestComponent{Expr: componentExpr(name)}}
}

func componentExpr(name string) string {
	return "incident_status_page_component." + name + ".id"
}

func groupItem(name string, componentIDs ...string) structureTestItem {
	return structureTestItem{Group: &structureTestGroup{
		Name: name,
		Components: lo.Map(componentIDs, func(id string, _ int) structureTestComponent {
			return structureTestComponent{ID: id}
		}),
	}}
}

func testIncidentStatusPageStructureConfig(config structureTestConfig) string {
	return testRunTemplate("incident_status_page_structure", `
{{- define "component" }}{ component_id = {{ if .Expr }}{{ .Expr }}{{ else }}{{ .ID | quote }}{{ end }}
{{- if .Hidden }}, hidden = {{ .Hidden }}{{ end }}
{{- if .DisplayUptime }}, display_uptime = {{ .DisplayUptime }}{{ end }} }{{ end -}}
{{ range .Components }}
resource "incident_status_page_component" {{ . | quote }} {
  name = {{ printf "Structure test %s" . | stableSuffix | quote }}
}
{{ end }}
resource "incident_status_page_structure" "test" {
  status_page_id = {{ .PageID | quote }}
  {{ if .UnlockInDashboard }}unlock_in_dashboard = true{{ end }}
  {{ if .DisplayUptimeMode }}display_uptime_mode = {{ .DisplayUptimeMode | quote }}{{ end }}
  items = [
{{- range .Items }}
{{- if .Component }}
    {{ template "component" .Component }},
{{- else }}
    { group = {
      name = {{ .Group.Name | quote }}
      {{ if .Group.Description }}description = {{ .Group.Description | quote }}{{ end }}
      {{ if .Group.Hidden }}hidden = {{ .Group.Hidden }}{{ end }}
      {{ if .Group.DisplayAggregatedUptime }}display_aggregated_uptime = {{ .Group.DisplayAggregatedUptime }}{{ end }}
      components = [{{ range $i, $c := .Group.Components }}{{ if $i }}, {{ end }}{{ template "component" $c }}{{ end }}]
    } },
{{- end }}
{{- end }}
  ]
}
`, config)
}

// TestIncidentStatusPageStructureResourceLifecycle adopts a structure the dashboard left,
// keeping its group's ID and the display settings the config doesn't mention; imports it;
// takes over the display settings; hands the structure back to the dashboard and claims it
// again; watches a rename make a new group; and sees a destroy leave the structure, and
// its claim, in place.
func TestIncidentStatusPageStructureResourceLifecycle(t *testing.T) {
	fake, url := startFakeStatusPageStructuresAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	api := fake.seedComponent("API")
	web := fake.seedComponent("Website")
	db := fake.seedComponent("Database")
	page := fake.seedPage(client.StatusPageStructureV2{Items: []client.StatusPageStructureItemV2{
		{Group: &client.StatusPageStructureGroupV2{
			Id:                      "core-services",
			Name:                    "Core",
			Description:             lo.ToPtr("The services behind everything"),
			Hidden:                  false,
			DisplayAggregatedUptime: false,
			Components:              []client.StatusPageStructureComponentV2{{ComponentId: api, Name: "API", Hidden: true, DisplayUptime: false}},
		}},
		{Component: &client.StatusPageStructureComponentV2{ComponentId: web, Name: "Website", Hidden: false, DisplayUptime: true}},
	}}, client.StatusPagesShowStatusPageStructureResultV2DisplayUptimeModeChartOnly)

	const address = "incident_status_page_structure.test"

	checkManagedBy := func(want client.ManagementMetaV2ManagedBy) resource.TestCheckFunc {
		return resource.TestCheckResourceAttrWith(address, "id", func(id string) error {
			if got := fake.managedBy(id); got != want {
				return fmt.Errorf("structure of page %s is managed by %q, want %q", id, got, want)
			}

			return nil
		})
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Adopted: the dashboard's group is kept by name, so it keeps its ID and
				// settings while gaining a member. Settings the config leaves out stay as
				// the dashboard had them, bar the description, which the config owns.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: page, Items: []structureTestItem{
					groupItem("Core", api, db),
					componentItem(web),
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", page),
					resource.TestCheckNoResourceAttr(address, "unlock_in_dashboard"),
					resource.TestCheckResourceAttr(address, "display_uptime_mode", "chart_only"),
					resource.TestCheckResourceAttr(address, "items.#", "2"),
					resource.TestCheckResourceAttr(address, "items.0.group.id", "core-services"),
					resource.TestCheckResourceAttr(address, "items.0.group.name", "Core"),
					resource.TestCheckNoResourceAttr(address, "items.0.group.description"),
					resource.TestCheckResourceAttr(address, "items.0.group.hidden", "false"),
					resource.TestCheckResourceAttr(address, "items.0.group.display_aggregated_uptime", "false"),
					resource.TestCheckResourceAttr(address, "items.0.group.components.#", "2"),
					// Already placed: kept hidden, without uptime.
					resource.TestCheckResourceAttr(address, "items.0.group.components.0.hidden", "true"),
					resource.TestCheckResourceAttr(address, "items.0.group.components.0.display_uptime", "false"),
					// Newly placed: the defaults.
					resource.TestCheckResourceAttr(address, "items.0.group.components.1.component_id", db),
					resource.TestCheckResourceAttr(address, "items.0.group.components.1.hidden", "false"),
					resource.TestCheckResourceAttr(address, "items.0.group.components.1.display_uptime", "true"),
					// A group item carries no loose-component settings.
					resource.TestCheckNoResourceAttr(address, "items.0.hidden"),
					resource.TestCheckNoResourceAttr(address, "items.0.display_uptime"),
					resource.TestCheckResourceAttr(address, "items.1.component_id", web),
					resource.TestCheckResourceAttr(address, "items.1.hidden", "false"),
					resource.TestCheckResourceAttr(address, "items.1.display_uptime", "true"),
					resource.TestCheckNoResourceAttr(address, "items.1.group"),
					checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     page,
				ImportStateVerify: true,
				// Whether Terraform claims a structure is configuration, which an import
				// has none of.
				ImportStateVerifyIgnore: []string{"unlock_in_dashboard"},
			},
			{
				// The config takes over the display settings, and the group moves below the
				// loose component while keeping its ID.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: page, DisplayUptimeMode: "nothing", Items: []structureTestItem{
					{Component: &structureTestComponent{ID: web, Hidden: "true"}},
					{Group: &structureTestGroup{
						Name:                    "Core",
						Description:             "Backend services",
						DisplayAggregatedUptime: "true",
						Components: []structureTestComponent{
							{ID: db},
							{ID: api, Hidden: "false", DisplayUptime: "true"},
						},
					}},
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "display_uptime_mode", "nothing"),
					resource.TestCheckResourceAttr(address, "items.0.component_id", web),
					resource.TestCheckResourceAttr(address, "items.0.hidden", "true"),
					resource.TestCheckResourceAttr(address, "items.0.display_uptime", "true"),
					resource.TestCheckResourceAttr(address, "items.1.group.id", "core-services"),
					resource.TestCheckResourceAttr(address, "items.1.group.description", "Backend services"),
					resource.TestCheckResourceAttr(address, "items.1.group.display_aggregated_uptime", "true"),
					resource.TestCheckResourceAttr(address, "items.1.group.components.0.component_id", db),
					resource.TestCheckResourceAttr(address, "items.1.group.components.1.hidden", "false"),
					resource.TestCheckResourceAttr(address, "items.1.group.components.1.display_uptime", "true"),
					checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
				),
			},
			{
				// Handed back to the dashboard: the update unclaims it. The settings set
				// last time and now left out are kept.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: page, UnlockInDashboard: true, Items: []structureTestItem{
					componentItem(web),
					{Group: &structureTestGroup{Name: "Core", Description: "Backend services", Components: []structureTestComponent{{ID: db}, {ID: api}}}},
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "unlock_in_dashboard", "true"),
					resource.TestCheckResourceAttr(address, "display_uptime_mode", "nothing"),
					resource.TestCheckResourceAttr(address, "items.0.hidden", "true"),
					resource.TestCheckResourceAttr(address, "items.1.group.display_aggregated_uptime", "true"),
					checkManagedBy(client.ManagementMetaV2ManagedByDashboard),
				),
			},
			{
				// Renamed, and claimed again: a group is matched by name, so this is a new
				// group with a new ID and no description.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: page, Items: []structureTestItem{
					componentItem(web),
					groupItem("Backend", db, api),
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "items.1.group.name", "Backend"),
					resource.TestCheckNoResourceAttr(address, "items.1.group.description"),
					resource.TestCheckResourceAttrWith(address, "items.1.group.id", func(id string) error {
						if id == "core-services" {
							return fmt.Errorf("renamed group kept the old ID")
						}
						if got := fake.groupIDs(page); len(got) != 1 || got[0] != id {
							return fmt.Errorf("fake has groups %v, state has %s", got, id)
						}

						return nil
					}),
					checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
				),
			},
			{
				// Destroyed: the structure stays on the page, and so does the claim, since
				// it is still what Terraform last applied.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: page, Items: []structureTestItem{
					componentItem(web),
					groupItem("Backend", db, api),
				}}),
				Destroy: true,
			},
			{
				PreConfig: func() {
					if got := fake.managedBy(page); got != client.ManagementMetaV2ManagedByTerraform {
						t.Errorf("after destroy, structure of page %s is managed by %q, want terraform", page, got)
					}
					if got := fake.groupIDs(page); len(got) != 1 {
						t.Errorf("after destroy, page %s has groups %v, want the one left in place", page, got)
					}
				},
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: page, Items: []structureTestItem{
					componentItem(web),
					groupItem("Backend", db, api),
				}}),
			},
		},
	})
}

// TestIncidentStatusPageStructureResourceValidation covers the plan-time shape checks and
// the API's refusal of an unknown component reaching the practitioner with its reason.
func TestIncidentStatusPageStructureResourceValidation(t *testing.T) {
	fake, url := startFakeStatusPageStructuresAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	api := fake.seedComponent("API")
	page := fake.seedPage(client.StatusPageStructureV2{Items: []client.StatusPageStructureItemV2{
		{Component: &client.StatusPageStructureComponentV2{ComponentId: api, Name: "API", DisplayUptime: true}},
	}}, client.StatusPagesShowStatusPageStructureResultV2DisplayUptimeModeChartAndPercentage)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "incident_status_page_structure" "test" {
  status_page_id = "` + page + `"
  items = [
    { component_id = "` + api + `", group = { name = "Core", components = [{ component_id = "` + api + `" }] } },
  ]
}
`,
				ExpectError: regexp.MustCompile(`Ambiguous item`),
			},
			{
				Config: `
resource "incident_status_page_structure" "test" {
  status_page_id = "` + page + `"
  items = [
    { hidden = true, group = { name = "Core", components = [{ component_id = "` + api + `" }] } },
  ]
}
`,
				ExpectError: regexp.MustCompile(`Display flags on a group item`),
			},
			{
				Config: `
resource "incident_status_page_structure" "test" {
  status_page_id = "` + page + `"
  items = [{ hidden = true }]
}
`,
				ExpectError: regexp.MustCompile(`Empty item`),
			},
			{
				Config: `
resource "incident_status_page_structure" "test" {
  status_page_id = "` + page + `"
  items = [{}]
}
`,
				ExpectError: regexp.MustCompile(`Empty item`),
			},
			{
				Config: `
resource "incident_status_page_structure" "test" {
  status_page_id = "` + page + `"
  display_uptime_mode = "sparkline"
  items = [{ component_id = "` + api + `" }]
}
`,
				ExpectError: wrapRe(`display_uptime_mode value must be one of`),
			},
			{
				Config: `
resource "incident_status_page_structure" "test" {
  status_page_id = "` + page + `"
  items = [{ component_id = "01NOTACOMPONENT" }]
}
`,
				ExpectError: wrapRe(`That component does not exist`),
			},
			{
				// Still a valid page afterwards: the failed sets changed nothing.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: page, Items: []structureTestItem{
					componentItem(api),
				}}),
				Check: resource.TestCheckResourceAttr("incident_status_page_structure.test", "items.0.component_id", api),
			},
		},
	})
}

// TestAccIncidentStatusPageStructureResource runs the lifecycle against a real standalone
// page, named by TF_ACC_STATUS_PAGE_ID. A page has one structure, and CI's matrix legs run
// at the same time, so each leg has a page of its own; see .github/workflows/test.yml.
//
// The test replaces the page's structure with components it creates, and puts the page's
// original items back at the end so its own components can be archived: incident.io
// refuses to archive a component a structure still places, and destroying the structure
// resource leaves the structure on the page. That last apply also unlocks the page in the
// dashboard, since a destroy keeps the claim.
func TestAccIncidentStatusPageStructureResource(t *testing.T) {
	pageID := os.Getenv("TF_ACC_STATUS_PAGE_ID")
	if pageID == "" {
		t.Skip("TF_ACC_STATUS_PAGE_ID is not set: skipping, as the test needs a standalone status page of its own")
	}
	// Skip before reading the page, which needs the client testAccPreCheck builds.
	testAccPreCheck(t)

	original, originalMode := testAccStatusPageStructure(t, pageID)
	if len(original) == 0 {
		t.Fatalf("status page %s has an empty structure, and the test needs items to put back at the end", pageID)
	}

	const address = "incident_status_page_structure.test"
	components := []string{"web", "api", "db"}

	// managedBy asks the API who manages the page's structure.
	managedBy := func() (client.ManagementMetaV2ManagedBy, error) {
		result, err := testClient.StatusPagesV2ShowStatusPageStructureWithResponse(context.Background(), pageID)
		if err != nil {
			return "", err
		}
		if result.JSON200 == nil {
			return "", fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}

		return result.JSON200.ManagementMeta.ManagedBy, nil
	}
	checkManagedBy := func(want client.ManagementMetaV2ManagedBy) resource.TestCheckFunc {
		return func(*terraform.State) error {
			got, err := managedBy()
			if err != nil {
				return err
			}
			if got != want {
				return fmt.Errorf("structure of page %s is managed by %q, want %q", pageID, got, want)
			}

			return nil
		}
	}

	// The group's ID from the first apply, which later steps expect to keep or lose.
	var groupID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			// The structure stays on the page, as the last apply left it: the page's own
			// items, handed back to the dashboard.
			got, err := managedBy()
			if err != nil {
				return err
			}
			if got != client.ManagementMetaV2ManagedByDashboard {
				return fmt.Errorf("after destroy, structure of page %s is managed by %q, want dashboard", pageID, got)
			}
			items, _ := testAccStatusPageStructure(t, pageID)
			if len(items) != len(original) {
				return fmt.Errorf("after destroy, page %s has %d items, want its original %d", pageID, len(items), len(original))
			}

			return nil
		},
		Steps: []resource.TestStep{
			{
				// New components, placed with the defaults.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: pageID, Components: components, DisplayUptimeMode: "chart_only", Items: []structureTestItem{
					{Group: &structureTestGroup{Name: "Core", Components: []structureTestComponent{{Expr: componentExpr("api")}, {Expr: componentExpr("db")}}}},
					componentRef("web"),
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", pageID),
					resource.TestCheckResourceAttr(address, "display_uptime_mode", "chart_only"),
					resource.TestCheckResourceAttr(address, "items.#", "2"),
					resource.TestCheckResourceAttr(address, "items.0.group.name", "Core"),
					resource.TestCheckResourceAttrWith(address, "items.0.group.id", func(id string) error {
						if id == "" {
							return fmt.Errorf("group has no ID")
						}
						groupID = id

						return nil
					}),
					resource.TestCheckNoResourceAttr(address, "items.0.group.description"),
					resource.TestCheckResourceAttr(address, "items.0.group.hidden", "false"),
					resource.TestCheckResourceAttr(address, "items.0.group.display_aggregated_uptime", "true"),
					resource.TestCheckResourceAttr(address, "items.0.group.components.#", "2"),
					resource.TestCheckResourceAttrPair(address, "items.0.group.components.0.component_id", "incident_status_page_component.api", "id"),
					resource.TestCheckResourceAttr(address, "items.0.group.components.0.hidden", "false"),
					resource.TestCheckResourceAttr(address, "items.0.group.components.0.display_uptime", "true"),
					resource.TestCheckNoResourceAttr(address, "items.0.hidden"),
					resource.TestCheckResourceAttrPair(address, "items.1.component_id", "incident_status_page_component.web", "id"),
					resource.TestCheckResourceAttr(address, "items.1.hidden", "false"),
					resource.TestCheckResourceAttr(address, "items.1.display_uptime", "true"),
					checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
				),
			},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateId:           pageID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"unlock_in_dashboard"},
			},
			{
				// Reordered, with the display settings taken over. The group keeps its ID.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: pageID, Components: components, DisplayUptimeMode: "nothing", Items: []structureTestItem{
					{Component: &structureTestComponent{Expr: componentExpr("web"), Hidden: "true"}},
					{Group: &structureTestGroup{
						Name:                    "Core",
						Description:             "The services behind everything",
						DisplayAggregatedUptime: "false",
						Components: []structureTestComponent{
							{Expr: componentExpr("db")},
							{Expr: componentExpr("api"), DisplayUptime: "false"},
						},
					}},
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "display_uptime_mode", "nothing"),
					resource.TestCheckResourceAttr(address, "items.0.hidden", "true"),
					resource.TestCheckResourceAttrWith(address, "items.1.group.id", func(id string) error {
						if id != groupID {
							return fmt.Errorf("group ID changed from %s to %s on a reorder", groupID, id)
						}

						return nil
					}),
					resource.TestCheckResourceAttr(address, "items.1.group.description", "The services behind everything"),
					resource.TestCheckResourceAttr(address, "items.1.group.display_aggregated_uptime", "false"),
					resource.TestCheckResourceAttrPair(address, "items.1.group.components.0.component_id", "incident_status_page_component.db", "id"),
					resource.TestCheckResourceAttr(address, "items.1.group.components.1.display_uptime", "false"),
					resource.TestCheckResourceAttr(address, "items.1.group.components.1.hidden", "false"),
				),
			},
			{
				// Handed back to the dashboard. Settings left out this time are kept.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: pageID, Components: components, UnlockInDashboard: true, Items: []structureTestItem{
					componentRef("web"),
					{Group: &structureTestGroup{Name: "Core", Description: "The services behind everything", Components: []structureTestComponent{{Expr: componentExpr("db")}, {Expr: componentExpr("api")}}}},
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "display_uptime_mode", "nothing"),
					resource.TestCheckResourceAttr(address, "items.0.hidden", "true"),
					resource.TestCheckResourceAttr(address, "items.1.group.components.1.display_uptime", "false"),
					checkManagedBy(client.ManagementMetaV2ManagedByDashboard),
				),
			},
			{
				// Renamed and claimed again: a new group, so a new ID.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: pageID, Components: components, Items: []structureTestItem{
					componentRef("web"),
					{Group: &structureTestGroup{Name: "Backend", Components: []structureTestComponent{{Expr: componentExpr("db")}, {Expr: componentExpr("api")}}}},
				}}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "items.1.group.name", "Backend"),
					resource.TestCheckNoResourceAttr(address, "items.1.group.description"),
					resource.TestCheckResourceAttrWith(address, "items.1.group.id", func(id string) error {
						if id == groupID {
							return fmt.Errorf("renamed group kept the old ID %s", id)
						}

						return nil
					}),
					checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
				),
			},
			{
				// The page's own items back, so the destroy that follows can archive the
				// test's components, and the page handed back to the dashboard.
				Config: testIncidentStatusPageStructureConfig(structureTestConfig{PageID: pageID, Components: components, UnlockInDashboard: true, DisplayUptimeMode: originalMode, Items: original}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "items.#", fmt.Sprint(len(original))),
					resource.TestCheckResourceAttr(address, "display_uptime_mode", originalMode),
					checkManagedBy(client.ManagementMetaV2ManagedByDashboard),
				),
			},
		},
	})
}

// testAccStatusPageStructure reads a page's structure as test items, with only each
// placement's component ID so the settings are left as they are, and the page's uptime
// mode.
func testAccStatusPageStructure(t *testing.T, pageID string) ([]structureTestItem, string) {
	t.Helper()

	result, err := testClient.StatusPagesV2ShowStatusPageStructureWithResponse(context.Background(), pageID)
	if err != nil {
		t.Fatalf("reading the structure of status page %s: %s", pageID, err)
	}
	if result.JSON200 == nil {
		t.Fatalf("reading the structure of status page %s: unexpected response (status %s)", pageID, result.Status())
	}

	var items []structureTestItem
	for _, item := range result.JSON200.CurrentStructure.Items {
		switch {
		case item.Component != nil:
			items = append(items, componentItem(item.Component.ComponentId))
		case item.Group != nil:
			items = append(items, groupItem(item.Group.Name, lo.Map(item.Group.Components, func(component client.StatusPageStructureComponentV2, _ int) string {
				return component.ComponentId
			})...))
		default:
			t.Fatalf("status page %s has a sub-page, so it is not standalone and the test cannot manage it", pageID)
		}
	}

	return items, string(result.JSON200.DisplayUptimeMode)
}
