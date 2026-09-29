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

// fakeStatusPagesAPI is an in-memory stand-in for the two read-only status page endpoints
// the data source uses. Status pages can't be created through the API, so this is the
// only way to drive the data source without a hand-configured account: it seeds pages
// directly, serves them newest first, and pages the list with a cursor as the real
// endpoint does.
type fakeStatusPagesAPI struct {
	t *testing.T

	mu        sync.Mutex
	pages     []client.StatusPageV2
	listCount int
	nextID    int

	// pageSize caps the page the list endpoint serves, whatever page_size asks for, so a
	// test can make the data source walk the cursor with a handful of pages.
	pageSize int
}

func startFakeStatusPagesAPI(t *testing.T) (*fakeStatusPagesAPI, string) {
	t.Helper()

	fake := &fakeStatusPagesAPI{t: t, pageSize: 250}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/status_pages", fake.list)
	mux.HandleFunc("GET /v2/status_pages/{id}", fake.show)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return fake, server.URL
}

// seed adds a page directly. A nil description or public URL is left out of the response,
// as the real endpoint omits them for a page that has none.
func (f *fakeStatusPagesAPI) seed(name string, description, publicURL *string) client.StatusPageV2 {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	page := client.StatusPageV2{
		// IDs that sort in creation order, as ULIDs do.
		Id:          fmt.Sprintf("01FAKE%020d", f.nextID),
		Name:        name,
		Description: description,
		PublicUrl:   publicURL,
	}
	f.pages = append(f.pages, page)

	return page
}

// listCalls counts the list requests so far, and forgets them.
func (f *fakeStatusPagesAPI) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	count := f.listCount
	f.listCount = 0

	return count
}

func (f *fakeStatusPagesAPI) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.listCount++

	pageSize := f.pageSize
	if requested, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && requested < pageSize {
		pageSize = requested
	}

	// Newest first, as the real endpoint orders by ID descending.
	ordered := append([]client.StatusPageV2{}, f.pages...)
	slices.Reverse(ordered)
	if after := r.URL.Query().Get("after"); after != "" {
		ordered = lo.Filter(ordered, func(page client.StatusPageV2, _ int) bool {
			return page.Id < after
		})
	}

	page := ordered[:min(pageSize, len(ordered))]
	meta := client.PaginationMetaResultV2{PageSize: int64(pageSize)}
	// The real endpoint offers a cursor whenever the page filled, whether or not another
	// page exists.
	if len(page) == pageSize && len(page) > 0 {
		meta.After = lo.ToPtr(page[len(page)-1].Id)
	}

	writeJSON(f.t, w, client.StatusPagesListStatusPagesResultV2{StatusPages: page, PaginationMeta: meta})
}

func (f *fakeStatusPagesAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := r.PathValue("id")
	page, found := lo.Find(f.pages, func(page client.StatusPageV2) bool { return page.Id == id })
	if !found {
		writeNotFound(w, "status page")
		return
	}

	writeJSON(f.t, w, client.StatusPagesShowStatusPageResultV2{StatusPage: page})
}

// TestIncidentStatusPageDataSourceByID reads a page through the show endpoint, and checks
// the optional attributes come back null rather than empty when the API leaves them out.
func TestIncidentStatusPageDataSourceByID(t *testing.T) {
	fake, url := startFakeStatusPagesAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	full := fake.seed("Our public status page",
		lo.ToPtr("This status page is our public status page."),
		lo.ToPtr("https://statuspage.incident.io/our-public-status-page"))
	bare := fake.seed("Internal status page", nil, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "incident_status_page" "full" {
  id = %q
}

data "incident_status_page" "bare" {
  id = %q
}
`, full.Id, bare.Id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_status_page.full", "id", full.Id),
					resource.TestCheckResourceAttr("data.incident_status_page.full", "name", "Our public status page"),
					resource.TestCheckResourceAttr("data.incident_status_page.full", "description", "This status page is our public status page."),
					resource.TestCheckResourceAttr("data.incident_status_page.full", "public_url", "https://statuspage.incident.io/our-public-status-page"),
					resource.TestCheckResourceAttr("data.incident_status_page.bare", "id", bare.Id),
					resource.TestCheckResourceAttr("data.incident_status_page.bare", "name", "Internal status page"),
					resource.TestCheckNoResourceAttr("data.incident_status_page.bare", "description"),
					resource.TestCheckNoResourceAttr("data.incident_status_page.bare", "public_url"),
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
data "incident_status_page" "missing" {
  id = "01FAKE00000000000000000000"
}
`,
				ExpectError: regexp.MustCompile(`Unable to read status page`),
			},
		},
	})
}

// TestIncidentStatusPageDataSourcePagesToFindAName is the case a lookup by ID never hits:
// the list is paginated and carries no name filter, so a name on a later page is only
// found by walking the cursor, and a name on no page has to end the walk. Two pages
// sharing a name are reported rather than picked between.
func TestIncidentStatusPageDataSourcePagesToFindAName(t *testing.T) {
	fake, url := startFakeStatusPagesAPI(t)
	t.Setenv("INCIDENT_ENDPOINT", url)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	// One per page. The list is newest first, so the oldest page is on the last one.
	oldest := fake.seed("Our public status page", nil, lo.ToPtr("https://statuspage.incident.io/public"))
	fake.seed("Customer status page", nil, nil)
	fake.seed("Customer status page", nil, nil)
	fake.pageSize = 1

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_status_page" "missing" {
  name = "Nobody's status page"
}
`,
				ExpectError: wrapRe(`no status page found with name "Nobody's status page"`),
			},
			{
				Config: `
data "incident_status_page" "duplicated" {
  name = "Customer status page"
}
`,
				ExpectError: wrapRe(`found 2 status pages named "Customer status page"; look it up by id instead`),
			},
			{
				Config: `
data "incident_status_page" "both" {
  id   = "01FAKE"
  name = "Our public status page"
}
`,
				ExpectError: regexp.MustCompile(`Ambiguous lookup`),
			},
			{
				Config: `
data "incident_status_page" "neither" {
}
`,
				ExpectError: regexp.MustCompile(`Missing lookup`),
			},
			{
				// Last, so the config the harness tears down with is one that works.
				Config: `
data "incident_status_page" "oldest" {
  name = "Our public status page"
}
`,
				PreConfig: func() { fake.listCalls() },
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incident_status_page.oldest", "id", oldest.Id),
					resource.TestCheckResourceAttr("data.incident_status_page.oldest", "public_url", "https://statuspage.incident.io/public"),
					resource.TestCheckNoResourceAttr("data.incident_status_page.oldest", "description"),
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
