package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestScheduleRotationResourceSchema builds the schema, which resolves every
// apischema.Docstring call against the embedded OpenAPI schema and panics if a
// definition or property is missing. It's the quickest way to catch a resource built
// against a stale schema.
func TestScheduleRotationResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewIncidentScheduleRotationBetaResource()

	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_schedule_rotation_beta" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, name := range []string{
		"id", "schedule_id", "name", "users", "handovers",
		"first_interval_starts_at", "concurrent_shifts", "working_intervals",
		"rank", "rollout", "effective_from",
	} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing expected attribute %q", name)
		}
	}
}

// londonTZ is the schedule timezone these tests project rotations in. It observes
// daylight saving, so an anchor whose UTC offset changes without its local handover
// time moving is covered.
const londonTZ = "Europe/London"

func rotationFixture() client.ScheduleRotationV3 {
	return client.ScheduleRotationV3{
		Id:                    "01ROTA",
		ScheduleId:            "01SCHED",
		Name:                  "Primary",
		Users:                 []client.UserV2{{Id: "01USER"}},
		ConcurrentShifts:      1,
		FirstIntervalStartsAt: time.Date(2024, 1, 8, 9, 0, 0, 0, time.UTC),
		Handovers: []client.ScheduleRotationHandoverV2{
			{Interval: 1, IntervalType: client.Weekly},
		},
	}
}

// TestScheduleRotationFromAPIWorkingIntervals checks that a rotation with no
// restrictions reads back as an absent attribute. The API always returns
// working_intervals, and omitting it is the only way to say "around the clock" in
// config, so storing [] would diff on every plan.
func TestScheduleRotationFromAPIWorkingIntervals(t *testing.T) {
	rotation := rotationFixture()
	rotation.WorkingIntervals = &[]client.ScheduleRotationWorkingIntervalV2{}

	unrestricted := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{}, londonTZ)
	if unrestricted.WorkingIntervals != nil {
		t.Errorf("no intervals should read back as absent, got %+v", unrestricted.WorkingIntervals)
	}

	rotation.WorkingIntervals = &[]client.ScheduleRotationWorkingIntervalV2{
		{Weekday: client.ScheduleRotationWorkingIntervalV2Weekday("monday"), StartTime: "09:00", EndTime: "17:00"},
	}
	populated := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{}, londonTZ)
	if len(populated.WorkingIntervals) != 1 {
		t.Fatalf("expected one interval, got %+v", populated.WorkingIntervals)
	}
	if populated.WorkingIntervals[0].Weekday.ValueString() != "monday" {
		t.Errorf("unexpected weekday: %+v", populated.WorkingIntervals[0])
	}
}

// TestScheduleRotationFromAPIRank checks Terraform only tracks rank when the config
// asked to. Adopting whatever the server held would make a later plan re-assert an
// order nobody wrote down, fighting anyone who reorders in the dashboard.
func TestScheduleRotationFromAPIRank(t *testing.T) {
	rotation := rotationFixture()
	rotation.Rank = ptr(int64(3))

	unset := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{Rank: types.Int64Null()}, londonTZ)
	if !unset.Rank.IsNull() {
		t.Errorf("rank should stay null when the config doesn't set it, got %v", unset.Rank)
	}

	declared := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{Rank: types.Int64Value(3)}, londonTZ)
	if declared.Rank.ValueInt64() != 3 {
		t.Errorf("expected rank 3, got %v", declared.Rank)
	}
}

