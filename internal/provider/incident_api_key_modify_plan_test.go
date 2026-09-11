package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakeAPIKeyValidateAPI stands in for the validate endpoint during a plan. Counting
// requests is what proves the plan stays quiet when there is nothing worth checking.
type fakeAPIKeyValidateAPI struct {
	// status is the response code, 204 when unset - which is what the endpoint answers on
	// success, with no body at all. body is served with it, so a case can assert the API's
	// own words reach the user.
	status int
	body   string

	requests int
	// received is the last request body, so a case can assert what we send.
	received []byte
}

func (f *fakeAPIKeyValidateAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api_keys/actions/validate", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		f.received, _ = io.ReadAll(r.Body)

		status := f.status
		if status == 0 {
			status = http.StatusNoContent
		}

		if f.body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		if f.body != "" {
			_, _ = w.Write([]byte(f.body))
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

// apiKeyPlan builds a plan the way the validate tests build a config: every attribute
// null, then the overrides the case is about.
//
// The role and team sets are given their schema defaults unless a case overrides them,
// because that is what the framework will have done by the time ModifyPlan runs, and a
// null there would look like a value still waiting on another resource.
func apiKeyPlan(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Plan {
	t.Helper()

	withDefaults := map[string]tftypes.Value{
		"name":            tftypes.NewValue(tftypes.String, "CI deploy key"),
		"role_names":      stringSetValue(),
		"team_ids":        stringSetValue(),
		"team_role_names": stringSetValue(),
	}
	for name, value := range overrides {
		withDefaults[name] = value
	}

	_, raw := apiKeyObject(t, withDefaults)

	return tfsdk.Plan{Schema: apiKeySchema(t), Raw: raw}
}

// modifyAPIKeyPlan runs ModifyPlan from no prior state, which is a create.
func modifyAPIKeyPlan(t *testing.T, api *fakeAPIKeyValidateAPI, plan tfsdk.Plan) resource.ModifyPlanResponse {
	t.Helper()

	keySchema := apiKeySchema(t)
	objType, _ := apiKeyObject(t, nil)

	return modifyAPIKeyPlanFrom(t, api, plan, tfsdk.State{
		Schema: keySchema,
		Raw:    tftypes.NewValue(objType, nil),
	})
}

func modifyAPIKeyPlanFrom(
	t *testing.T, api *fakeAPIKeyValidateAPI, plan tfsdk.Plan, state tfsdk.State,
) resource.ModifyPlanResponse {
	t.Helper()

	r := &IncidentAPIKeyResource{resourceConfigurer: withClient(api.start(t))}

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: state}, &resp)

	return resp
}

// Asserts the body too: a payload that doesn't describe the planned key would validate
// something nobody is about to apply.
func TestAPIKeyModifyPlanAcceptsValidKey(t *testing.T) {
	api := &fakeAPIKeyValidateAPI{}
	plan := apiKeyPlan(t, map[string]tftypes.Value{
		"name":            tftypes.NewValue(tftypes.String, "CI deploy key"),
		"comments":        tftypes.NewValue(tftypes.String, "Requested in #ask-infra"),
		"role_names":      stringSetValue("viewer"),
		"team_ids":        stringSetValue("01TEAM"),
		"team_role_names": stringSetValue("schedules_editor"),
	})

	resp := modifyAPIKeyPlan(t, api, plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Fatalf("expected 1 request, got %d", api.requests)
	}

	var sent struct {
		Name          string   `json:"name"`
		Comments      *string  `json:"comments"`
		RoleNames     []string `json:"role_names"`
		TeamIds       []string `json:"team_ids"`
		TeamRoleNames []string `json:"team_role_names"`
	}
	if err := json.Unmarshal(api.received, &sent); err != nil {
		t.Fatalf("decoding what we sent: %v (%s)", err, api.received)
	}

	if sent.Name != "CI deploy key" {
		t.Errorf("expected the planned name, got %q", sent.Name)
	}
	if sent.Comments == nil || *sent.Comments != "Requested in #ask-infra" {
		t.Errorf("expected the planned comments, got %v", sent.Comments)
	}
	if len(sent.RoleNames) != 1 || sent.RoleNames[0] != "viewer" {
		t.Errorf("expected the planned roles, got %v", sent.RoleNames)
	}
	if len(sent.TeamIds) != 1 || sent.TeamIds[0] != "01TEAM" {
		t.Errorf("expected the planned teams, got %v", sent.TeamIds)
	}
	if len(sent.TeamRoleNames) != 1 || sent.TeamRoleNames[0] != "schedules_editor" {
		t.Errorf("expected the planned team roles, got %v", sent.TeamRoleNames)
	}
}

// TestAPIKeyModifyPlanRejectsScopeEscalation is the case the endpoint exists for: a role
// the calling key cannot grant. The provider has no way to know that itself, so it has to
// carry the API's own words through to the plan.
func TestAPIKeyModifyPlanRejectsScopeEscalation(t *testing.T) {
	const detail = "You cannot grant the api_keys_manage role"

	api := &fakeAPIKeyValidateAPI{
		status: http.StatusUnprocessableEntity,
		body:   `{"type":"validation_error","status":422,"request_id":"req_1","errors":[{"code":"invalid_value","message":"` + detail + `"}]}`,
	}
	plan := apiKeyPlan(t, map[string]tftypes.Value{
		"role_names": stringSetValue("api_keys_manage"),
	})

	resp := modifyAPIKeyPlan(t, api, plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Fatalf("expected 1 request, got %d", api.requests)
	}

	// The API named the problem, so the plan should repeat it rather than paraphrase.
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), detail) {
		t.Errorf("expected the API's message in the diagnostic, got %+v", resp.Diagnostics.Errors()[0])
	}
}

