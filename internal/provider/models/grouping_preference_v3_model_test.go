package models

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func groupingPreferenceSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{"default": GroupingDefaultAttribute("The default.")},
		Blocks:     GroupingPreferenceBlocks(),
	}
}

func groupingPreferenceState(t *testing.T, m GroupingPreferenceModel) tftypes.Value {
	t.Helper()
	ctx := context.Background()

	s := groupingPreferenceSchema()
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("writing the model to state: %v", diags)
	}

	return state.Raw
}

func requireSameGroupingState(t *testing.T, want, got GroupingPreferenceModel) {
	t.Helper()

	wantState, gotState := groupingPreferenceState(t, want), groupingPreferenceState(t, got)
	if !wantState.Equal(gotState) {
		diffs, _ := wantState.Diff(gotState)
		t.Fatalf("state differs, which Terraform reports as a change:\n%v", diffs)
	}
}

// groupingResponse answers a write the way the API does: it assigns an ID to each new
// branch, returns an operation alias in its canonical form, and returns expressions in its
// own order.
func groupingResponse(t *testing.T, payload client.TeamGroupingPreferencesUpdatePayloadV3) client.TeamGroupingPreferenceV3 {
	t.Helper()

	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var stored client.TeamGroupingPreferencesUpdatePayloadV3
	require.NoError(t, json.Unmarshal(raw, &stored))

	asStored := func(settings client.TeamGroupingSettingsV3) client.TeamGroupingSettingsV3 {
		settings.AiEnabled = lo.ToPtr(lo.FromPtr(settings.AiEnabled))
		return settings
	}
	branches := lo.FromPtr(stored.Branches)
	for idx := range branches {
		branch := &branches[idx]
		if branch.Id == nil {
			branch.Id = lo.ToPtr(fmt.Sprintf("01BRANCH%d", idx))
		}
		branch.Settings = asStored(branch.Settings)
		for gi := range branch.ConditionGroups {
			for ci := range branch.ConditionGroups[gi].Conditions {
				if branch.ConditionGroups[gi].Conditions[ci].Operation == "one_of" {
					branch.ConditionGroups[gi].Conditions[ci].Operation = "contains_one_of"
				}
			}
		}
	}

	expressions := slices.Clone(lo.FromPtr(stored.Expressions))
	slices.Reverse(expressions)

	return client.TeamGroupingPreferenceV3{
		Id:          "01PREFERENCE",
		TeamId:      "01TEAM",
		Version:     stored.Version,
		Default:     client.TeamGroupingBranchV3{Settings: asStored(stored.Default.Settings)},
		Branches:    branches,
		Expressions: expressions,
	}
}

func applyGroupingPreference(t *testing.T, plan GroupingPreferenceModel) GroupingPreferenceModel {
	t.Helper()

	payload, err := plan.ToTeamUpdatePayload(2)
	require.NoError(t, err)
	response := groupingResponse(t, payload)

	return GroupingPreferenceModel{}.FromAPI(response.Default, response.Branches, response.Expressions, &plan)
}

func groupingOn(windowSeconds int64, keys ...string) *TeamGroupingPreferenceSettingsModel {
	return &TeamGroupingPreferenceSettingsModel{
		Enabled:   types.BoolValue(true),
		AIEnabled: types.BoolValue(false),
		GroupingKeys: lo.Map(keys, func(key string, _ int) AlertRouteGroupingKey {
			return AlertRouteGroupingKey{Reference: types.StringValue(key)}
		}),
		WindowSeconds: types.Int64Value(windowSeconds),
		WindowType:    types.StringValue("rolling"),
	}
}

func groupingOff() *TeamGroupingPreferenceSettingsModel {
	return &TeamGroupingPreferenceSettingsModel{
		Enabled:       types.BoolValue(false),
		AIEnabled:     types.BoolValue(false),
		WindowSeconds: types.Int64Null(),
		WindowType:    types.StringNull(),
	}
}

func groupingPreferenceModel(def *TeamGroupingPreferenceSettingsModel, branches ...GroupingBranchModel) GroupingPreferenceModel {
	return GroupingPreferenceModel{
		Default:          &TeamGroupingPreferenceBranchModel{Settings: def},
		Branches:         lo.CoalesceSliceOrEmpty(branches),
		NamedExpressions: []NamedExpression{},
	}
}

// withAssignedIDs is the plan as the API answers it, with an ID on every new branch.
func withAssignedIDs(plan GroupingPreferenceModel) GroupingPreferenceModel {
	plan.Branches = slices.Clone(plan.Branches)
	for idx := range plan.Branches {
		if plan.Branches[idx].ID.IsUnknown() {
			plan.Branches[idx].ID = types.StringValue(fmt.Sprintf("01BRANCH%d", idx))
		}
	}

	return plan
}

