package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// fakeHeartbeatPauseAPI creates alert sources happily and fails every update, which is the
// shape of a create whose follow-up pause doesn't land.
type fakeHeartbeatPauseAPI struct {
	updates int
}

// start serves both API versions, so each resource's test can point at the paths it uses.
func (f *fakeHeartbeatPauseAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	writeSource := func(w http.ResponseWriter, status int, source any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"alert_source": source})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v3/alert_sources", func(w http.ResponseWriter, r *http.Request) {
		writeSource(w, http.StatusCreated, client.AlertSourceV3{
			Id:         "01SOURCE",
			Name:       "Cron",
			SourceType: client.AlertSourceV3SourceType("heartbeat"),
		})
	})
	mux.HandleFunc("POST /v2/alert_sources", func(w http.ResponseWriter, r *http.Request) {
		writeSource(w, http.StatusOK, client.AlertSourceV2{
			Id:         "01SOURCE",
			Name:       "Cron",
			SourceType: client.AlertSourceV2SourceType("heartbeat"),
		})
	})
	mux.HandleFunc("POST /v2/managed_resources", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// The pause. Both versions update by ID, and both fail here. A 422 rather than a 500
	// because the client retries a 500 ten times, which the count below would rather not
	// wait for.
	for _, pattern := range []string{"/v2/alert_sources/{id}", "/v3/alert_sources/{id}"} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			f.updates++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"type":"validation_error","errors":[{"code":"invalid_value","message":"cannot pause"}]}`))
		})
	}

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

// A source that exists but isn't in state can't be adopted by the next apply: Terraform
// creates a second one, and the first keeps monitoring with nobody to pause it. So a failed
// pause must still store what was created.
func TestAlertSourceBetaCreateStoresSourceWhenPauseFails(t *testing.T) {
	api := &fakeHeartbeatPauseAPI{}
	config, objType := alertSourceBetaSchemaType(t)

	plan := tfsdk.Plan(alertSourceBetaConfig(t, map[string]tftypes.Value{
		"name":        stringValue("Cron"),
		"source_type": stringValue("heartbeat"),
		"disabled":    tftypes.NewValue(tftypes.Bool, true),
	}))

	r := &alertSourceBetaResource{resourceConfigurer: withClient(api.start(t))}
	resp := resource.CreateResponse{
		State: tfsdk.State{Schema: config.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected the failed pause to error, got %+v", resp.Diagnostics)
	}
	if api.updates != 1 {
		t.Errorf("expected 1 pause attempt, got %d", api.updates)
	}

	assertAlertSourceStored(t, resp.State)
}

func TestAlertSourceCreateStoresSourceWhenPauseFails(t *testing.T) {
	api := &fakeHeartbeatPauseAPI{}

	var schemaResp resource.SchemaResponse
	NewIncidentAlertSourceResource().Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)
	objType := alertSourceSchemaType(t)

	attributes := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	attributes["name"] = stringValue("Cron")
	attributes["source_type"] = stringValue("heartbeat")
	attributes["disabled"] = tftypes.NewValue(tftypes.Bool, true)
	attributes["template"] = nullFilledObject(t, objType.AttributeTypes["template"])

	r := &IncidentAlertSourceResource{resourceConfigurer: withClient(api.start(t))}
	resp := resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	r.Create(t.Context(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, attributes)},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected the failed pause to error, got %+v", resp.Diagnostics)
	}
	if api.updates != 1 {
		t.Errorf("expected 1 pause attempt, got %d", api.updates)
	}

	assertAlertSourceStored(t, resp.State)
}

// assertAlertSourceStored checks the created source is recoverable from state, and that we
// didn't record it as paused when it is still monitoring: a stored true would match the
// config, so the next plan would leave a running heartbeat alone.
func assertAlertSourceStored(t *testing.T, state tfsdk.State) {
	t.Helper()

	if state.Raw.IsNull() {
		t.Fatal("expected the created source in state, got no state at all")
	}

	var id types.String
	state.GetAttribute(context.Background(), path.Root("id"), &id)
	if id.ValueString() != "01SOURCE" {
		t.Errorf("expected the created source's ID in state, got %v", id)
	}

	var disabled types.Bool
	state.GetAttribute(context.Background(), path.Root("disabled"), &disabled)
	if disabled.ValueBool() {
		t.Error("a source that failed to pause must not be stored as disabled")
	}
}
