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
	return declaredResourceSchema(ctx, NewIncidentGroupingPreferenceResource())
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

func groupingTestModel(scope string, teamID types.String, branches ...models.GroupingBranchModel) models.GroupingPreferenceResourceModel {
	return models.GroupingPreferenceResourceModel{
		ID:                types.StringValue("01PREFERENCE"),
		UnlockInDashboard: types.BoolNull(),
		Scope:             types.StringValue(scope),
		TeamID:            teamID,
		Version:           types.Int64Value(1),
		GroupingPreferenceModel: models.GroupingPreferenceModel{
			Default:          &models.TeamGroupingPreferenceBranchModel{Settings: groupingOffSettings()},
			Branches:         lo.CoalesceSliceOrEmpty(branches),
			NamedExpressions: []models.NamedExpression{},
		},
	}
}

func groupingTestValue(ctx context.Context, t *testing.T, m *models.GroupingPreferenceResourceModel) tftypes.Value {
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
	ctx context.Context, t *testing.T, r *IncidentGroupingPreferenceResource, state, plan *models.GroupingPreferenceResourceModel,
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
	state := groupingTestModel(models.GroupingScopeTeam, team,
		groupingTestBranch(types.StringValue("01A"), "alert.a"),
		groupingTestBranch(types.StringValue("01B"), "alert.b"))
	plan := groupingTestModel(models.GroupingScopeTeam, team,
		groupingTestBranch(types.StringUnknown(), "alert.new"),
		groupingTestBranch(types.StringUnknown(), "alert.b"),
		groupingTestBranch(types.StringUnknown(), "alert.a"))

	resp := modifyGroupingPlan(ctx, t, &IncidentGroupingPreferenceResource{}, &state, &plan)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

	var got models.GroupingPreferenceResourceModel
	require.False(t, resp.Plan.Get(ctx, &got).HasError())
	assert.Equal(t,
		[]types.String{types.StringUnknown(), types.StringValue("01B"), types.StringValue("01A")},
		lo.Map(got.Branches, func(b models.GroupingBranchModel, _ int) types.String { return b.ID }))
}

func TestGroupingPreferenceModifyPlanRefusesAMove(t *testing.T) {
	ctx := t.Context()
	team := groupingTestModel(models.GroupingScopeTeam, types.StringValue("01TEAM"))

	for _, tc := range []struct {
		name    string
		plan    models.GroupingPreferenceResourceModel
		summary string
	}{
		{
			name:    "to the organisation default",
			plan:    groupingTestModel(models.GroupingScopeOrganisation, types.StringNull()),
			summary: "Scope cannot change",
		},
		{
			name:    "to another team",
			plan:    groupingTestModel(models.GroupingScopeTeam, types.StringValue("01OTHERTEAM")),
			summary: "Team cannot change",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := modifyGroupingPlan(ctx, t, &IncidentGroupingPreferenceResource{}, &team, &tc.plan)
			require.Len(t, resp.Diagnostics.Errors(), 1)
			assert.Equal(t, tc.summary, resp.Diagnostics.Errors()[0].Summary())
		})
	}
}

// fakeGroupingAPI serves the grouping preference endpoints a plan or an import reaches for.
type fakeGroupingAPI struct {
	teamPreferences []client.TeamGroupingPreferenceV3
	defaults        []client.DefaultGroupingPreferenceV3
	claimedTypes    []string
}

