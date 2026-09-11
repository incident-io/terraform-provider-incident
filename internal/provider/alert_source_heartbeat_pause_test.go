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

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakeHeartbeatPauseAPI creates alert sources happily and fails every update, which is the
// shape of a create whose follow-up pause doesn't land.
type fakeHeartbeatPauseAPI struct {
	updates int
}

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
	mux.HandleFunc("POST /v2/managed_resources", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// The pause, which fails. A 422 rather than a 500 because the client retries a 500 ten
	// times, which the count below would rather not wait for.
	mux.HandleFunc("/v3/alert_sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.updates++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"type":"validation_error","errors":[{"code":"invalid_value","message":"cannot pause"}]}`))
	})

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
func TestAlertSourceCreateStoresSourceWhenPauseFails(t *testing.T) {
	api := &fakeHeartbeatPauseAPI{}
	config, objType := alertSourceSchemaType(t)

	plan := tfsdk.Plan(alertSourceConfig(t, map[string]tftypes.Value{
		"name":        stringValue("Cron"),
		"source_type": stringValue("heartbeat"),
		"disabled":    tftypes.NewValue(tftypes.Bool, true),
	}))

	r := &alertSourceResource{resourceConfigurer: withClient(api.start(t))}
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
