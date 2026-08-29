package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// fakeAPI stands in for the API during a plan, serving the two endpoints
// ModifyPlan reaches for. Counting requests is what proves the plan doesn't call
// out when nothing is at stake — the scaling concern behind this check.
type fakeAPI struct {
	layers    []client.ScheduleLayerV2
	overrides []client.ScheduleOverrideV2

	// rotationStatus and overridesStatus override the response code, for the
	// rotation-already-gone and lookup-failed paths.
	rotationStatus  int
	overridesStatus int

	requests int
	server   *httptest.Server
}

func (f *fakeAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v3/schedules/", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		if f.rotationStatus != 0 {
			w.WriteHeader(f.rotationStatus)
			return
		}

		rotation := rotationFixture()
		rotation.Layers = f.layers
		rotation.ConcurrentShifts = int64(len(f.layers))
		writeJSON(t, w, map[string]any{"rotation": rotation})
	})
	mux.HandleFunc("/v2/schedule_overrides", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		if f.overridesStatus != 0 {
			w.WriteHeader(f.overridesStatus)
			return
		}

		// Honour the rotation filter, so a test can prove we scope the lookup.
		rotationID := r.URL.Query().Get("rotation_id")
		var overrides []client.ScheduleOverrideV2
		for _, override := range f.overrides {
			if rotationID == "" || override.RotationId == rotationID {
				overrides = append(overrides, override)
			}
		}

		writeJSON(t, w, map[string]any{
			"overrides": overrides,
			// A short page with no cursor, which is how the real endpoint says
			// "that's everything".
			"pagination_meta": map[string]any{"page_size": overrideLookupPageSize},
		})
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	api, err := client.New(t.Context(), "test-key", f.server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

func writeJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}

func layer(id, name string) client.ScheduleLayerV2 {
	return client.ScheduleLayerV2{Id: ptr(id), Name: ptr(name)}
}

// override builds an override on a layer, ending endsIn from now — negative for one
// that has already finished.
func override(id, layerID string, endsIn time.Duration) client.ScheduleOverrideV2 {
	return client.ScheduleOverrideV2{
		Id:         id,
		ScheduleId: "01SCHED",
		RotationId: "01ROTA",
		LayerId:    layerID,
		StartAt:    time.Now().Add(endsIn - time.Hour),
		EndAt:      time.Now().Add(endsIn),
	}
}

// rotationValue builds a raw rotation value against the real schema, so plan and
// state are shaped exactly as Terraform hands them over.
func rotationValue(t *testing.T, scheduleID string, concurrentShifts int64) tftypes.Value {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentScheduleRotationBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}

	return tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "01ROTA"),
		"schedule_id": tftypes.NewValue(tftypes.String, scheduleID),
		"name":        tftypes.NewValue(tftypes.String, "Weekday shadow"),
		"users": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "01USER"),
		}),
		"handovers": tftypes.NewValue(tftypes.List{ElementType: handoverObjectType}, []tftypes.Value{
			tftypes.NewValue(handoverObjectType, map[string]tftypes.Value{
				"interval":      tftypes.NewValue(tftypes.Number, 1),
				"interval_type": tftypes.NewValue(tftypes.String, "weekly"),
			}),
		}),
		"first_interval_starts_at": tftypes.NewValue(tftypes.String, "2024-01-08T09:00:00Z"),
		"concurrent_shifts":        tftypes.NewValue(tftypes.Number, concurrentShifts),
		"working_intervals":        tftypes.NewValue(workingIntervalsListType, nil),
		"rank":                     tftypes.NewValue(tftypes.Number, nil),
		"scheduling_mode":          tftypes.NewValue(tftypes.String, nil),
		// No rollout, so these cases are about the overrides alone: a phased change
		// previews its effective_from, which would have the plan calling out even when
		// no layer is lost.
		"rollout":        tftypes.NewValue(tftypes.String, nil),
		"effective_from": tftypes.NewValue(tftypes.String, nil),
	})
}

// modifyRotationPlan runs ModifyPlan against the fake API. A nil plan is a destroy.
func modifyRotationPlan(t *testing.T, api *fakeAPI, state tftypes.Value, plan *tftypes.Value) resource.ModifyPlanResponse {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentScheduleRotationBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	planRaw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), nil)
	if plan != nil {
		planRaw = *plan
	}

	r := &IncidentScheduleRotationBetaResource{resourceConfigurer: withClient(api.start(t))}
	var resp resource.ModifyPlanResponse
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: state},
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
	}, &resp)

	return resp
}