// TestAPIKeyModifyPlanWarnsWhenUnreachable covers the check not running. That says nothing
// about the config, so failing the plan would break one that would have applied fine.
//
// The timeout is shortened because the client retries a server error with a backoff, and
// otherwise these cases would each wait out the real one.
func TestAPIKeyModifyPlanWarnsWhenUnreachable(t *testing.T) {
	previous := apiKeyValidateTimeout
	apiKeyValidateTimeout = 50 * time.Millisecond
	t.Cleanup(func() { apiKeyValidateTimeout = previous })

	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		api := &fakeAPIKeyValidateAPI{status: status}
		plan := apiKeyPlan(t, map[string]tftypes.Value{
			"role_names": stringSetValue("viewer"),
		})

		resp := modifyAPIKeyPlan(t, api, plan)

		if resp.Diagnostics.HasError() {
			t.Errorf("status %d: expected no error, got %+v", status, resp.Diagnostics)
		}
		if resp.Diagnostics.WarningsCount() != 1 {
			t.Errorf("status %d: expected 1 warning, got %+v", status, resp.Diagnostics)
			continue
		}
		if !strings.Contains(resp.Diagnostics.Warnings()[0].Summary(), "Could not validate") {
			t.Errorf("status %d: expected a could-not-validate warning, got %+v", status, resp.Diagnostics.Warnings()[0])
		}
	}
}

