package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// TestScheduleRotationUpdateAnchor checks when an update sends first_interval_starts_at.
//
// Phasing in a change slides the anchor along the cadence, and state keeps the
// configured value while the slid one lands on the same handover. Sending that
// configured value back on an unrelated edit undoes the slide, which changes who is on
// call without the plan showing anything but the edit.
func TestScheduleRotationUpdateAnchor(t *testing.T) {
	for name, tc := range map[string]struct {
		edit       func(context.Context, *tfsdk.Plan)
		wantAnchor string
	}{
		"a rename leaves the anchor alone": {
			edit: func(ctx context.Context, plan *tfsdk.Plan) {
				plan.SetAttribute(ctx, path.Root("name"), "Renamed")
			},
		},
		"a moved anchor is sent": {
			edit: func(ctx context.Context, plan *tfsdk.Plan) {
				plan.SetAttribute(ctx, path.Root("first_interval_starts_at"), "2024-01-09T14:00:00Z")
			},
			wantAnchor: "2024-01-09T14:00:00Z",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			var sent map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("PUT /v3/schedules/01SCHED/rotations/01ROTA", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Rotation map[string]any `json:"rotation"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decoding request: %v", err)
				}
				sent = body.Rotation

				// The server still holds the anchor a phased rollout slid forward, on
				// the same Monday 09:00 handover as the configured one.
				rotation := rotationFixture()
				rotation.FirstIntervalStartsAt = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
				writeJSON(t, w, map[string]any{"rotation": rotation})
			})
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)

			api, err := client.New(t.Context(), "test-key", server.URL, "test")
			if err != nil {
				t.Fatalf("building client: %v", err)
			}

			var schemaResp resource.SchemaResponse
			NewIncidentScheduleRotationResource().Schema(ctx, resource.SchemaRequest{}, &schemaResp)

			state := tfsdk.State{Schema: schemaResp.Schema, Raw: rotationValue(t, "01SCHED", 1)}
			plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: rotationValue(t, "01SCHED", 1)}
			tc.edit(ctx, &plan)

			r := &IncidentScheduleRotationResource{resourceConfigurer: withClient(api)}
			resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("update failed: %+v", resp.Diagnostics)
			}

			anchor, ok := sent["first_interval_starts_at"]
			switch {
			case tc.wantAnchor == "" && ok:
				t.Errorf("expected the anchor to be left out, but sent %v", anchor)
			case tc.wantAnchor != "" && anchor != tc.wantAnchor:
				t.Errorf("expected anchor %s, sent %v", tc.wantAnchor, anchor)
			}
		})
	}
}