// warnings returns the summary and detail of every warning, so assertions can read
// the copy a user would see.
func warnings(resp resource.ModifyPlanResponse) []string {
	var out []string
	for _, diag := range resp.Diagnostics.Warnings() {
		out = append(out, diag.Summary()+" | "+diag.Detail())
	}

	return out
}

// TestScheduleRotationModifyPlanTrimmedShifts is the case the warning exists for:
// dropping a shift takes its layer, and the overrides on that layer stop applying
// for good.
func TestScheduleRotationModifyPlanTrimmedShifts(t *testing.T) {
	api := &fakeAPI{
		layers: []client.ScheduleLayerV2{layer("01LAYER1", "Primary"), layer("01LAYER2", "Weekday shadow")},
		overrides: []client.ScheduleOverrideV2{
			override("01OVER1", "01LAYER2", 24*time.Hour),
			override("01OVER2", "01LAYER2", 48*time.Hour),
			// On a layer that survives, so untouched by the change.
			override("01OVER3", "01LAYER1", 24*time.Hour),
			// Already finished, so nothing anyone would notice losing.
			override("01OVER4", "01LAYER2", -24*time.Hour),
		},
	}

	plan := rotationValue(t, "01SCHED", 1)
	resp := modifyRotationPlan(t, api, rotationValue(t, "01SCHED", 2), &plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %+v", resp.Diagnostics)
	}

	got := warnings(resp)
	if len(got) != 1 {
		t.Fatalf("expected one warning, got %+v", got)
	}
	for _, want := range []string{
		"2 active or upcoming overrides will stop applying",
		`Reducing concurrent shifts from 2 to 1 on rotation "Weekday shadow"`,
		`This removes shift(s) "Weekday shadow"`,
		"2 active or upcoming overrides will no longer have a shift to apply to",
		"Review your overrides after this change",
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning missing %q, got: %s", want, got[0])
		}
	}
}

// TestScheduleRotationModifyPlanCountAgreement checks the copy reads as English for a
// single override, since "1 active or upcoming overrides" would look broken.
func TestScheduleRotationModifyPlanCountAgreement(t *testing.T) {
	api := &fakeAPI{
		layers:    []client.ScheduleLayerV2{layer("01LAYER1", "Primary"), layer("01LAYER2", "Shadow")},
		overrides: []client.ScheduleOverrideV2{override("01OVER1", "01LAYER2", 24*time.Hour)},
	}

	plan := rotationValue(t, "01SCHED", 1)
	got := warnings(modifyRotationPlan(t, api, rotationValue(t, "01SCHED", 2), &plan))
	if len(got) != 1 || !strings.HasPrefix(got[0], "1 active or upcoming override will stop applying") {
		t.Errorf("unexpected warning: %+v", got)
	}
	// The detail has to agree in number too, which is why the count is only ever
	// followed by a verb that reads the same either way.
	if !strings.Contains(got[0], "1 active or upcoming override will no longer have a shift") {
		t.Errorf("unexpected warning: %+v", got)
	}
}

// TestScheduleRotationModifyPlanQuietChanges checks the warning stays quiet for
// changes that keep every layer — the reason it's worth trusting when it does fire.
// Each of these makes no API call at all, which is what keeps a plan over a large
// schedule cheap.
func TestScheduleRotationModifyPlanQuietChanges(t *testing.T) {
	// Overrides exist throughout, so a warning here would be the broad
	// "this rotation has overrides" reading rather than a real loss.
	newFake := func() *fakeAPI {
		return &fakeAPI{
			layers:    []client.ScheduleLayerV2{layer("01LAYER1", "Primary"), layer("01LAYER2", "Shadow")},
			overrides: []client.ScheduleOverrideV2{override("01OVER1", "01LAYER2", 24*time.Hour)},
		}
	}

	for name, shifts := range map[string]struct{ state, plan int64 }{
		"an edit that keeps the shift count": {2, 2},
		"adding a shift":                     {2, 3},
	} {
		t.Run(name, func(t *testing.T) {
			api := newFake()
			plan := rotationValue(t, "01SCHED", shifts.plan)
			resp := modifyRotationPlan(t, api, rotationValue(t, "01SCHED", shifts.state), &plan)

			if got := warnings(resp); len(got) != 0 {
				t.Errorf("expected no warnings, got %+v", got)
			}
			if api.requests != 0 {
				t.Errorf("expected no API calls, got %d", api.requests)
			}
		})
	}

	// Creating a rotation has no state, so there's nothing it could be losing.
	t.Run("a create", func(t *testing.T) {
		api := newFake()
		var schemaResp resource.SchemaResponse
		NewIncidentScheduleRotationBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
		nullState := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), nil)

		plan := rotationValue(t, "01SCHED", 1)
		resp := modifyRotationPlan(t, api, nullState, &plan)

		if got := warnings(resp); len(got) != 0 {
			t.Errorf("expected no warnings, got %+v", got)
		}
		if api.requests != 0 {
			t.Errorf("expected no API calls, got %d", api.requests)
		}
	})
}