// TestAPIKeyModifyPlanSkipsWhenNothingWorthChecking covers the cases that must not reach
// the API at all. Each one either cannot be helped by an answer or would be asked about a
// key nobody is applying.
func TestAPIKeyModifyPlanSkipsWhenNothingWorthChecking(t *testing.T) {
	keySchema := apiKeySchema(t)
	objType, _ := apiKeyObject(t, nil)
	null := tftypes.NewValue(objType, nil)

	t.Run("destroy", func(t *testing.T) {
		api := &fakeAPIKeyValidateAPI{}
		resp := modifyAPIKeyPlanFrom(t, api,
			tfsdk.Plan{Schema: keySchema, Raw: null},
			tfsdk.State{Schema: keySchema, Raw: null},
		)

		if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
			t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
		}
		if api.requests != 0 {
			t.Fatalf("expected no requests, got %d", api.requests)
		}
	})

	t.Run("no change", func(t *testing.T) {
		// The framework runs ModifyPlan for every resource in the plan, changed or not. A
		// rejection here isn't something an apply could fix, because no apply is coming.
		api := &fakeAPIKeyValidateAPI{status: http.StatusUnprocessableEntity}
		plan := apiKeyPlan(t, map[string]tftypes.Value{"role_names": stringSetValue("viewer")})

		resp := modifyAPIKeyPlanFrom(t, api, plan, tfsdk.State{Schema: keySchema, Raw: plan.Raw})

		if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
			t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
		}
		if api.requests != 0 {
			t.Fatalf("expected no requests, got %d", api.requests)
		}
	})

	t.Run("unconfigured provider", func(t *testing.T) {
		// `terraform validate` runs the provider without configuring it, so there is no
		// client to ask with.
		r := &IncidentAPIKeyResource{}

		var resp resource.ModifyPlanResponse
		r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
			Plan:  apiKeyPlan(t, nil),
			State: tfsdk.State{Schema: keySchema, Raw: null},
		}, &resp)

		if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
			t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
		}
	})

	for _, tc := range []struct {
		name      string
		overrides map[string]tftypes.Value
	}{
		{
			// A team ID from a catalog entry this same apply creates. Validating around the
			// gap would report errors the apply won't hit.
			name: "unknown team ids",
			overrides: map[string]tftypes.Value{
				"team_ids": tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, tftypes.UnknownValue),
			},
		},
		{
			name: "unknown role names",
			overrides: map[string]tftypes.Value{
				"role_names": tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, tftypes.UnknownValue),
			},
		},
		{
			name: "unknown name",
			overrides: map[string]tftypes.Value{
				"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			},
		},
		{
			// An element inside the set, rather than the set itself: a team list built from
			// one known and one computed ID.
			name: "unknown team id element",
			overrides: map[string]tftypes.Value{
				"team_ids": tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, []tftypes.Value{
					tftypes.NewValue(tftypes.String, "01TEAM"),
					tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				}),
				"team_role_names": stringSetValue("schedules_editor"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPIKeyValidateAPI{status: http.StatusUnprocessableEntity}

			resp := modifyAPIKeyPlan(t, api, apiKeyPlan(t, tc.overrides))

			if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
				t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
			}
			if api.requests != 0 {
				t.Fatalf("expected no requests, got %d", api.requests)
			}
		})
	}
}

// TestAPIKeyModifyPlanChecksAnUpdate covers an edit to an existing key, not just a create:
// the scope rules apply to both, and the update payload has the same shape.
func TestAPIKeyModifyPlanChecksAnUpdate(t *testing.T) {
	api := &fakeAPIKeyValidateAPI{}
	keySchema := apiKeySchema(t)

	state := apiKeyPlan(t, map[string]tftypes.Value{
		"role_names": stringSetValue("viewer"),
	})
	plan := apiKeyPlan(t, map[string]tftypes.Value{
		"role_names": stringSetValue("viewer", "catalog_viewer"),
	})

	resp := modifyAPIKeyPlanFrom(t, api, plan, tfsdk.State{Schema: keySchema, Raw: state.Raw})

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Fatalf("expected 1 request, got %d", api.requests)
	}

	var sent struct {
		RoleNames []string `json:"role_names"`
	}
	if err := json.Unmarshal(api.received, &sent); err != nil {
		t.Fatalf("decoding what we sent: %v (%s)", err, api.received)
	}
	if len(sent.RoleNames) != 2 {
		t.Errorf("expected the planned roles rather than the stored ones, got %v", sent.RoleNames)
	}
}