// TestScheduleRotationFromAPISchedulingMode covers the case that fails an apply outright:
// a rotation that never stated a mode comes back without one, and the attribute has to
// end up a known null rather than carrying the config's value through.
func TestScheduleRotationFromAPISchedulingMode(t *testing.T) {
	rotation := rotationFixture()
	rotation.SchedulingMode = nil // what the API sends for a rotation with no mode set

	// The create path: nothing in config, nothing in the response. Anything other than a
	// known value here is "provider returned invalid result object after apply".
	unset := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{
		SchedulingMode: types.StringNull(),
	}, londonTZ)
	if unset.SchedulingMode.IsUnknown() {
		t.Error("scheduling_mode must not be left unknown after an apply")
	}
	if !unset.SchedulingMode.IsNull() {
		t.Errorf("expected a null scheduling_mode, got %v", unset.SchedulingMode)
	}

	// The shape that actually broke an apply: the attribute was Computed, so the plan
	// carried an unknown into the projection, which passed it straight back out. Guards
	// the projection itself rather than the schema, so re-adding Computed can't
	// reintroduce it.
	fromUnknown := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{
		SchedulingMode: types.StringUnknown(),
	}, londonTZ)
	if fromUnknown.SchedulingMode.IsUnknown() {
		t.Error("an unknown in config must not survive into state")
	}

	mode := client.ScheduleRotationV3SchedulingModeSequential
	rotation.SchedulingMode = &mode

	declared := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{
		SchedulingMode: types.StringValue("sequential"),
	}, londonTZ)
	if declared.SchedulingMode.ValueString() != "sequential" {
		t.Errorf("expected sequential, got %v", declared.SchedulingMode)
	}

	// Set on the rotation but absent from config — someone chose it in the dashboard.
	// Adopting it would diff against a config that doesn't mention it, on every plan.
	ignored := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{
		SchedulingMode: types.StringNull(),
	}, londonTZ)
	if !ignored.SchedulingMode.IsNull() {
		t.Errorf("scheduling_mode should stay null when the config doesn't set it, got %v", ignored.SchedulingMode)
	}
}

// TestScheduleRotationFirstIntervalStartsAt checks a config that writes the same moment
// a different way doesn't propose an update on every plan.
//
// Two mechanisms share the job. The attribute type reconciles "Z" with "+00:00" itself,
// but documents differing offsets for one instant as NOT equal — so the projection has
// to keep the config's spelling for those.
func TestScheduleRotationFirstIntervalStartsAt(t *testing.T) {
	ctx := context.Background()
	rotation := rotationFixture() // 2024-01-08T09:00:00Z

	for _, configured := range []string{
		"2024-01-08T09:00:00Z",      // the same spelling
		"2024-01-08T09:00:00+00:00", // the same instant, written with a zero offset
		"2024-01-08T10:00:00+01:00", // the same instant, in another zone
	} {
		got := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{
			FirstIntervalStartsAt: timetypes.NewRFC3339ValueMust(configured),
		}, londonTZ).FirstIntervalStartsAt

		equal, diags := got.StringSemanticEquals(ctx, timetypes.NewRFC3339ValueMust(configured))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics for %q: %+v", configured, diags)
		}
		if !equal {
			t.Errorf("state %q would plan a diff against config %q", got.ValueString(), configured)
		}
	}

	// A moment that isn't the one the config asked for is a real change, so the API's
	// value has to win — otherwise the drift would be invisible.
	moved := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{
		FirstIntervalStartsAt: timetypes.NewRFC3339ValueMust("2024-02-01T09:00:00Z"),
	}, londonTZ).FirstIntervalStartsAt
	if moved.ValueString() != "2024-01-08T09:00:00Z" {
		t.Errorf("expected the API's timestamp, got %q", moved.ValueString())
	}

	// No prior value at all, e.g. an import.
	imported := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{}, londonTZ).FirstIntervalStartsAt
	if imported.ValueString() != "2024-01-08T09:00:00Z" {
		t.Errorf("expected the API's timestamp, got %q", imported.ValueString())
	}
}

// TestScheduleRotationTimestampValidation checks a malformed timestamp is caught at
// plan time by the attribute type, rather than surfacing as an API error on apply.
func TestScheduleRotationTimestampValidation(t *testing.T) {
	_, diags := timetypes.NewRFC3339Value("8th January")
	if !diags.HasError() {
		t.Error("expected a diagnostic for a timestamp that isn't RFC3339")
	}
}

