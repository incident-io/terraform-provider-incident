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

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// fakeAlertSourceValidateAPI stands in for the validate endpoint during a plan. Counting
// requests is what proves the plan stays quiet when there is nothing worth checking.
type fakeAlertSourceValidateAPI struct {
	// status is the response code, 200 when unset. body is served with it, so a case can
	// assert the API's own words reach the user.
	status int
	body   string

	requests int
	// received is the last request body, so a case can assert what we send.
	received []byte
}

func (f *fakeAlertSourceValidateAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v3/alert_sources/actions/validate", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		f.received, _ = io.ReadAll(r.Body)

		status, body := f.status, f.body
		if status == 0 {
			status = http.StatusOK
			if body == "" {
				body = `{"warnings":[]}`
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
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

// alertSourceBetaPlan builds a plan the way the validate tests build a config: every
// attribute null, then the overrides the case is about.
func alertSourceBetaPlan(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Plan {
	t.Helper()

	config := alertSourceBetaConfig(t, overrides)

	return tfsdk.Plan(config)
}

// modifyAlertSourceBetaPlan runs ModifyPlan from no prior state, which is a create. A nil
// plan is a destroy.
func modifyAlertSourceBetaPlan(t *testing.T, api *fakeAlertSourceValidateAPI, plan *tfsdk.Plan) resource.ModifyPlanResponse {
	t.Helper()

	config, objType := alertSourceBetaSchemaType(t)
	null := tftypes.NewValue(objType, nil)

	if plan == nil {
		plan = &tfsdk.Plan{Schema: config.Schema, Raw: null}
	}

	return modifyAlertSourceBetaPlanFrom(t, api, *plan, tfsdk.State{Schema: config.Schema, Raw: null})
}

func modifyAlertSourceBetaPlanFrom(
	t *testing.T, api *fakeAlertSourceValidateAPI, plan tfsdk.Plan, state tfsdk.State,
) resource.ModifyPlanResponse {
	t.Helper()

	r := &alertSourceBetaResource{client: api.start(t)}

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: state}, &resp)

	return resp
}

// Asserts the body too: a payload that doesn't describe the planned source would validate
// something nobody is about to apply.
func TestAlertSourceBetaModifyPlanAcceptsValidSource(t *testing.T) {
	api := &fakeAlertSourceValidateAPI{}
	plan := alertSourceBetaPlan(t, map[string]tftypes.Value{
		"source_type": stringValue("http"),
		"is_private":  tftypes.NewValue(tftypes.Bool, true),
		"owning_team_ids": tftypes.NewValue(attributeType(t, "owning_team_ids"), []tftypes.Value{
			stringValue("01TEAM"),
		}),
	})

	resp := modifyAlertSourceBetaPlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Fatalf("expected 1 request, got %d", api.requests)
	}

	var sent struct {
		AlertSource struct {
			SourceType    string   `json:"source_type"`
			IsPrivate     *bool    `json:"is_private"`
			OwningTeamIds []string `json:"owning_team_ids"`
		} `json:"alert_source"`
	}
	if err := json.Unmarshal(api.received, &sent); err != nil {
		t.Fatalf("decoding what we sent: %v (%s)", err, api.received)
	}

	if sent.AlertSource.SourceType != "http" {
		t.Errorf("expected the planned source type, got %q", sent.AlertSource.SourceType)
	}
	if sent.AlertSource.IsPrivate == nil || !*sent.AlertSource.IsPrivate {
		t.Errorf("expected is_private to be sent as true, got %v", sent.AlertSource.IsPrivate)
	}
	if len(sent.AlertSource.OwningTeamIds) != 1 || sent.AlertSource.OwningTeamIds[0] != "01TEAM" {
		t.Errorf("expected the owning teams to be sent, got %v", sent.AlertSource.OwningTeamIds)
	}
}

// A 422 fails the plan and repeats what the API said, field path included.
func TestAlertSourceBetaModifyPlanRejectsInvalidSource(t *testing.T) {
	api := &fakeAlertSourceValidateAPI{
		status: http.StatusUnprocessableEntity,
		body:   `{"type":"validation_error","errors":[{"code":"invalid_value","message":"referenced resource not found in scope","source":{"field":"alert_source.expressions"}}]}`,
	}
	plan := alertSourceBetaPlan(t, map[string]tftypes.Value{
		"source_type": stringValue("http"),
	})

	resp := modifyAlertSourceBetaPlan(t, api, &plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error, got %+v", resp.Diagnostics)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "alert_source.expressions") {
		t.Errorf("expected the field path in the diagnostic, got %q", detail)
	}
}

// Any other status means the check didn't run, over a config that may be perfectly good.
func TestAlertSourceBetaModifyPlanWarnsWhenCheckUnavailable(t *testing.T) {
	original := alertSourceBetaValidateTimeout
	alertSourceBetaValidateTimeout = 50 * time.Millisecond
	t.Cleanup(func() { alertSourceBetaValidateTimeout = original })

	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		api := &fakeAlertSourceValidateAPI{status: status}
		plan := alertSourceBetaPlan(t, map[string]tftypes.Value{
			"source_type": stringValue("http"),
		})

		resp := modifyAlertSourceBetaPlan(t, api, &plan)

		if resp.Diagnostics.HasError() {
			t.Errorf("status %d: expected no error, got %+v", status, resp.Diagnostics)
		}
		if resp.Diagnostics.WarningsCount() != 1 {
			t.Errorf("status %d: expected 1 warning, got %+v", status, resp.Diagnostics)
		}
	}
}