func databaseBranches() []GroupingBranchModel {
	return []GroupingBranchModel{
		{
			ID: types.StringUnknown(),
			ConditionGroups: IncidentEngineConditionGroups{
				conditionGroup("alert.attributes.priority", "one_of", bindValues("P1", "P2")),
			},
			Settings: groupingOn(600, "alert.title"),
		},
		{
			ID:              types.StringValue("01EXISTING"),
			ConditionGroups: IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")},
			Settings:        groupingOff(),
		},
	}
}

func TestGroupingPreferenceSchemaIsValid(t *testing.T) {
	s := groupingPreferenceSchema()
	if diags := s.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("the schema is not a valid implementation: %s", diags)
	}

	groupingPreferenceState(t, groupingPreferenceModel(groupingOn(1800), databaseBranches()...))
}

func TestGroupingPreferenceRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		plan GroupingPreferenceModel
	}{
		{
			name: "no branches",
			plan: groupingPreferenceModel(groupingOn(1800, "alert.title")),
		},
		{
			name: "grouping off",
			plan: groupingPreferenceModel(groupingOff()),
		},
		{
			name: "branches keep their order",
			plan: groupingPreferenceModel(groupingOn(1800), databaseBranches()...),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := applyGroupingPreference(t, tc.plan)
			requireSameGroupingState(t, withAssignedIDs(tc.plan), got)

			// A refresh reads back the state it started from.
			refreshed := applyGroupingPreference(t, got)
			requireSameGroupingState(t, got, refreshed)
		})
	}
}

func TestGroupingPreferencePayloads(t *testing.T) {
	plan := groupingPreferenceModel(groupingOff(), databaseBranches()...)
	branches := `[
		{
			"condition_groups": [{"conditions": [{
				"subject": "alert.attributes.priority",
				"operation": "one_of",
				"param_bindings": [{"array_value": [{"literal": "P1"}, {"literal": "P2"}]}]
			}]}],
			"settings": {
				"enabled": true,
				"ai_enabled": false,
				"grouping_keys": [{"reference": "alert.title"}],
				"window_seconds": 600,
				"window_type": "rolling"
			}
		},
		{
			"id": "01EXISTING",
			"condition_groups": [{"conditions": [{"subject": "alert.title", "operation": "is_set", "param_bindings": []}]}],
			"settings": {"enabled": false}
		}
	]`

	teamCreate, err := plan.ToTeamCreatePayload("01TEAM")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"team_id": "01TEAM",
		"default": {"settings": {"enabled": false}},
		"branches": `+branches+`,
		"expressions": []
	}`, string(lo.Must(json.Marshal(teamCreate))))

	teamUpdate, err := plan.ToTeamUpdatePayload(3)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"version": 3,
		"default": {"settings": {"enabled": false}},
		"branches": `+branches+`,
		"expressions": []
	}`, string(lo.Must(json.Marshal(teamUpdate))))

	defaultCreate, err := plan.ToDefaultCreatePayload()
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"default": {"settings": {"enabled": false}},
		"branches": `+branches+`,
		"expressions": []
	}`, string(lo.Must(json.Marshal(defaultCreate))))

	defaultUpdate, err := plan.ToDefaultUpdatePayload(3)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"version": 3,
		"default": {"settings": {"enabled": false}},
		"branches": `+branches+`,
		"expressions": []
	}`, string(lo.Must(json.Marshal(defaultUpdate))))
}