// TestScheduleRotationModifyPlanDestroy checks a destroy warns about every override on
// the rotation, not just those on a trailing layer — the whole rotation goes.
func TestScheduleRotationModifyPlanDestroy(t *testing.T) {
	api := &fakeAPI{
		layers: []client.ScheduleLayerV2{layer("01LAYER1", "Primary"), layer("01LAYER2", "Shadow")},
		overrides: []client.ScheduleOverrideV2{
			override("01OVER1", "01LAYER1", 24*time.Hour),
			override("01OVER2", "01LAYER2", 24*time.Hour),
			override("01OVER3", "01LAYER1", -24*time.Hour),
			// On another rotation of the same schedule, so not ours to warn about.
			func() client.ScheduleOverrideV2 {
				other := override("01OVER4", "01LAYER9", 24*time.Hour)
				other.RotationId = "01OTHER"
				return other
			}(),
		},
	}

	got := warnings(modifyRotationPlan(t, api, rotationValue(t, "01SCHED", 2), nil))
	if len(got) != 1 {
		t.Fatalf("expected one warning, got %+v", got)
	}
	for _, want := range []string{
		"2 active or upcoming overrides will stop applying",
		`Destroying rotation "Weekday shadow" stops 2 active or upcoming overrides on it from applying.`,
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning missing %q, got: %s", want, got[0])
		}
	}
}

// TestScheduleRotationModifyPlanMovedSchedule checks moving a rotation between
// schedules warns. It replaces the rotation, and the replacement mints new layers, so
// every override is left behind — even though the shift count hasn't changed.
func TestScheduleRotationModifyPlanMovedSchedule(t *testing.T) {
	api := &fakeAPI{
		layers:    []client.ScheduleLayerV2{layer("01LAYER1", "Primary")},
		overrides: []client.ScheduleOverrideV2{override("01OVER1", "01LAYER1", 24*time.Hour)},
	}

	plan := rotationValue(t, "01OTHERSCHED", 1)
	got := warnings(modifyRotationPlan(t, api, rotationValue(t, "01SCHED", 1), &plan))
	if len(got) != 1 {
		t.Fatalf("expected one warning, got %+v", got)
	}
	if !strings.Contains(got[0], `Moving rotation "Weekday shadow" to another schedule replaces it`) {
		t.Errorf("unexpected warning: %s", got[0])
	}
}

// TestScheduleRotationModifyPlanRotationGone checks a destroy of a rotation the API no
// longer has stays silent, rather than reporting a lookup failure on a plan that's
// about to succeed.
func TestScheduleRotationModifyPlanRotationGone(t *testing.T) {
	api := &fakeAPI{overridesStatus: http.StatusNotFound}

	if got := warnings(modifyRotationPlan(t, api, rotationValue(t, "01SCHED", 2), nil)); len(got) != 0 {
		t.Errorf("expected no warnings, got %+v", got)
	}
}

// TestScheduleRotationModifyPlanLookupFailed checks a broken lookup says so instead of
// failing the plan. The check is advice — the apply doesn't depend on it, so a bad
// response shouldn't stop anyone editing a schedule.
func TestScheduleRotationModifyPlanLookupFailed(t *testing.T) {
	api := &fakeAPI{
		layers: []client.ScheduleLayerV2{layer("01LAYER1", "Primary"), layer("01LAYER2", "Shadow")},
		// Any failure the client won't retry. A 500 exercises the same path, but the
		// client retries it ten times with backoff, so the test would spend three
		// minutes proving the retry policy rather than this plan behaviour.
		overridesStatus: http.StatusForbidden,
	}

	plan := rotationValue(t, "01SCHED", 1)
	resp := modifyRotationPlan(t, api, rotationValue(t, "01SCHED", 2), &plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a failed check should not fail the plan: %+v", resp.Diagnostics)
	}

	got := warnings(resp)
	if len(got) != 1 || !strings.HasPrefix(got[0], "Couldn't check for affected schedule overrides") {
		t.Errorf("unexpected warnings: %+v", got)
	}
}