// Every create plans the computed attributes unknown, and the payload carries none of them.
// Treating any unknown as unsettled would skip the check on exactly the plans worth making.
func TestAlertSourceBetaModifyPlanValidatesWhenOnlyComputedAttributesAreUnknown(t *testing.T) {
	api := &fakeAlertSourceValidateAPI{}
	plan := alertSourceBetaPlan(t, map[string]tftypes.Value{
		"source_type":      stringValue("http"),
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"secret_token":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"alert_events_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})

	resp := modifyAlertSourceBetaPlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Errorf("expected the source to be checked, got %d requests", api.requests)
	}
}

// An unsettled value would be sent as a source with no type, or one owned by nobody, and
// rejected for reasons the apply won't hit.
//
// Both of these decode into the model happily, which is the point: the gate has to stop
// them, not the decode.
func TestAlertSourceBetaModifyPlanSkipsUnknownValues(t *testing.T) {
	for name, override := range map[string]map[string]tftypes.Value{
		"source type": {
			"source_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		},
		"owning teams": {
			"source_type":     stringValue("http"),
			"owning_team_ids": tftypes.NewValue(attributeType(t, "owning_team_ids"), tftypes.UnknownValue),
		},
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeAlertSourceValidateAPI{}
			plan := alertSourceBetaPlan(t, override)

			resp := modifyAlertSourceBetaPlan(t, api, &plan)

			if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
				t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
			}
			if api.requests != 0 {
				t.Errorf("expected no request for an unsettled plan, got %d", api.requests)
			}
		})
	}
}

// The framework runs ModifyPlan for every resource in the plan, changed or not, so without
// this a workspace of thirty sources costs thirty registry builds per plan.
func TestAlertSourceBetaModifyPlanSkipsUnchangedSource(t *testing.T) {
	api := &fakeAlertSourceValidateAPI{}
	plan := alertSourceBetaPlan(t, map[string]tftypes.Value{
		"source_type": stringValue("http"),
		"id":          stringValue("01SOURCE"),
	})

	resp := modifyAlertSourceBetaPlanFrom(t, api, plan, tfsdk.State(plan))

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for an unchanged source, got %d", api.requests)
	}
}

func TestAlertSourceBetaModifyPlanSkipsDestroy(t *testing.T) {
	api := &fakeAlertSourceValidateAPI{}

	resp := modifyAlertSourceBetaPlan(t, api, nil)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for a destroy, got %d", api.requests)
	}
}