// TestScheduleRotationFromAPIEffectiveFrom checks a rotation that has only ever had
// one shape reads back with no effective_from, rather than a zero timestamp.
func TestScheduleRotationFromAPIEffectiveFrom(t *testing.T) {
	only := incidentScheduleRotationBetaFromAPI(rotationFixture(), IncidentScheduleRotationBetaModel{}, londonTZ)
	if !only.EffectiveFrom.IsNull() {
		t.Errorf("expected no effective_from, got %v", only.EffectiveFrom)
	}

	rotation := rotationFixture()
	rotation.EffectiveFrom = ptr(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
	phased := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{}, londonTZ)
	if phased.EffectiveFrom.ValueString() != "2024-03-01T00:00:00Z" {
		t.Errorf("unexpected effective_from: %v", phased.EffectiveFrom)
	}
}

// TestScheduleRotationFromAPIRollout checks rollout survives a read. The API has
// nothing to say about it, so losing it here would drop the attribute out of state
// and plan a change on every run.
func TestScheduleRotationFromAPIRollout(t *testing.T) {
	got := incidentScheduleRotationBetaFromAPI(rotationFixture(), IncidentScheduleRotationBetaModel{
		Rollout: types.StringValue("after_current_shift"),
	}, londonTZ)
	if got.Rollout.ValueString() != "after_current_shift" {
		t.Errorf("expected the configured rollout, got %v", got.Rollout)
	}
}

// TestScheduleRotationAnchorPhasing checks the anchor that comes back from phasing a
// change in doesn't read as drift, while a handover time that really moved does.
//
// The fixture is anchored at 2024-01-08T09:00:00Z — a Monday, 09:00 in London — and
// hands over weekly.
func TestScheduleRotationAnchorPhasing(t *testing.T) {
	for name, tc := range map[string]struct {
		served string // what the rotation comes back with
		want   string // what state should record
	}{
		"phased on by whole weeks": {
			served: "2026-01-05T09:00:00Z", // a later Monday, same local time
			want:   "2024-01-08T09:00:00Z",
		},
		"phased across daylight saving": {
			served: "2026-07-27T08:00:00Z", // a Monday, still 09:00 in London
			want:   "2024-01-08T09:00:00Z",
		},
		"handover time moved": {
			served: "2026-01-05T14:00:00Z", // a Monday, but 14:00
			want:   "2026-01-05T14:00:00Z",
		},
		"handover day moved": {
			served: "2026-01-07T09:00:00Z", // 09:00, but a Wednesday
			want:   "2026-01-07T09:00:00Z",
		},
	} {
		t.Run(name, func(t *testing.T) {
			rotation := rotationFixture()
			served, err := time.Parse(time.RFC3339, tc.served)
			if err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			rotation.FirstIntervalStartsAt = served

			got := incidentScheduleRotationBetaFromAPI(rotation, IncidentScheduleRotationBetaModel{
				FirstIntervalStartsAt: timetypes.NewRFC3339ValueMust("2024-01-08T09:00:00Z"),
			}, londonTZ).FirstIntervalStartsAt

			if got.ValueString() != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got.ValueString())
			}
		})
	}

	// Handovers that alternate have no single slot to be on, so there's nothing to
	// judge a moved anchor against and the served value has to stand.
	alternating := rotationFixture()
	alternating.FirstIntervalStartsAt = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	alternating.Handovers = append(alternating.Handovers,
		client.ScheduleRotationHandoverV2{Interval: 2, IntervalType: client.Weekly})

	got := incidentScheduleRotationBetaFromAPI(alternating, IncidentScheduleRotationBetaModel{
		FirstIntervalStartsAt: timetypes.NewRFC3339ValueMust("2024-01-08T09:00:00Z"),
	}, londonTZ).FirstIntervalStartsAt
	if got.ValueString() != "2026-01-05T09:00:00Z" {
		t.Errorf("expected the served timestamp, got %q", got.ValueString())
	}

	// The timezone comes from a second read of the schedule, which can fail. Without it
	// a slid anchor can't be told from a moved one, so what the config asked for stands
	// rather than a diff being invented.
	unresolved := rotationFixture()
	unresolved.FirstIntervalStartsAt = time.Date(2026, 1, 5, 14, 0, 0, 0, time.UTC)

	kept := incidentScheduleRotationBetaFromAPI(unresolved, IncidentScheduleRotationBetaModel{
		FirstIntervalStartsAt: timetypes.NewRFC3339ValueMust("2024-01-08T09:00:00Z"),
	}, "").FirstIntervalStartsAt
	if kept.ValueString() != "2024-01-08T09:00:00Z" {
		t.Errorf("expected the configured timestamp, got %q", kept.ValueString())
	}
}

