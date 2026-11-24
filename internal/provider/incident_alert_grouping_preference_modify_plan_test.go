package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

func groupingPreferenceResourceSchema(ctx context.Context) schema.Schema {
	return declaredResourceSchema(ctx, NewIncidentAlertGroupingPreferenceResource())
}

func groupingOffSettings() *models.TeamGroupingPreferenceSettingsModel {
	return &models.TeamGroupingPreferenceSettingsModel{
		Enabled:       types.BoolValue(false),
		AIEnabled:     types.BoolValue(false),
		WindowSeconds: types.Int64Null(),
		WindowType:    types.StringNull(),
	}
}

func groupingTestBranch(id types.String, subject string) models.GroupingBranchModel {
	return models.GroupingBranchModel{
		ID: id,
		ConditionGroups: models.IncidentEngineConditionGroups{{Conditions: models.IncidentEngineConditions{{
			Subject:       types.StringValue(subject),
			Operation:     types.StringValue("is_set"),
			ParamBindings: models.IncidentEngineParamBindings{},
		}}}},
		Settings: groupingOffSettings(),
	}
}

func groupingTestModel(kind string, teamID types.String, branches ...models.GroupingBranchModel) models.AlertGroupingPreferenceResourceModel {
	return models.AlertGroupingPreferenceResourceModel{
		ID:                types.StringValue("01PREFERENCE"),
		UnlockInDashboard: types.BoolNull(),
		Kind:              types.StringValue(kind),
		TeamID:            teamID,
		Version:           types.Int64Value(1),
		GroupingPreferenceModel: models.GroupingPreferenceModel{
			Default:          &models.TeamGroupingPreferenceBranchModel{Settings: groupingOffSettings()},
			Branches:         lo.CoalesceSliceOrEmpty(branches),
			NamedExpressions: []models.NamedExpression{},
		},
	}
}

func groupingTestValue(ctx context.Context, t *testing.T, m *models.AlertGroupingPreferenceResourceModel) tftypes.Value {
	t.Helper()

	s := groupingPreferenceResourceSchema(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if m != nil {
		require.False(t, state.Set(ctx, m).HasError())
	}

	return state.Raw
}

// modifyGroupingPlan runs ModifyPlan as Terraform would for a plan from state to plan, with
// state nil for a create.
func modifyGroupingPlan(
	ctx context.Context, t *testing.T, r *IncidentAlertGroupingPreferenceResource, state, plan *models.AlertGroupingPreferenceResourceModel,
) resource.ModifyPlanResponse {
	t.Helper()

	s := groupingPreferenceResourceSchema(ctx)
	planned := tfsdk.Plan{Schema: s, Raw: groupingTestValue(ctx, t, plan)}
	resp := resource.ModifyPlanResponse{Plan: planned}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State:  tfsdk.State{Schema: s, Raw: groupingTestValue(ctx, t, state)},
		Plan:   planned,
		Config: tfsdk.Config{Schema: s, Raw: planned.Raw},
	}, &resp)

	return resp
}

// Terraform plans a changed branch list with every unwritten ID unknown, and ModifyPlan has
// to put each ID back on its own branch: pairing by index would hand one branch's alert
// groups to another.
func TestGroupingPreferenceModifyPlanCarriesBranchIDs(t *testing.T) {
	ctx := t.Context()
	team := types.StringValue("01TEAM")
	state := groupingTestModel(models.GroupingKindTeam, team,
		groupingTestBranch(types.StringValue("01A"), "alert.a"),
		groupingTestBranch(types.StringValue("01B"), "alert.b"))
	plan := groupingTestModel(models.GroupingKindTeam, team,
		groupingTestBranch(types.StringUnknown(), "alert.new"),
		groupingTestBranch(types.StringUnknown(), "alert.b"),
		groupingTestBranch(types.StringUnknown(), "alert.a"))

	resp := modifyGroupingPlan(ctx, t, &IncidentAlertGroupingPreferenceResource{}, &state, &plan)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

	var got models.AlertGroupingPreferenceResourceModel
	require.False(t, resp.Plan.Get(ctx, &got).HasError())
	assert.Equal(t,
		[]types.String{types.StringUnknown(), types.StringValue("01B"), types.StringValue("01A")},
		lo.Map(got.Branches, func(b models.GroupingBranchModel, _ int) types.String { return b.ID }))
}