// The settled gate names its attributes as strings, so a schema rename would silently stop
// gating on one and start validating against unknowns.
func TestAlertSourceBetaValidatedAttributesExist(t *testing.T) {
	_, objType := alertSourceBetaSchemaType(t)

	for _, name := range alertSourceBetaValidatedAttributes {
		if _, ok := objType.AttributeTypes[name]; !ok {
			t.Errorf("the validate payload reads %q, which the schema no longer has", name)
		}
	}
}

// warningPaths returns each warning's attribute path, with "" for one raised against the
// resource as a whole.
func warningPaths(diags diag.Diagnostics) []string {
	paths := []string{}
	for _, warning := range diags.Warnings() {
		withPath, ok := warning.(diag.DiagnosticWithPath)
		if !ok {
			paths = append(paths, "")
			continue
		}

		paths = append(paths, withPath.Path().String())
	}

	return paths
}

// A reference nothing in scope resolves renders as "(not set)" once the source is live, so
// the plan is the last place anyone sees it.
func TestAlertSourceBetaModifyPlanSurfacesWarnings(t *testing.T) {
	api := &fakeAlertSourceValidateAPI{
		status: http.StatusOK,
		body: `{"warnings":[
			{"path":"title","summary":"payload.sumary doesn't resolve","detail":"The title references payload.sumary, which this alert source can't resolve."},
			{"path":"description","summary":"expressions[\"Team\"] doesn't resolve","detail":"The description references expressions[\"Team\"], which this alert source can't resolve."}
		]}`,
	}
	plan := alertSourceBetaPlan(t, map[string]tftypes.Value{
		"source_type": stringValue("http"),
		"title": objectWith(t, attributeType(t, "title"), map[string]tftypes.Value{
			"literal": stringValue("{{ payload.sumary }}"),
		}),
	})

	resp := modifyAlertSourceBetaPlan(t, api, &plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected warnings not errors, got %+v", resp.Diagnostics)
	}

	// The literal is where the reference is written, so that's what the plan should point at.
	paths := warningPaths(resp.Diagnostics)
	if len(paths) != 2 || paths[0] != "title.literal" || paths[1] != "description.literal" {
		t.Fatalf("expected the literals to be pointed at, got %v", paths)
	}

	detail := resp.Diagnostics.Warnings()[0].Detail()
	if !strings.Contains(detail, "payload.sumary") {
		t.Errorf("expected the API's explanation to reach the user, got %q", detail)
	}
}

// A path we don't map still has something to say, and losing it would mean a new check on
// the API going unheard until the provider caught up.
func TestAlertSourceBetaModifyPlanKeepsWarningsAboutUnknownPaths(t *testing.T) {
	api := &fakeAlertSourceValidateAPI{
		status: http.StatusOK,
		body:   `{"warnings":[{"path":"expressions[0].returns","summary":"something to know","detail":"about a field the provider doesn't map"}]}`,
	}
	plan := alertSourceBetaPlan(t, map[string]tftypes.Value{
		"source_type": stringValue("http"),
	})

	resp := modifyAlertSourceBetaPlan(t, api, &plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected a warning not an error, got %+v", resp.Diagnostics)
	}
	if paths := warningPaths(resp.Diagnostics); len(paths) != 1 || paths[0] != "" {
		t.Fatalf("expected one warning against no attribute, got %v", paths)
	}
}

// The warning paths are the API's payload fields, spelled as attribute paths. A rename on
// either side would silently stop anchoring the warning.
func TestAlertSourceBetaWarningPathsExist(t *testing.T) {
	_, objType := alertSourceBetaSchemaType(t)

	for field, attribute := range alertSourceBetaWarningPaths {
		attrType, ok := objType.AttributeTypes[field].(tftypes.Object)
		if !ok {
			t.Errorf("the API warns about %q, which the schema no longer has as an object", field)
			continue
		}
		if _, ok := attrType.AttributeTypes["literal"]; !ok {
			t.Errorf("%s has no literal to point %s at", field, attribute)
		}
	}
}