func (f *fakeGroupingAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	notFound := func(w http.ResponseWriter) {
		writeJSONStatus(t, w, http.StatusNotFound, map[string]any{"type": "not_found", "status": 404})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v3/team_grouping_preferences", func(w http.ResponseWriter, r *http.Request) {
		matching := lo.Filter(f.teamPreferences, func(p client.TeamGroupingPreferenceV3, _ int) bool {
			return p.TeamId == r.URL.Query().Get("team_id")
		})
		writeJSON(t, w, client.TeamGroupingPreferencesListResultV3{TeamGroupingPreferences: matching})
	})
	mux.HandleFunc("GET /v3/team_grouping_preferences/{id}", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := lo.Find(f.teamPreferences, func(p client.TeamGroupingPreferenceV3) bool { return p.Id == r.PathValue("id") }); ok {
			writeJSON(t, w, client.TeamGroupingPreferencesShowResultV3{TeamGroupingPreference: p})
			return
		}
		notFound(w)
	})
	mux.HandleFunc("GET /v3/default_grouping_preferences", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, client.DefaultGroupingPreferencesListResultV3{DefaultGroupingPreferences: lo.CoalesceSliceOrEmpty(f.defaults)})
	})
	mux.HandleFunc("GET /v3/default_grouping_preferences/{id}", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := lo.Find(f.defaults, func(p client.DefaultGroupingPreferenceV3) bool { return p.Id == r.PathValue("id") }); ok {
			writeJSON(t, w, client.DefaultGroupingPreferencesShowResultV3{DefaultGroupingPreference: p})
			return
		}
		notFound(w)
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

func (f *fakeGroupingAPI) resource(t *testing.T) *IncidentGroupingPreferenceResource {
	r := &IncidentGroupingPreferenceResource{}
	r.client = f.start(t)
	r.markImportedAsManaged = true

	return r
}

func groupingOffAPI() client.TeamGroupingBranchV3 {
	return client.TeamGroupingBranchV3{Settings: client.TeamGroupingSettingsV3{Enabled: false, AiEnabled: lo.ToPtr(false)}}
}

// A create that the API would refuse because the preference exists fails at plan instead,
// with the ID to import it by.
func TestGroupingPreferenceModifyPlanFindsTheExistingPreference(t *testing.T) {
	ctx := t.Context()
	api := &fakeGroupingAPI{
		teamPreferences: []client.TeamGroupingPreferenceV3{{Id: "01TEAMPREF", TeamId: "01TEAM", Default: groupingOffAPI()}},
		defaults:        []client.DefaultGroupingPreferenceV3{{Id: "01DEFAULT", Default: groupingOffAPI()}},
	}

	for _, tc := range []struct {
		name   string
		plan   models.GroupingPreferenceResourceModel
		wantID string
	}{
		{name: "the organisation default", plan: groupingTestModel(models.GroupingScopeOrganisation, types.StringNull()), wantID: "01DEFAULT"},
		{name: "a team's preference", plan: groupingTestModel(models.GroupingScopeTeam, types.StringValue("01TEAM")), wantID: "01TEAMPREF"},
		{name: "a team with none", plan: groupingTestModel(models.GroupingScopeTeam, types.StringValue("01NEWTEAM"))},
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

// One ID imports a team's preference or the organisation default, and the claim names the
// type the API locks each by.
func TestGroupingPreferenceImportFindsTheScope(t *testing.T) {
	ctx := t.Context()
	s := groupingPreferenceResourceSchema(ctx)

	for _, tc := range []struct {
		id        string
		wantScope string
		wantTeam  types.String
		wantClaim string
	}{
		{id: "01TEAMPREF", wantScope: models.GroupingScopeTeam, wantTeam: types.StringValue("01TEAM"), wantClaim: "team_grouping_preference"},
		{id: "01DEFAULT", wantScope: models.GroupingScopeOrganisation, wantTeam: types.StringNull(), wantClaim: "default_grouping_preference"},
	} {
		t.Run(tc.wantScope, func(t *testing.T) {
			api := &fakeGroupingAPI{
				teamPreferences: []client.TeamGroupingPreferenceV3{{Id: "01TEAMPREF", TeamId: "01TEAM", Version: 2, Default: groupingOffAPI()}},
				defaults:        []client.DefaultGroupingPreferenceV3{{Id: "01DEFAULT", Version: 5, Default: groupingOffAPI()}},
			}
			resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
			api.resource(t).ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, &resp)
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

			var got models.GroupingPreferenceResourceModel
			require.False(t, resp.State.Get(ctx, &got).HasError())
			assert.Equal(t, types.StringValue(tc.id), got.ID)
			assert.Equal(t, types.StringValue(tc.wantScope), got.Scope)
			assert.Equal(t, tc.wantTeam, got.TeamID)
			assert.Equal(t, []string{tc.wantClaim}, api.claimedTypes)
		})
	}

	t.Run("an ID that is neither", func(t *testing.T) {
		api := &fakeGroupingAPI{}
		resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
		api.resource(t).ImportState(ctx, resource.ImportStateRequest{ID: "01MISSING"}, &resp)
		assert.True(t, resp.Diagnostics.HasError())
		assert.Empty(t, api.claimedTypes, "nothing to claim")
	})
}
