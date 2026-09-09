package provider

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// fakeManagedResourcesAPI counts claims, which is how these tests tell an import that
// claimed the resource apart from one that left the account untouched.
type fakeManagedResourcesAPI struct {
	requests int
	received []byte
}

func (f *fakeManagedResourcesAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/managed_resources", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		f.received, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"managed_resource":{"id":"01MANAGED","resource_type":"workflow","resource_id":"01WORKFLOW","annotations":{}}}`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

// TestImportStateMarkImportedAsManaged covers the plumbing end to end: an import with
// the provider option off must not write to the account, because Terraform runs imports
// during plan.
func TestImportStateMarkImportedAsManaged(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		markImportedAsManaged bool
		wantRequests          int
	}{
		{name: "claims the resource", markImportedAsManaged: true, wantRequests: 1},
		{name: "leaves the resource unclaimed", markImportedAsManaged: false, wantRequests: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			api := &fakeManagedResourcesAPI{}

			r := &IncidentWorkflowResource{}
			configureResp := &resource.ConfigureResponse{}
			r.Configure(ctx, resource.ConfigureRequest{
				ProviderData: &IncidentProviderData{
					Client:                api.start(t),
					TerraformVersion:      "1.14.0",
					MarkImportedAsManaged: tc.markImportedAsManaged,
				},
			}, configureResp)
			if configureResp.Diagnostics.HasError() {
				t.Fatalf("configuring resource: %v", configureResp.Diagnostics)
			}

			schemaResp := &resource.SchemaResponse{}
			r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

			importResp := &resource.ImportStateResponse{
				State: tfsdk.State{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
				},
			}
			r.ImportState(ctx, resource.ImportStateRequest{ID: "01WORKFLOW"}, importResp)
			if importResp.Diagnostics.HasError() {
				t.Fatalf("importing: %v", importResp.Diagnostics)
			}

			if api.requests != tc.wantRequests {
				t.Errorf("managed resource requests = %d, want %d", api.requests, tc.wantRequests)
			}
		})
	}
}

// TestProviderMarkImportedResourcesAsManaged pins the default: an unset attribute keeps
// the behaviour every version before this option had.
func TestProviderMarkImportedResourcesAsManaged(t *testing.T) {
	ctx := t.Context()

	p := New("test")()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	objType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("expected the provider schema to be an object, got %T", schemaResp.Schema.Type().TerraformType(ctx))
	}

	for _, tc := range []struct {
		name  string
		value tftypes.Value
		want  bool
	}{
		{name: "unset", value: tftypes.NewValue(tftypes.Bool, nil), want: true},
		{name: "true", value: tftypes.NewValue(tftypes.Bool, true), want: true},
		{name: "false", value: tftypes.NewValue(tftypes.Bool, false), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureResp := &provider.ConfigureResponse{}
			p.Configure(ctx, provider.ConfigureRequest{
				Config: tfsdk.Config{
					Schema: schemaResp.Schema,
					Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
						"endpoint":                           tftypes.NewValue(tftypes.String, "https://api.example.com"),
						"api_key":                            tftypes.NewValue(tftypes.String, "test-key"),
						"mark_imported_resources_as_managed": tc.value,
					}),
				},
			}, configureResp)
			if configureResp.Diagnostics.HasError() {
				t.Fatalf("configuring provider: %v", configureResp.Diagnostics)
			}

			data, ok := configureResp.ResourceData.(*IncidentProviderData)
			if !ok {
				t.Fatalf("expected *IncidentProviderData, got %T", configureResp.ResourceData)
			}

			if data.MarkImportedAsManaged != tc.want {
				t.Errorf("MarkImportedAsManaged = %v, want %v", data.MarkImportedAsManaged, tc.want)
			}
		})
	}
}

// TestImportStateClaimsCorrectResourceType pins the resource type each resource claims
// itself as. The plumbing is shared, so the realistic bug is a resource passing the wrong
// constant, which no other test would catch.
//
// The fake serves only the claim endpoint, so each import claims and then fails to read
// the resource back. The claim happens before the read, so it is recorded either way.
func TestImportStateClaimsCorrectResourceType(t *testing.T) {
	for _, tc := range []struct {
		name             string
		newResource      func() resource.Resource
		importID         string
		wantResourceType string
		wantErrSummary   string
	}{
		{
			name:             "api key",
			newResource:      NewIncidentAPIKeyResource,
			importID:         "01APIKEY",
			wantResourceType: `"resource_type":"api_key"`,
			wantErrSummary:   "API Key Not Found",
		},
		{
			name:             "secret",
			newResource:      NewIncidentSecretResource,
			importID:         "01SECRET",
			wantResourceType: `"resource_type":"secret"`,
			wantErrSummary:   "Secret Not Found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			api := &fakeManagedResourcesAPI{}

			r := tc.newResource()
			configurable, ok := r.(resource.ResourceWithConfigure)
			if !ok {
				t.Fatalf("%T does not implement ResourceWithConfigure", r)
			}
			configureResp := &resource.ConfigureResponse{}
			configurable.Configure(ctx, resource.ConfigureRequest{
				ProviderData: &IncidentProviderData{
					Client:                api.start(t),
					TerraformVersion:      "1.14.0",
					MarkImportedAsManaged: true,
				},
			}, configureResp)
			if configureResp.Diagnostics.HasError() {
				t.Fatalf("configuring resource: %v", configureResp.Diagnostics)
			}

			importable, ok := r.(resource.ResourceWithImportState)
			if !ok {
				t.Fatalf("%T does not implement ResourceWithImportState", r)
			}

			schemaResp := &resource.SchemaResponse{}
			r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

			importResp := &resource.ImportStateResponse{
				State: tfsdk.State{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
				},
			}
			importable.ImportState(ctx, resource.ImportStateRequest{ID: tc.importID}, importResp)

			if api.requests != 1 {
				t.Fatalf("managed resource requests = %d, want 1", api.requests)
			}
			if got := string(api.received); !strings.Contains(got, tc.wantResourceType) {
				t.Errorf("claim payload = %s, want it to contain %s", got, tc.wantResourceType)
			}
			errs := importResp.Diagnostics.Errors()
			if len(errs) == 0 {
				t.Fatalf("expected the import to fail against a fake that serves no %s", tc.name)
			}
			if summary := errs[0].Summary(); !strings.Contains(summary, tc.wantErrSummary) {
				t.Errorf("diagnostic summary = %q, want it to mention %q", summary, tc.wantErrSummary)
			}
		})
	}
}