// TestScheduleRotationModifyPlanUnconfigured checks ModifyPlan is a no-op before
// Configure has run, which is how Terraform validates a configuration.
func TestScheduleRotationModifyPlanUnconfigured(t *testing.T) {
	var schemaResp resource.SchemaResponse
	NewIncidentScheduleRotationBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	plan := rotationValue(t, "01SCHED", 1)
	r := &IncidentScheduleRotationBetaResource{}
	var resp resource.ModifyPlanResponse
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: rotationValue(t, "01SCHED", 2)},
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: plan},
	}, &resp)

	if resp.Diagnostics.HasError() || len(warnings(resp)) != 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
}

// withAttribute returns a rotation with one attribute replaced, so a case can vary a
// single field without restating the whole thing.
func withAttribute(t *testing.T, rotation tftypes.Value, name string, value tftypes.Value) tftypes.Value {
	t.Helper()

	// As hands back the value's own map, so copy first: mutating it in place would
	// change the rotation we were handed too, leaving a plan built from state equal to
	// it and quietly passing every "this changed" check.
	var attributes map[string]tftypes.Value
	if err := rotation.Copy().As(&attributes); err != nil {
		t.Fatalf("unpacking rotation: %v", err)
	}
	attributes[name] = value

	return tftypes.NewValue(rotation.Type(), attributes)
}

// effectiveFrom returns a rotation whose scheduled change lands in from now —
// negative for a cutover that has already happened.
func effectiveFrom(t *testing.T, rotation tftypes.Value, in time.Duration) tftypes.Value {
	t.Helper()

	return withAttribute(t, rotation, "effective_from",
		tftypes.NewValue(tftypes.String, time.Now().Add(in).UTC().Format(time.RFC3339)))
}

// TestScheduleRotationModifyPlanSupersededChange is the case this warning exists for: a
// rotation with a change still pending, being edited. A rotation holds one scheduled
// change, so the pending one goes — and after an import there's nothing else to say so,
// since we report the scheduled shape rather than the line-up on call.
func TestScheduleRotationModifyPlanSupersededChange(t *testing.T) {
	api := &fakeAPI{layers: []client.ScheduleLayerV2{layer("01LAYER1", "Primary")}}

	state := effectiveFrom(t, rotationValue(t, "01SCHED", 1), 90*24*time.Hour)
	plan := withAttribute(t, state, "name", tftypes.NewValue(tftypes.String, "Renamed"))

	resp := modifyRotationPlan(t, api, state, &plan)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %+v", resp.Diagnostics)
	}

	got := warnings(resp)
	if len(got) != 1 {
		t.Fatalf("expected one warning, got %+v", got)
	}
	for _, want := range []string{
		"This edits a change that is already scheduled",
		`Rotation "Weekday shadow" has a change scheduled for`,
		"rewrites that change rather than who is on call now",
		"Set rollout to immediate",
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning missing %q, got: %s", want, got[0])
		}
	}

	// State alone settles this, so it costs a plan nothing.
	if api.requests != 0 {
		t.Errorf("expected no API calls, got %d", api.requests)
	}
}

// supersededWarnings runs the check on its own. A rollout that phases in a line-up
// change sends planEffectiveFrom to the preview endpoint, which isn't what these cases
// are about — calling the check directly keeps them to one reason to fail.
func supersededWarnings(t *testing.T, state, plan tftypes.Value) []string {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentScheduleRotationBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	r := &IncidentScheduleRotationBetaResource{}
	var resp resource.ModifyPlanResponse
	r.warnSupersededChange(context.Background(), resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: state},
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: plan},
	}, &resp)

	return warnings(resp)
}

// TestScheduleRotationModifyPlanSupersededChangeWithRollout is the other half: a
// rollout picks its own moment to take over, which discards the one already scheduled
// rather than rewriting what happens at it. Same situation, materially different
// outcome, so the warning has to say something different.
func TestScheduleRotationModifyPlanSupersededChangeWithRollout(t *testing.T) {
	state := effectiveFrom(t, rotationValue(t, "01SCHED", 1), 90*24*time.Hour)
	// Two shifts rather than one, so there's a line-up change for the rollout to phase
	// in — a rollout on its own is never sent, and is covered below.
	plan := withAttribute(t,
		withAttribute(t, state, "rollout", tftypes.NewValue(tftypes.String, "after_current_shift")),
		"concurrent_shifts", tftypes.NewValue(tftypes.Number, 2))

	got := supersededWarnings(t, state, plan)
	if len(got) != 1 {
		t.Fatalf("expected one warning, got %+v", got)
	}
	for _, want := range []string{
		"A scheduled change to this rotation will be replaced",
		`Rotation "Weekday shadow" has a change scheduled for`,
		"Rolling this edit out replaces it, so that change won't happen",
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning missing %q, got: %s", want, got[0])
		}
	}
}