func TestGroupingPreferenceModifyPlanRefusesAMove(t *testing.T) {
	ctx := t.Context()
	team := groupingTestModel(models.GroupingKindTeam, types.StringValue("01TEAM"))

	for _, tc := range []struct {
		name    string
		plan    models.AlertGroupingPreferenceResourceModel
		summary string
	}{
		{
			name:    "to the organisation default",
			plan:    groupingTestModel(models.GroupingKindDefault, types.StringNull()),
			summary: "Kind cannot change",
		},
		{
			name:    "to another team",
			plan:    groupingTestModel(models.GroupingKindTeam, types.StringValue("01OTHERTEAM")),
			summary: "Team cannot change",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := modifyGroupingPlan(ctx, t, &IncidentAlertGroupingPreferenceResource{}, &team, &tc.plan)
			require.Len(t, resp.Diagnostics.Errors(), 1)
			assert.Equal(t, tc.summary, resp.Diagnostics.Errors()[0].Summary())
		})
	}
}

// fakeGroupingAPI serves the grouping preference endpoints a plan or an import reaches for.
type fakeGroupingAPI struct {
	preferences  []client.AlertGroupingPreferenceV3
	claimedTypes []string
}

func (f *fakeGroupingAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v3/alert_grouping_preferences", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		matching := lo.Filter(f.preferences, func(p client.AlertGroupingPreferenceV3, _ int) bool {
			return (!query.Has("kind") || string(p.Kind) == query.Get("kind")) &&
				(!query.Has("team_id") || lo.FromPtr(p.TeamId) == query.Get("team_id"))
		})
		writeJSON(t, w, client.AlertGroupingPreferencesListResultV3{AlertGroupingPreferences: lo.CoalesceSliceOrEmpty(matching)})
	})
	mux.HandleFunc("GET /v3/alert_grouping_preferences/{id}", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := lo.Find(f.preferences, func(p client.AlertGroupingPreferenceV3) bool { return p.Id == r.PathValue("id") }); ok {
			writeJSON(t, w, client.AlertGroupingPreferencesShowResultV3{AlertGroupingPreference: p})
			return
		}
		writeJSONStatus(t, w, http.StatusNotFound, map[string]any{"type": "not_found", "status": 404})
	})
	mux.HandleFunc("POST /v2/managed_resources", func(w http.ResponseWriter, r *http.Request) {
		var payload client.ManagedResourcesV2CreateManagedResourceJSONRequestBody
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &payload))
		f.claimedTypes = append(f.claimedTypes, string(payload.ResourceType))
		writeJSON(t, w, map[string]any{"managed_resource": map[string]any{
			"id": "01MANAGED", "resource_type": payload.ResourceType, "resource_id": payload.ResourceId, "annotations": map[string]string{},
		}})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	require.NoError(t, err)

	return api
}

func (f *fakeGroupingAPI) resource(t *testing.T) *IncidentAlertGroupingPreferenceResource {
	r := &IncidentAlertGroupingPreferenceResource{}
	r.client = f.start(t)
	r.markImportedAsManaged = true

	return r
}

func groupingOffAPI() client.AlertGroupingDefaultBranchV3 {
	return client.AlertGroupingDefaultBranchV3{Settings: client.AlertGroupingSettingsV3{Enabled: false, AiEnabled: lo.ToPtr(false)}}
}

