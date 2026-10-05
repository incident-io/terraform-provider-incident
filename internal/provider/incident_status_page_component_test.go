package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakeStatusPageComponentsAPI is an in-memory stand-in for the component endpoints and
// the managed-resources claim, so the resource and data source can be driven through
// Terraform without an account. Like the real API it:
//
//   - stores a blank description as no description, and reads it back absent
//   - clears the description when an update omits it
//   - refuses to archive a component a page's structure still places, with a 422 naming it
//   - reports who manages a component from the annotations its last claim carried
type fakeStatusPageComponentsAPI struct {
	t *testing.T

	mu         sync.Mutex
	components []client.StatusPageComponentV2
	// placed holds the IDs of components a page's structure places.
	placed map[string]bool
	// claims holds the annotations each component's last managed-resource claim sent.
	claims map[string]map[string]string
	nextID int
}

func startFakeStatusPageComponentsAPI(t *testing.T) (*fakeStatusPageComponentsAPI, string) {
	t.Helper()

	fake := &fakeStatusPageComponentsAPI{
		t:      t,
		placed: map[string]bool{},
		claims: map[string]map[string]string{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v2/status_page_components", fake.create)
	mux.HandleFunc("GET /v2/status_page_components/{id}", fake.show)
	mux.HandleFunc("PUT /v2/status_page_components/{id}", fake.update)
	mux.HandleFunc("DELETE /v2/status_page_components/{id}", fake.destroy)
	mux.HandleFunc("POST /v2/managed_resources", fake.claim)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return fake, server.URL
}

// seed adds a component directly, for a data source test that wants something to find.
func (f *fakeStatusPageComponentsAPI) seed(name string, description *string) client.StatusPageComponentV2 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.add(name, description)
}

// add stores a new component. The caller holds the lock.
func (f *fakeStatusPageComponentsAPI) add(name string, description *string) client.StatusPageComponentV2 {
	f.nextID++
	component := client.StatusPageComponentV2{
		Id:          fmt.Sprintf("01FAKE%020d", f.nextID),
		Name:        name,
		Description: description,
	}
	f.components = append(f.components, component)

	return f.withManagement(component)
}

// setPlaced marks every component as placed on a page, or not, which decides whether it
// can be archived.
func (f *fakeStatusPageComponentsAPI) setPlaced(placed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, component := range f.components {
		f.placed[component.Id] = placed
	}
}

// managedBy reports who the fake thinks manages a component, from its last claim.
func (f *fakeStatusPageComponentsAPI) managedBy(id string) client.ManagementMetaV2ManagedBy {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.management(id).ManagedBy
}

// management builds a component's management_meta. The caller holds the lock.
func (f *fakeStatusPageComponentsAPI) management(id string) client.ManagementMetaV2 {
	annotations := f.claims[id]
	if annotations == nil {
		annotations = map[string]string{}
	}

	managedBy := client.ManagementMetaV2ManagedByDashboard
	if _, ok := annotations["incident.io/terraform/version"]; ok {
		managedBy = client.ManagementMetaV2ManagedByTerraform
	}

	return client.ManagementMetaV2{Annotations: annotations, ManagedBy: managedBy}
}

func (f *fakeStatusPageComponentsAPI) withManagement(component client.StatusPageComponentV2) client.StatusPageComponentV2 {
	component.ManagementMeta = f.management(component.Id)

	return component
}

// find returns the index of a live component, or -1. The caller holds the lock.
func (f *fakeStatusPageComponentsAPI) find(id string) int {
	return lo.IndexOf(lo.Map(f.components, func(component client.StatusPageComponentV2, _ int) string {
		return component.Id
	}), id)
}

// normaliseDescription mirrors the API, which stores a blank description as none.
func normaliseDescription(description *string) *string {
	if description == nil || strings.TrimSpace(*description) == "" {
		return nil
	}

	return description
}

func (f *fakeStatusPageComponentsAPI) create(w http.ResponseWriter, r *http.Request) {
	var payload client.StatusPageComponentsCreatePayloadV2
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.t.Fatalf("decoding create payload: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	component := f.add(payload.Name, normaliseDescription(payload.Description))

	writeJSONStatus(f.t, w, http.StatusCreated, client.StatusPageComponentsCreateResultV2{StatusPageComponent: component})
}

func (f *fakeStatusPageComponentsAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	i := f.find(r.PathValue("id"))
	if i < 0 {
		writeNotFound(w, "status page component")
		return
	}

	writeJSON(f.t, w, client.StatusPageComponentsShowResultV2{StatusPageComponent: f.withManagement(f.components[i])})
}

func (f *fakeStatusPageComponentsAPI) update(w http.ResponseWriter, r *http.Request) {
	var payload client.StatusPageComponentsUpdatePayloadV2
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.t.Fatalf("decoding update payload: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	i := f.find(r.PathValue("id"))
	if i < 0 {
		writeNotFound(w, "status page component")
		return
	}

	f.components[i].Name = payload.Name
	f.components[i].Description = normaliseDescription(payload.Description)

	writeJSON(f.t, w, client.StatusPageComponentsUpdateResultV2{StatusPageComponent: f.withManagement(f.components[i])})
}

func (f *fakeStatusPageComponentsAPI) destroy(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := r.PathValue("id")
	i := f.find(id)
	if i < 0 {
		writeNotFound(w, "status page component")
		return
	}

	if f.placed[id] {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"type":"validation_error","status":422,"errors":[{"code":"in_use","message":"Component is still placed on a status page","source":{"pointer":"id"}}]}`)
		return
	}

	f.components = append(f.components[:i], f.components[i+1:]...)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeStatusPageComponentsAPI) claim(w http.ResponseWriter, r *http.Request) {
	var payload client.ManagedResourcesCreateManagedResourcePayloadV2
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.t.Fatalf("decoding claim payload: %v", err)
	}
	if payload.ResourceType != client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageComponent {
		f.t.Fatalf("claim for unexpected resource type %q", payload.ResourceType)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.find(payload.ResourceId) < 0 {
		writeNotFound(w, "status page component")
		return
	}

	f.claims[payload.ResourceId] = payload.Annotations
	management := f.management(payload.ResourceId)

	writeJSONStatus(f.t, w, http.StatusCreated, client.ManagedResourcesCreateManagedResourceResultV2{
		ManagedResource: client.ManagedResourceV2{
			Annotations:  management.Annotations,
			ManagedBy:    client.ManagedResourceV2ManagedBy(management.ManagedBy),
			ResourceId:   payload.ResourceId,
			ResourceType: client.ManagedResourceV2ResourceTypeStatusPageComponent,
		},
	})
}

type statusPageComponentTestConfig struct {
	Name              string
	Description       string
	UnlockInDashboard bool
	WithLookups       bool
}

func testAccIncidentStatusPageComponentResourceConfig(config statusPageComponentTestConfig) string {
	return testRunTemplate("incident_status_page_component", `
resource "incident_status_page_component" "test" {
  name = {{ .Name | quote }}
  {{ if .Description }}description = {{ .Description | quote }}{{ end }}
  {{ if .UnlockInDashboard }}unlock_in_dashboard = true{{ end }}
}
{{ if .WithLookups }}
data "incident_status_page_component" "by_id" {
  id = incident_status_page_component.test.id
}
{{ end }}
`, config)
}

// testStatusPageComponentLifecycleSteps is the life of a component: created with a
// description and looked up by id, imported, renamed with the description dropped,
// then handed back to the dashboard. managedBy reports who the API thinks manages the
// component; nil skips the claim checks, for a real account where no read endpoint
// reports it.
func testStatusPageComponentLifecycleSteps(name string, managedBy func(id string) client.ManagementMetaV2ManagedBy) []resource.TestStep {
	const address = "incident_status_page_component.test"

	checkManagedBy := func(want client.ManagementMetaV2ManagedBy) resource.TestCheckFunc {
		if managedBy == nil {
			return func(*terraform.State) error { return nil }
		}

		return resource.TestCheckResourceAttrWith(address, "id", func(id string) error {
			if got := managedBy(id); got != want {
				return fmt.Errorf("component %s is managed by %q, want %q", id, got, want)
			}

			return nil
		})
	}

	return []resource.TestStep{
		{
			Config: testAccIncidentStatusPageComponentResourceConfig(statusPageComponentTestConfig{
				Name:        name,
				Description: "The public REST API",
				WithLookups: true,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(address, "id"),
				resource.TestCheckResourceAttr(address, "name", name),
				resource.TestCheckResourceAttr(address, "description", "The public REST API"),
				resource.TestCheckNoResourceAttr(address, "unlock_in_dashboard"),
				checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
				resource.TestCheckResourceAttrPair("data.incident_status_page_component.by_id", "id", address, "id"),
				resource.TestCheckResourceAttr("data.incident_status_page_component.by_id", "description", "The public REST API"),
			),
		},
		{
			ResourceName:      address,
			ImportState:       true,
			ImportStateVerify: true,
			// Whether Terraform claims a component is configuration, which an import has
			// none of.
			ImportStateVerifyIgnore: []string{"unlock_in_dashboard"},
		},
		{
			// Renamed, and the description dropped.
			Config: testAccIncidentStatusPageComponentResourceConfig(statusPageComponentTestConfig{
				Name: name + " (renamed)",
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(address, "name", name+" (renamed)"),
				resource.TestCheckNoResourceAttr(address, "description"),
				checkManagedBy(client.ManagementMetaV2ManagedByTerraform),
			),
		},
		{
			// Handed back to the dashboard: the update unclaims it.
			Config: testAccIncidentStatusPageComponentResourceConfig(statusPageComponentTestConfig{
				Name:              name + " (renamed)",
				UnlockInDashboard: true,
			}),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(address, "unlock_in_dashboard", "true"),
				checkManagedBy(client.ManagementMetaV2ManagedByDashboard),
			),
		},
	}
}

// TestIncidentStatusPageComponentResourceLifecycle runs the lifecycle against a fake API,
// so it needs no account.
func TestIncidentStatusPageComponentResourceLifecycle(t *testing.T) {
	fake, url := startFakeStatusPageComponentsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testStatusPageComponentLifecycleSteps("API", fake.managedBy),
	})
}

// TestIncidentStatusPageComponentResourceDeleteWhilePlaced checks the API's refusal to
// archive a placed component reaches the practitioner with its reason.
func TestIncidentStatusPageComponentResourceDeleteWhilePlaced(t *testing.T) {
	fake, url := startFakeStatusPageComponentsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	config := testAccIncidentStatusPageComponentResourceConfig(statusPageComponentTestConfig{Name: "Placed"})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				PreConfig:   func() { fake.setPlaced(true) },
				Config:      config,
				Destroy:     true,
				ExpectError: regexp.MustCompile(`Component is still placed on a status page`),
			},
			{
				// Unplaced again, so the test's own clean-up can archive it.
				PreConfig: func() { fake.setPlaced(false) },
				Config:    config,
			},
		},
	})
}

// TestIncidentStatusPageComponentDataSource reads a component created outside Terraform,
// and checks a missing one fails rather than reading as empty.
func TestIncidentStatusPageComponentDataSource(t *testing.T) {
	fake, url := startFakeStatusPageComponentsAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	dashboard := fake.seed("Dashboard", nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_status_page_component" "missing" {
  id = "01FAKE"
}
`,
				ExpectError: regexp.MustCompile(`could not find status page component`),
			},
			{
				Config: `
data "incident_status_page_component" "dashboard" {
  id = "` + dashboard.Id + `"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_status_page_component.dashboard", "name", "Dashboard"),
					// A component without a description reads back null rather than empty.
					resource.TestCheckNoResourceAttr("data.incident_status_page_component.dashboard", "description"),
				),
			},
		},
	})
}

// TestAccIncidentStatusPageComponentResource runs the lifecycle against a real account.
//
// The API key needs the "Configure status pages" permission for the writes. The managed
// resource record isn't returned by any read endpoint, so the claim isn't checked here.
func TestAccIncidentStatusPageComponentResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testStatusPageComponentLifecycleSteps(StableSuffix("TF test component"), nil),
	})
}