// TestScheduleRotationModifyPlanSupersededChangeReplaced checks a rotation being
// replaced stays quiet: replacing takes the scheduled change with it.
func TestScheduleRotationModifyPlanSupersededChangeReplaced(t *testing.T) {
	api := &fakeAPI{
		layers:    []client.ScheduleLayerV2{layer("01LAYER1", "Primary")},
		overrides: []client.ScheduleOverrideV2{override("01OVER1", "01LAYER1", 24*time.Hour)},
	}

	var schemaResp resource.SchemaResponse
	NewIncidentScheduleRotationBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	state := effectiveFrom(t, rotationValue(t, "01SCHED", 1), 90*24*time.Hour)
	plan := effectiveFrom(t, rotationValue(t, "01OTHERSCHED", 1), 90*24*time.Hour)

	r := &IncidentScheduleRotationBetaResource{resourceConfigurer: withClient(api.start(t))}
	resp := resource.ModifyPlanResponse{RequiresReplace: path.Paths{path.Root("schedule_id")}}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: state},
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: plan},
	}, &resp)

	for _, got := range warnings(resp) {
		if strings.Contains(got, "scheduled") {
			t.Errorf("expected no scheduled-change warning on a replace, got: %s", got)
		}
	}
}

// TestScheduleRotationModifyPlanSupersededChangeQuiet covers the cases that must not
// warn — the ones that decide whether the warning is worth trusting when it does fire.
func TestScheduleRotationModifyPlanSupersededChangeQuiet(t *testing.T) {
	rename := func(t *testing.T, state tftypes.Value) (tftypes.Value, tftypes.Value) {
		return state, withAttribute(t, state, "name", tftypes.NewValue(tftypes.String, "Renamed"))
	}

	for name, build := range map[string]func(*testing.T) (tftypes.Value, tftypes.Value){
		// The overwhelmingly common shape: no cutover has ever happened.
		"no scheduled change": func(t *testing.T) (tftypes.Value, tftypes.Value) {
			return rename(t, rotationValue(t, "01SCHED", 1))
		},
		// A rotation whose line-up changed in the past keeps that dated version, and
		// it's the one we report — so editing it supersedes nothing.
		"a cutover that already happened": func(t *testing.T) (tftypes.Value, tftypes.Value) {
			return rename(t, effectiveFrom(t, rotationValue(t, "01SCHED", 1), -90*24*time.Hour))
		},
		// Nothing is written, so the scheduled change survives — this is the plan
		// straight after importing a rotation that has one pending.
		"a plan that changes nothing": func(t *testing.T) (tftypes.Value, tftypes.Value) {
			state := effectiveFrom(t, rotationValue(t, "01SCHED", 1), 90*24*time.Hour)
			return state, state
		},
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{layers: []client.ScheduleLayerV2{layer("01LAYER1", "Primary")}}
			state, plan := build(t)

			resp := modifyRotationPlan(t, api, state, &plan)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected errors: %+v", resp.Diagnostics)
			}
			if got := warnings(resp); len(got) != 0 {
				t.Errorf("expected no warnings, got %+v", got)
			}
		})
	}

	// A rollout is only sent when there's a line-up to phase in, so an edit that sets one
	// while leaving the line-up alone rewrites the scheduled change like any other.
	t.Run("a rollout with no line-up change", func(t *testing.T) {
		api := &fakeAPI{layers: []client.ScheduleLayerV2{layer("01LAYER1", "Primary")}}

		state := effectiveFrom(t, rotationValue(t, "01SCHED", 1), 90*24*time.Hour)
		plan := withAttribute(t,
			withAttribute(t, state, "rollout", tftypes.NewValue(tftypes.String, "after_current_shift")),
			"name", tftypes.NewValue(tftypes.String, "Renamed"))

		got := warnings(modifyRotationPlan(t, api, state, &plan))
		if len(got) != 1 || !strings.Contains(got[0], "This edits a change that is already scheduled") {
			t.Errorf("expected the rewrite warning, got %+v", got)
		}
	})
}
