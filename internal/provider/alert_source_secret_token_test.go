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

const testSecretToken = "incident_io_alert_source_token_01FCNDV6P870EA6S7TK1DSYDG0"

// TestAlertSourceFromAPISecretToken covers the API not telling us the token. It is returned
// only to callers allowed to update the source, and left out of any response that doesn't
// carry one, so a null has to mean "unchanged" rather than "no longer has one".
func TestAlertSourceFromAPISecretToken(t *testing.T) {
	t.Run("keeps the token we hold when the API omits it", func(t *testing.T) {
		config := &alertSourceModel{SecretToken: types.StringValue(testSecretToken)}

		got := fromAPI(t, alertSourceV3("grafana"), config).SecretToken
		if got.ValueString() != testSecretToken {
			t.Errorf("the prior token should be kept when the API omits it, got %v", got)
		}
	})

	t.Run("keeps the token we hold when the API returns it empty", func(t *testing.T) {
		source := alertSourceV3("grafana")
		source.SecretToken = new(string)

		config := &alertSourceModel{SecretToken: types.StringValue(testSecretToken)}
		if got := fromAPI(t, source, config).SecretToken; got.ValueString() != testSecretToken {
			t.Errorf("an empty token is no token, so the prior one should be kept, got %v", got)
		}
	})

	t.Run("takes the API's token when it gives one", func(t *testing.T) {
		rotated := "incident_io_alert_source_token_01ROTATED"
		source := alertSourceV3("grafana")
		source.SecretToken = &rotated

		config := &alertSourceModel{SecretToken: types.StringValue(testSecretToken)}
		if got := fromAPI(t, source, config).SecretToken; got.ValueString() != rotated {
			t.Errorf("the API's answer should win when it gives one, got %v", got)
		}
	})

	// A create plans the attribute unknown and has no prior state, so there is nothing to
	// keep. Storing the unknown would fail the apply outright.
	t.Run("never stores an unknown", func(t *testing.T) {
		config := &alertSourceModel{SecretToken: types.StringUnknown()}

		got := fromAPI(t, alertSourceV3("heartbeat"), config).SecretToken
		if got.IsUnknown() {
			t.Error("an unknown planned value must not be written into state")
		}
		if !got.IsNull() {
			t.Errorf("it should settle as null, got %v", got)
		}
	})

	t.Run("stays null for a source that has no token", func(t *testing.T) {
		if got := fromAPI(t, alertSourceV3("heartbeat"), &alertSourceModel{}).SecretToken; !got.IsNull() {
			t.Errorf("a source with no token should read null, got %v", got)
		}
	})
}

// fakeTokenlessAlertSourceAPI accepts an update and answers with a source carrying no
// secret_token, which is what the API sends back for a source whose token it doesn't repeat.
type fakeTokenlessAlertSourceAPI struct {
	updates int
}

func (f *fakeTokenlessAlertSourceAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v3/alert_sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.updates++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alert_source": client.AlertSourceV3{
				Id:         "01SOURCE",
				Name:       "Grafana",
				SourceType: client.AlertSourceV3SourceType("grafana"),
				Version:    8,
			},
		})
	})
	mux.HandleFunc("POST /v2/managed_resources", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

// An update whose response omits the token used to store null over it, which Terraform
// rejects as "inconsistent values for sensitive attribute": the plan modifier fills the
// attribute from prior state, so the plan holds the token the apply then dropped.
func TestAlertSourceUpdateKeepsSecretTokenWhenAPIOmitsIt(t *testing.T) {
	api := &fakeTokenlessAlertSourceAPI{}
	schemaConfig, objType := alertSourceSchemaType(t)

	// Both the plan and the prior state carry the token: state from the import, the plan
	// from the plan modifier copying it forward.
	existing := map[string]tftypes.Value{
		"id":           stringValue("01SOURCE"),
		"name":         stringValue("Grafana"),
		"source_type":  stringValue("grafana"),
		"secret_token": stringValue(testSecretToken),
		"version":      tftypes.NewValue(tftypes.Number, 7),
	}

	r := &alertSourceResource{resourceConfigurer: withClient(api.start(t))}
	resp := resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaConfig.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan(alertSourceConfig(t, existing)),
		State: tfsdk.State(alertSourceConfig(t, existing)),
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %+v", resp.Diagnostics.Errors())
	}
	if api.updates != 1 {
		t.Errorf("expected 1 update, got %d", api.updates)
	}

	var token types.String
	resp.State.GetAttribute(context.Background(), path.Root("secret_token"), &token)
	if token.ValueString() != testSecretToken {
		t.Errorf("the planned token must survive an update that doesn't return one, got %v", token)
	}
}
