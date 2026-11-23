package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
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

// startFakeAlertRulesList serves the list endpoint ModifyPlan reads to find existing rules,
// filtered by kind and team as the real one is. It counts the requests, so
// a case can show the plan only asks when it needs to.
func startFakeAlertRulesList(t *testing.T, existing ...client.AlertRuleSetV3) (*client.ClientWithResponses, *int) {
	t.Helper()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, "GET /v3/alert_rules", r.Method+" "+r.URL.Path)

		kind, teamID := r.URL.Query().Get("kind"), r.URL.Query().Get("team_id")
		sets := lo.Filter(existing, func(set client.AlertRuleSetV3, _ int) bool {
			return string(set.Kind) == kind && (teamID == "" || lo.FromPtr(set.TeamId) == teamID)
		})

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(client.AlertRulesListResultV3{AlertRuleSets: sets}))
	}))
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	require.NoError(t, err)

	return api, &requests
}

// alertRulesPlanValue is a set of rules of the given kind, or null for none. Team rules belong
// to the team 01TEAM.
func alertRulesPlanValue(t *testing.T, s schema.Schema, id, kind string) tftypes.Value {
	t.Helper()

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(t.Context()), nil)}
	if kind != "" {
		model := models.AlertRuleSetResourceModel{
			ID:                types.StringValue(id),
			UnlockInDashboard: types.BoolNull(),
			Kind:              types.StringValue(kind),
			TeamID:            types.StringNull(),
			Version:           types.Int64Null(),
		}
		if kind == string(client.AlertRuleSetV3KindTeam) {
			model.TeamID = types.StringValue("01TEAM")
		}
		if id == "" {
			model.ID = types.StringUnknown()
		}
		require.False(t, plan.Set(t.Context(), &model).HasError())
	}

	return plan.Raw
}

// modifyAlertRulesPlan plans kind over a prior set of priorKind, where an empty priorKind is a create.
func modifyAlertRulesPlan(t *testing.T, api *client.ClientWithResponses, priorKind, kind string) resource.ModifyPlanResponse {
	t.Helper()

	r := &IncidentAlertRulesResource{resourceConfigurer: withClient(api)}
	s := declaredResourceSchema(t.Context(), r)
	id := lo.Ternary(priorKind == "", "", "01EXISTING")

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(t.Context(), resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: alertRulesPlanValue(t, s, id, kind)},
		State: tfsdk.State{Schema: s, Raw: alertRulesPlanValue(t, s, id, priorKind)},
	}, &resp)

	return resp
}

func TestAlertRulesModifyPlan(t *testing.T) {
	required := client.AlertRuleSetV3{Id: "01REQUIRED", Kind: client.AlertRuleSetV3KindRequired}

	t.Run("creating required rules that exist says to import them", func(t *testing.T) {
		api, _ := startFakeAlertRulesList(t, required)

		resp := modifyAlertRulesPlan(t, api, "", "required")

		require.True(t, resp.Diagnostics.HasError())
		withPath, ok := resp.Diagnostics.Errors()[0].(diag.DiagnosticWithPath)
		require.True(t, ok)
		assert.Equal(t, path.Root("kind"), withPath.Path())
		assert.Equal(t, "Alert rules already exist", resp.Diagnostics.Errors()[0].Summary())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), `id = "01REQUIRED"`)
	})

	t.Run("creating required rules fails when the lookup fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		t.Cleanup(server.Close)
		api, err := client.New(t.Context(), "test-key", server.URL, "test")
		require.NoError(t, err)

		resp := modifyAlertRulesPlan(t, api, "", "required")

		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Unable to check for existing alert rules")
	})

	t.Run("creating default rules when only required rules exist", func(t *testing.T) {
		api, _ := startFakeAlertRulesList(t, required)

		resp := modifyAlertRulesPlan(t, api, "", "default")

		assert.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	})

	t.Run("creating rules for a team that has them says to import them", func(t *testing.T) {
		team := client.AlertRuleSetV3{Id: "01TEAMRULES", Kind: client.AlertRuleSetV3KindTeam, TeamId: lo.ToPtr("01TEAM")}
		api, _ := startFakeAlertRulesList(t, required, team)

		resp := modifyAlertRulesPlan(t, api, "", "team")

		require.True(t, resp.Diagnostics.HasError())
		assert.Equal(t, "Alert rules already exist", resp.Diagnostics.Errors()[0].Summary())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "This team already has rules, with ID 01TEAMRULES")
	})

	t.Run("creating rules for a team without them", func(t *testing.T) {
		other := client.AlertRuleSetV3{Id: "01OTHERRULES", Kind: client.AlertRuleSetV3KindTeam, TeamId: lo.ToPtr("01OTHER")}
		api, requests := startFakeAlertRulesList(t, required, other)

		resp := modifyAlertRulesPlan(t, api, "", "team")

		assert.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		assert.Equal(t, 1, *requests)
	})

	t.Run("changing kind fails", func(t *testing.T) {
		api, requests := startFakeAlertRulesList(t, required)

		resp := modifyAlertRulesPlan(t, api, "team", "default")

		require.True(t, resp.Diagnostics.HasError())
		assert.Equal(t, "Kind cannot change", resp.Diagnostics.Errors()[0].Summary())
		assert.Zero(t, *requests)
	})

	t.Run("updating required rules keeps their kind", func(t *testing.T) {
		api, requests := startFakeAlertRulesList(t, required)

		resp := modifyAlertRulesPlan(t, api, "required", "required")

		assert.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		assert.Zero(t, *requests)
	})
}