// Omitting branches or expressions on an update keeps what the dashboard last wrote, so an
// empty config has to send both as [] for Terraform to clear them.
func TestGroupingPreferenceSendsEmptyLists(t *testing.T) {
	plan := groupingPreferenceModel(groupingOn(1800))
	plan.Branches = nil
	plan.NamedExpressions = nil

	payload, err := plan.ToDefaultUpdatePayload(2)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"version": 2,
		"default": {"settings": {
			"enabled": true,
			"ai_enabled": false,
			"grouping_keys": [],
			"window_seconds": 1800,
			"window_type": "rolling"
		}},
		"branches": [],
		"expressions": []
	}`, string(lo.Must(json.Marshal(payload))))

	got := GroupingPreferenceModel{}.FromAPI(payload.Default, nil, nil, nil)
	assert.NotNil(t, got.Branches, "an unset block is an empty list, not null")
	assert.NotNil(t, got.NamedExpressions, "an unset block is an empty list, not null")
}

func TestGroupingPreferenceReadsSettingsWhenGroupingIsOff(t *testing.T) {
	off := client.TeamGroupingSettingsV3{Enabled: false, AiEnabled: lo.ToPtr(false)}
	got := GroupingPreferenceModel{}.FromAPI(
		client.TeamGroupingBranchV3{Settings: off},
		[]client.TeamGroupingConditionalBranchV3{{
			Id:              lo.ToPtr("01BRANCH"),
			ConditionGroups: ConditionGroupsToV3Payload(IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")}),
			Settings:        off,
		}},
		nil, nil)

	for _, settings := range []*TeamGroupingPreferenceSettingsModel{got.Default.Settings, got.Branches[0].Settings} {
		assert.Equal(t, groupingOff(), settings)
	}
}

func TestGroupingPreferenceSendsBranchIDs(t *testing.T) {
	plan := groupingPreferenceModel(groupingOff(), GroupingBranchModel{
		ID:              types.StringUnknown(),
		ConditionGroups: IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")},
		Settings:        groupingOn(60),
	})

	payload, err := plan.ToTeamUpdatePayload(2)
	require.NoError(t, err)
	assert.Nil(t, (*payload.Branches)[0].Id, "a new branch is sent without an ID")

	created := applyGroupingPreference(t, plan)
	assert.Equal(t, types.StringValue("01BRANCH0"), created.Branches[0].ID)

	// Once the plan holds the ID, the update sends it back and the server keeps the branch's
	// alert groups.
	created.Branches[0].Settings = groupingOn(120)
	payload, err = created.ToTeamUpdatePayload(3)
	require.NoError(t, err)
	assert.Equal(t, lo.ToPtr("01BRANCH0"), (*payload.Branches)[0].Id)
	assert.Equal(t, types.StringValue("01BRANCH0"), applyGroupingPreference(t, created).Branches[0].ID)
}

func TestGroupingPreferenceExpressionsReferencedByConditions(t *testing.T) {
	plan := groupingPreferenceModel(groupingOff(), GroupingBranchModel{
		ID: types.StringUnknown(),
		ConditionGroups: IncidentEngineConditionGroups{
			conditionGroup(`expressions["team_path"]`, "is_set"),
		},
		Settings: groupingOn(600),
	})
	plan.NamedExpressions = []NamedExpression{
		{
			Name:       types.StringValue("team"),
			StartFrom:  types.StringValue("alert.attributes.team"),
			Operations: []Operation{{First: &EmptyOpts{}}},
		},
		{
			Name:      types.StringValue("team_path"),
			StartFrom: types.StringValue(`expressions["team"]`),
			Operations: []Operation{
				{Navigate: &Navigate{To: types.StringValue("01ESCALATIONPATHATTR")}},
				{First: &EmptyOpts{}},
			},
		},
	}

	payload, err := plan.ToTeamUpdatePayload(2)
	require.NoError(t, err)
	assert.Equal(t, []string{"team", "team_path"}, lo.Map(*payload.Expressions,
		func(e client.ExpressionPayloadV3, _ int) string { return e.Reference }))

	got := applyGroupingPreference(t, plan)
	requireSameGroupingState(t, withAssignedIDs(plan), got)
}

func TestCarryBranchIDs(t *testing.T) {
	branch := func(id types.String, subject string) GroupingBranchModel {
		return GroupingBranchModel{
			ID:              id,
			ConditionGroups: IncidentEngineConditionGroups{conditionGroup(subject, "is_set")},
			Settings:        groupingOff(),
		}
	}
	unknown := types.StringUnknown()
	state := []GroupingBranchModel{
		branch(types.StringValue("01A"), "alert.a"),
		branch(types.StringValue("01B"), "alert.b"),
		branch(types.StringValue("01C"), "alert.c"),
	}

	for _, tc := range []struct {
		name string
		plan []GroupingBranchModel
		want []types.String
	}{
		{
			name: "unchanged",
			plan: []GroupingBranchModel{branch(unknown, "alert.a"), branch(unknown, "alert.b"), branch(unknown, "alert.c")},
			want: []types.String{types.StringValue("01A"), types.StringValue("01B"), types.StringValue("01C")},
		},
		{
			name: "a branch removed from the middle",
			plan: []GroupingBranchModel{branch(unknown, "alert.a"), branch(unknown, "alert.c")},
			want: []types.String{types.StringValue("01A"), types.StringValue("01C")},
		},
		{
			name: "a branch inserted at the front",
			plan: []GroupingBranchModel{branch(unknown, "alert.new"), branch(unknown, "alert.a"), branch(unknown, "alert.b"), branch(unknown, "alert.c")},
			want: []types.String{unknown, types.StringValue("01A"), types.StringValue("01B"), types.StringValue("01C")},
		},
		{
			name: "branches reordered",
			plan: []GroupingBranchModel{branch(unknown, "alert.c"), branch(unknown, "alert.a"), branch(unknown, "alert.b")},
			want: []types.String{types.StringValue("01C"), types.StringValue("01A"), types.StringValue("01B")},
		},
		{
			name: "a branch's conditions edited in place",
			plan: []GroupingBranchModel{branch(unknown, "alert.a"), branch(unknown, "alert.edited"), branch(unknown, "alert.c")},
			want: []types.String{types.StringValue("01A"), types.StringValue("01B"), types.StringValue("01C")},
		},
		{
			name: "an ID written in the config wins",
			plan: []GroupingBranchModel{branch(types.StringValue("01A"), "alert.c"), branch(unknown, "alert.a")},
			want: []types.String{types.StringValue("01A"), types.StringValue("01B")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := slices.Clone(tc.plan)
			CarryBranchIDs(plan, state)
			assert.Equal(t, tc.want, lo.Map(plan, func(b GroupingBranchModel, _ int) types.String { return b.ID }))
		})
	}
}
