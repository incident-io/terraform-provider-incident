package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// scheduleV2PlanWith builds a plan holding a single rotation version that uses
// the given timestamp literals, which is all buildModel reads from the plan.
func scheduleV2PlanWith(effectiveFrom, handoverStartAt types.String) *models.IncidentScheduleResourceModelV2 {
	return &models.IncidentScheduleResourceModelV2{
		ID:       types.StringValue("01SCHED"),
		Name:     types.StringValue("Platform on-call"),
		Timezone: types.StringValue("Europe/London"),
		TeamIDs:  types.SetNull(types.StringType),
		Rotations: []models.RotationV2{
			{
				ID:   types.StringValue("weekdays"),
				Name: types.StringValue("Weekdays"),
				Versions: []models.RotationVersionV2{
					{
						EffectiveFrom:   effectiveFrom,
						HandoverStartAt: handoverStartAt,
						Users:           []types.String{types.StringValue("01USER")},
						Handovers: []models.HandoverV2{
							{Interval: types.Int64Value(1), IntervalType: types.StringValue("daily")},
						},
						Layers: []models.LayerV2{
							{ID: types.StringValue("layer"), Name: types.StringValue("Layer")},
						},
					},
				},
			},
		},
	}
}

// scheduleV2Response is the schedule the API hands back for that rotation, with
// its timestamps rendered the way the API renders them: UTC, second precision.
func scheduleV2Response(effectiveFrom *time.Time, handoverStartAt time.Time) client.ScheduleV2 {
	return client.ScheduleV2{
		Id:       "01SCHED",
		Name:     "Platform on-call",
		Timezone: "Europe/London",
		Config: &client.ScheduleConfigV2{
			Rotations: []client.ScheduleRotationV2{
				{
					Id:              "weekdays",
					Name:            "Weekdays",
					EffectiveFrom:   effectiveFrom,
					HandoverStartAt: handoverStartAt,
					Handovers: []client.ScheduleRotationHandoverV2{
						{Interval: 1, IntervalType: "daily"},
					},
					Layers: []client.ScheduleLayerV2{
						{Id: lo.ToPtr("layer"), Name: lo.ToPtr("Layer")},
					},
					Users: []client.UserV2{{Id: "01USER"}},
				},
			},
		},
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parsing %q: %v", value, err)
	}

	return parsed
}

// The API re-renders timestamps, so the string it returns for a moment often
// isn't the string the config used for it. Terraform correlates set elements by
// raw value, and rotations and versions are both sets, so any rewriting fails
// the post-apply consistency check on a schedule nothing has changed.
func TestIncidentScheduleBuildModelKeepsPlannedTimestamps(t *testing.T) {
	for _, tc := range []struct {
		name                string
		planEffectiveFrom   string
		planHandoverStartAt string
		apiEffectiveFrom    string
		apiHandoverStartAt  string
		wantEffectiveFrom   string
		wantHandoverStartAt string
	}{
		{
			// What the dashboard's Terraform export writes.
			name:                "millisecond precision",
			planEffectiveFrom:   "2025-06-01T12:00:00.000Z",
			planHandoverStartAt: "2025-06-01T12:00:00.000Z",
			apiEffectiveFrom:    "2025-06-01T12:00:00Z",
			apiHandoverStartAt:  "2025-06-01T12:00:00Z",
			wantEffectiveFrom:   "2025-06-01T12:00:00.000Z",
			wantHandoverStartAt: "2025-06-01T12:00:00.000Z",
		},
		{
			name:                "non-UTC offset",
			planEffectiveFrom:   "2026-04-10T19:00:00-04:00",
			planHandoverStartAt: "2026-04-10T19:00:00-04:00",
			apiEffectiveFrom:    "2026-04-10T23:00:00Z",
			apiHandoverStartAt:  "2026-04-10T23:00:00Z",
			wantEffectiveFrom:   "2026-04-10T19:00:00-04:00",
			wantHandoverStartAt: "2026-04-10T19:00:00-04:00",
		},
		{
			// A moment the API really did change is still reported as changed,
			// rather than papered over with the planned literal.
			name:                "different moment",
			planEffectiveFrom:   "2026-04-10T19:00:00Z",
			planHandoverStartAt: "2026-04-10T19:00:00Z",
			apiEffectiveFrom:    "2026-04-11T19:00:00Z",
			apiHandoverStartAt:  "2026-04-11T19:00:00Z",
			wantEffectiveFrom:   "2026-04-11T19:00:00Z",
			wantHandoverStartAt: "2026-04-11T19:00:00Z",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := scheduleV2PlanWith(
				types.StringValue(tc.planEffectiveFrom),
				types.StringValue(tc.planHandoverStartAt),
			)
			effectiveFrom := mustParseTime(t, tc.apiEffectiveFrom)
			response := scheduleV2Response(&effectiveFrom, mustParseTime(t, tc.apiHandoverStartAt))

			version := (&IncidentScheduleResource{}).buildModel(response, plan).Rotations[0].Versions[0]

			if got := version.EffectiveFrom.ValueString(); got != tc.wantEffectiveFrom {
				t.Errorf("effective_from: got %q, want %q", got, tc.wantEffectiveFrom)
			}
			if got := version.HandoverStartAt.ValueString(); got != tc.wantHandoverStartAt {
				t.Errorf("handover_start_at: got %q, want %q", got, tc.wantHandoverStartAt)
			}
		})
	}
}