// groupingPreferencesAPI is one team's preference and the organisation default.
func groupingPreferencesAPI() []client.AlertGroupingPreferenceV3 {
	return []client.AlertGroupingPreferenceV3{
		{Id: "01TEAMPREF", Kind: client.AlertGroupingPreferenceV3KindTeam, TeamId: lo.ToPtr("01TEAM"), Version: 2, Default: groupingOffAPI()},
		{Id: "01DEFAULT", Kind: client.AlertGroupingPreferenceV3KindDefault, Version: 5, Default: groupingOffAPI()},
	}
}

// A create that the API would refuse because the preference exists fails at plan instead,
// with the ID to import it by.
func TestGroupingPreferenceModifyPlanFindsTheExistingPreference(t *testing.T) {
	ctx := t.Context()
	api := &fakeGroupingAPI{preferences: groupingPreferencesAPI()}

	for _, tc := range []struct {
		name   string
		plan   models.AlertGroupingPreferenceResourceModel
		wantID string
	}{
		{name: "the organisation default", plan: groupingTestModel(models.GroupingKindDefault, types.StringNull()), wantID: "01DEFAULT"},
		{name: "a team's preference", plan: groupingTestModel(models.GroupingKindTeam, types.StringValue("01TEAM")), wantID: "01TEAMPREF"},
		{name: "a team with none", plan: groupingTestModel(models.GroupingKindTeam, types.StringValue("01NEWTEAM"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.plan.ID, tc.plan.Version = types.StringUnknown(), types.Int64Unknown()
			resp := modifyGroupingPlan(ctx, t, api.resource(t), nil, &tc.plan)

			if tc.wantID == "" {
				assert.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
				return
			}
			require.Len(t, resp.Diagnostics.Errors(), 1)
			assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "import {")
			assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), `id = "`+tc.wantID+`"`)
		})
	}
}

// A failed lookup fails the plan, so a replace can't hand the preference back before its
// create fails.
func TestGroupingPreferenceModifyPlanFailsWhenTheLookupFails(t *testing.T) {
	ctx := t.Context()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	require.NoError(t, err)

	r := &IncidentAlertGroupingPreferenceResource{}
	r.client = api
	plan := groupingTestModel(models.GroupingKindDefault, types.StringNull())
	plan.ID, plan.Version = types.StringUnknown(), types.Int64Unknown()
	resp := modifyGroupingPlan(ctx, t, r, nil, &plan)

	require.Len(t, resp.Diagnostics.Errors(), 1)
	assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Unable to check for an existing grouping preference")
}

// One ID imports a team's preference or the organisation default, and both are claimed as
// alert grouping preferences.
func TestGroupingPreferenceImportFindsTheKind(t *testing.T) {
	ctx := t.Context()
	s := groupingPreferenceResourceSchema(ctx)

	for _, tc := range []struct {
		id       string
		wantKind string
		wantTeam types.String
	}{
		{id: "01TEAMPREF", wantKind: models.GroupingKindTeam, wantTeam: types.StringValue("01TEAM")},
		{id: "01DEFAULT", wantKind: models.GroupingKindDefault, wantTeam: types.StringNull()},
	} {
		t.Run(tc.wantKind, func(t *testing.T) {
			api := &fakeGroupingAPI{preferences: groupingPreferencesAPI()}
			resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
			api.resource(t).ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, &resp)
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

			var got models.AlertGroupingPreferenceResourceModel
			require.False(t, resp.State.Get(ctx, &got).HasError())
			assert.Equal(t, types.StringValue(tc.id), got.ID)
			assert.Equal(t, types.StringValue(tc.wantKind), got.Kind)
			assert.Equal(t, tc.wantTeam, got.TeamID)
			assert.Equal(t, []string{"alert_grouping_preference"}, api.claimedTypes)
		})
	}

	t.Run("an ID that matches nothing", func(t *testing.T) {
		api := &fakeGroupingAPI{}
		resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
		api.resource(t).ImportState(ctx, resource.ImportStateRequest{ID: "01MISSING"}, &resp)
		assert.True(t, resp.Diagnostics.HasError())
		assert.Empty(t, api.claimedTypes, "nothing to claim")
	})
}
