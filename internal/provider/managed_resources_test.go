package provider

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// fakeManagedResourcesAPI counts claims, which is how these tests tell an import that
// claimed the resource apart from one that left the account untouched.
type fakeManagedResourcesAPI struct {
	requests     int
	received     []byte
	status       int
	responseBody string
}

func (f *fakeManagedResourcesAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/managed_resources", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		f.received, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		if f.status != 0 {
			w.WriteHeader(f.status)
		}
		body := f.responseBody
		if body == "" {
			body = `{"managed_resource":{"id":"01MANAGED","resource_type":"workflow","resource_id":"01WORKFLOW","annotations":{}}}`
		}
		_, _ = w.Write([]byte(body))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

func TestClaimResourceTeamScopedEscalationPathPermission(t *testing.T) {
	for _, tc := range []struct {
		name         string
		resourceType client.ManagedResourcesCreateManagedResourcePayloadV2ResourceType
		status       int
		body         string
		wantErrors   int
		wantWarnings int
	}{
		{
			name:         "warns for a team-scoped escalation path permission",
			resourceType: client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath,
			status:       http.StatusForbidden,
			body:         `{"type":"missing_required_scope","message":"Missing required scope escalation_paths.update"}`,
			wantWarnings: 1,
		},
		{
			name:         "fails for another escalation path permission error",
			resourceType: client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath,
			status:       http.StatusForbidden,
			body:         `{"type":"resource_forbidden"}`,
			wantErrors:   1,
		},
		{
			name:         "fails for another resource type",
			resourceType: client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeSchedule,
			status:       http.StatusForbidden,
			body:         `{"type":"missing_required_scope"}`,
			wantErrors:   1,
		},
		{
			name:         "fails for an unrelated server error",
			resourceType: client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath,
			status:       http.StatusInternalServerError,
			body:         `{"type":"internal_error"}`,
			wantErrors:   1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeManagedResourcesAPI{status: tc.status, responseBody: tc.body}
			var diagnostics diag.Diagnostics

			claimResource(t.Context(), api.start(t), "01ESCALATIONPATH", &diagnostics, tc.resourceType, "1.14.0")

			if got := diagnostics.ErrorsCount(); got != tc.wantErrors {
				t.Errorf("errors = %d, want %d: %v", got, tc.wantErrors, diagnostics)
			}
			if got := diagnostics.WarningsCount(); got != tc.wantWarnings {
				t.Errorf("warnings = %d, want %d: %v", got, tc.wantWarnings, diagnostics)
			}
		})
	}
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
