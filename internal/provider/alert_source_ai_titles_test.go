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

// fakeAITitlesAPI records the alert_source object of each create and update, and answers
// with a source whose ai_titles_enabled is stored.
type fakeAITitlesAPI struct {
	stored bool
	writes []map[string]json.RawMessage
}

func (f *fakeAITitlesAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	record := func(w http.ResponseWriter, r *http.Request, status int) {
		var body struct {
			AlertSource map[string]json.RawMessage `json:"alert_source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		f.writes = append(f.writes, body.AlertSource)

		// Like the API: a write that leaves the field out keeps the stored value.
		if raw, ok := body.AlertSource["ai_titles_enabled"]; ok {
			if err := json.Unmarshal(raw, &f.stored); err != nil {
				t.Errorf("decoding ai_titles_enabled: %v", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"alert_source": client.AlertSourceV3{
			Id:              "01SOURCE",
			Name:            "Alertmanager",
			SourceType:      client.AlertSourceV3SourceType("alertmanager"),
			AiTitlesEnabled: f.stored,
		}})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v3/alert_sources", func(w http.ResponseWriter, r *http.Request) {
		record(w, r, http.StatusCreated)
	})
	mux.HandleFunc("PUT /v3/alert_sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		record(w, r, http.StatusOK)
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

// sent returns what the last write sent for ai_titles_enabled, and whether it was sent.
func (f *fakeAITitlesAPI) sent(t *testing.T) (bool, bool) {
	t.Helper()

	if len(f.writes) == 0 {
		t.Fatal("expected a write to the API, got none")
	}
	raw, ok := f.writes[len(f.writes)-1]["ai_titles_enabled"]
	if !ok {
		return false, false
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decoding ai_titles_enabled: %v", err)
	}

	return value, true
}

func alertSourceAITitlesValues(aiTitlesEnabled tftypes.Value) map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"name":              stringValue("Alertmanager"),
		"source_type":       stringValue("alertmanager"),
		"ai_titles_enabled": aiTitlesEnabled,
	}
}

func aiTitlesEnabledInState(t *testing.T, state tfsdk.State) types.Bool {
	t.Helper()

	var value types.Bool
	if diags := state.GetAttribute(context.Background(), path.Root("ai_titles_enabled"), &value); diags.HasError() {
		t.Fatalf("reading ai_titles_enabled from state: %+v", diags)
	}

	return value
}

func TestAlertSourceCreateAITitlesEnabled(t *testing.T) {
	cases := []struct {
		name     string
		planned  tftypes.Value
		wantSent bool
		want     bool
	}{
		{
			// Terraform plans an Optional+Computed attribute the config leaves out as
			// unknown. Sending nothing gives the API's default, which is off.
			name:     "not sent when the config leaves it out",
			planned:  tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
			wantSent: false,
			want:     false,
		},
		{
			name:     "sent when the config sets it",
			planned:  tftypes.NewValue(tftypes.Bool, true),
			wantSent: true,
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAITitlesAPI{}
			config, objType := alertSourceSchemaType(t)

			plan := tfsdk.Plan(alertSourceConfig(t, alertSourceAITitlesValues(tc.planned)))

			r := &alertSourceResource{resourceConfigurer: withClient(api.start(t))}
			resp := resource.CreateResponse{
				State: tfsdk.State{Schema: config.Schema, Raw: tftypes.NewValue(objType, nil)},
			}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("create failed: %+v", resp.Diagnostics)
			}

			value, sent := api.sent(t)
			if sent != tc.wantSent {
				t.Errorf("ai_titles_enabled sent = %v, want %v", sent, tc.wantSent)
			}
			if sent && value != tc.want {
				t.Errorf("ai_titles_enabled = %v, want %v", value, tc.want)
			}

			got := aiTitlesEnabledInState(t, resp.State)
			if got.IsUnknown() || got.IsNull() || got.ValueBool() != tc.want {
				t.Errorf("state ai_titles_enabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAlertSourceUpdateAITitlesEnabled(t *testing.T) {
	cases := []struct {
		name string
		// configured is what the config holds. The plan holds the stored value (true)
		// when the config leaves the attribute out, as the plan modifier carries it over.
		configured tftypes.Value
		planned    tftypes.Value
		wantSent   bool
		want       bool
	}{
		{
			// Sending the planned true back would undo a change made in the dashboard
			// since the last refresh, so the config decides.
			name:       "not sent when the config leaves it out",
			configured: tftypes.NewValue(tftypes.Bool, nil),
			planned:    tftypes.NewValue(tftypes.Bool, true),
			wantSent:   false,
			want:       true,
		},
		{
			name:       "sent when the config turns it off",
			configured: tftypes.NewValue(tftypes.Bool, false),
			planned:    tftypes.NewValue(tftypes.Bool, false),
			wantSent:   true,
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAITitlesAPI{stored: true}
			schemaConfig, _ := alertSourceSchemaType(t)

			stateValues := alertSourceAITitlesValues(tftypes.NewValue(tftypes.Bool, true))
			stateValues["id"] = stringValue("01SOURCE")
			state := tfsdk.State(alertSourceConfig(t, stateValues))

			planValues := alertSourceAITitlesValues(tc.planned)
			planValues["id"] = stringValue("01SOURCE")
			plan := tfsdk.Plan(alertSourceConfig(t, planValues))

			config := alertSourceConfig(t, alertSourceAITitlesValues(tc.configured))

			r := &alertSourceResource{resourceConfigurer: withClient(api.start(t))}
			resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaConfig.Schema, Raw: plan.Raw}}
			r.Update(context.Background(), resource.UpdateRequest{Config: config, Plan: plan, State: state}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("update failed: %+v", resp.Diagnostics)
			}

			value, sent := api.sent(t)
			if sent != tc.wantSent {
				t.Errorf("ai_titles_enabled sent = %v, want %v", sent, tc.wantSent)
			}
			if sent && value != tc.want {
				t.Errorf("ai_titles_enabled = %v, want %v", value, tc.want)
			}

			if got := aiTitlesEnabledInState(t, resp.State); got.ValueBool() != tc.want {
				t.Errorf("state ai_titles_enabled = %v, want %v", got, tc.want)
			}
		})
	}
}