// TestRotationLineUpDiffers checks what counts as a change worth phasing in. It
// decides both whether the plan asks the API to preview, and whether the apply sends
// a rollout at all.
func TestRotationLineUpDiffers(t *testing.T) {
	base := func() IncidentScheduleRotationBetaModel {
		return IncidentScheduleRotationBetaModel{
			Name:                  types.StringValue("Primary"),
			Users:                 []types.String{types.StringValue("01USER")},
			Handovers:             []IncidentScheduleRotationBetaHandover{{Interval: types.Int64Value(1), IntervalType: types.StringValue("weekly")}},
			FirstIntervalStartsAt: timetypes.NewRFC3339ValueMust("2024-01-08T09:00:00Z"),
			ConcurrentShifts:      types.Int64Value(1),
			Rank:                  types.Int64Value(1),
		}
	}

	if rotationLineUpDiffers(base(), base()) {
		t.Error("an unchanged rotation should need no rollout")
	}

	// Name and rank belong to the rotation rather than to one of its shapes, so they
	// apply across every shape and never disturb who is on call. A moved handover time
	// gets re-anchored by the phasing, so routing it through a rollout would throw the
	// move away where a plain edit makes it.
	unchanged := base()
	unchanged.Name = types.StringValue("Secondary")
	unchanged.Rank = types.Int64Value(2)
	unchanged.FirstIntervalStartsAt = timetypes.NewRFC3339ValueMust("2024-01-09T14:00:00Z")
	if rotationLineUpDiffers(base(), unchanged) {
		t.Error("a rename, reorder or moved handover time should need no rollout")
	}

	for name, changed := range map[string]func(*IncidentScheduleRotationBetaModel){
		"users":             func(m *IncidentScheduleRotationBetaModel) { m.Users = append(m.Users, types.StringValue("02USER")) },
		"handovers":         func(m *IncidentScheduleRotationBetaModel) { m.Handovers[0].Interval = types.Int64Value(2) },
		"concurrent_shifts": func(m *IncidentScheduleRotationBetaModel) { m.ConcurrentShifts = types.Int64Value(2) },
		// Picks who lands on which shift, so on an uneven rotation it can move who is
		// on call — exactly what a rollout is asked to hold back.
		"scheduling_mode": func(m *IncidentScheduleRotationBetaModel) {
			m.SchedulingMode = types.StringValue("sequential")
		},
		"working_intervals": func(m *IncidentScheduleRotationBetaModel) {
			m.WorkingIntervals = []IncidentScheduleRotationBetaWorkingWindow{{
				Weekday:   types.StringValue("monday"),
				StartTime: types.StringValue("09:00"),
				EndTime:   types.StringValue("17:00"),
			}}
		},
	} {
		t.Run("changing "+name+" needs a rollout", func(t *testing.T) {
			plan := base()
			changed(&plan)
			if !rotationLineUpDiffers(plan, base()) {
				t.Error("expected the change to need phasing in")
			}
		})
	}
}

func ptr[T any](value T) *T {
	return &value
}