// With nothing planned — an import, or a version the config doesn't have — we
// fall back to the API's rendering.
func TestIncidentScheduleBuildModelWithoutAPlan(t *testing.T) {
	effectiveFrom := mustParseTime(t, "2025-06-01T12:00:00Z")
	response := scheduleV2Response(&effectiveFrom, mustParseTime(t, "2025-06-01T12:00:00Z"))

	version := (&IncidentScheduleResource{}).buildModel(response, nil).Rotations[0].Versions[0]

	if got := version.EffectiveFrom.ValueString(); got != "2025-06-01T12:00:00Z" {
		t.Errorf("effective_from: got %q", got)
	}
	if got := version.HandoverStartAt.ValueString(); got != "2025-06-01T12:00:00Z" {
		t.Errorf("handover_start_at: got %q", got)
	}
}

// A version with no effective_from is a rotation's first, and stays null.
func TestIncidentScheduleBuildModelKeepsNullEffectiveFrom(t *testing.T) {
	plan := scheduleV2PlanWith(types.StringNull(), types.StringValue("2025-06-01T12:00:00.000Z"))
	response := scheduleV2Response(nil, mustParseTime(t, "2025-06-01T12:00:00Z"))

	version := (&IncidentScheduleResource{}).buildModel(response, plan).Rotations[0].Versions[0]

	if !version.EffectiveFrom.IsNull() {
		t.Errorf("effective_from: got %q, want null", version.EffectiveFrom.ValueString())
	}
	if got := version.HandoverStartAt.ValueString(); got != "2025-06-01T12:00:00.000Z" {
		t.Errorf("handover_start_at: got %q, want the planned literal", got)
	}
}

// rotationIDs builds a config whose rotations carry the given ids, which is all
// ValidateConfig looks at.
func scheduleV2ConfigWithRotationIDs(t *testing.T, ids []tftypes.Value) tfsdk.Config {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentScheduleResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	rotationsType, ok := objType.AttributeTypes["rotations"].(tftypes.Set)
	if !ok {
		t.Fatalf("rotations is not a set")
	}
	rotationType, ok := rotationsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("a rotation is not an object")
	}
	versionsType := rotationType.AttributeTypes["versions"]

	rotations := lo.Map(ids, func(id tftypes.Value, idx int) tftypes.Value {
		return tftypes.NewValue(rotationType, map[string]tftypes.Value{
			"id": id,
			// Versions play no part in the check, so we leave them null and vary
			// the name instead: a set holds no two equal elements, and in a real
			// config it's the versions that differ between same-id rotations.
			"name":     tftypes.NewValue(tftypes.String, fmt.Sprintf("Rotation %d", idx)),
			"versions": tftypes.NewValue(versionsType, nil),
		})
	})

	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"id":                     tftypes.NewValue(tftypes.String, "01SCHED"),
			"name":                   tftypes.NewValue(tftypes.String, "Platform on-call"),
			"timezone":               tftypes.NewValue(tftypes.String, "Europe/London"),
			"team_ids":               tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, nil),
			"holidays_public_config": tftypes.NewValue(holidaysObjectType, nil),
			"rotations":              tftypes.NewValue(rotationsType, rotations),
		}),
	}
}

func TestIncidentScheduleValidateConfigDuplicateRotationIDs(t *testing.T) {
	id := func(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	for _, tc := range []struct {
		name      string
		ids       []tftypes.Value
		wantError bool
	}{
		{name: "distinct ids", ids: []tftypes.Value{id("weekdays"), id("weekends")}},
		{name: "duplicate ids", ids: []tftypes.Value{id("weekdays"), id("weekdays")}, wantError: true},
		{name: "unknown ids", ids: []tftypes.Value{unknown, unknown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := NewIncidentScheduleResource().(*IncidentScheduleResource)
			if !ok {
				t.Fatalf("NewIncidentScheduleResource did not return a *IncidentScheduleResource")
			}

			var resp resource.ValidateConfigResponse
			r.ValidateConfig(
				context.Background(),
				resource.ValidateConfigRequest{Config: scheduleV2ConfigWithRotationIDs(t, tc.ids)},
				&resp,
			)

			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("HasError() = %v, want %v: %+v", got, tc.wantError, resp.Diagnostics)
			}
			if tc.wantError && resp.Diagnostics.Errors()[0].Summary() != "Duplicate rotation ID" {
				t.Errorf("unexpected error: %+v", resp.Diagnostics)
			}
		})
	}
}
