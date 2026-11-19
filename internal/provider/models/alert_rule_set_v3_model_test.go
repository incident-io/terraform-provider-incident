package models

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func alertRuleSetSchema() schema.Schema {
	attributes := AlertRuleSetAttributes()
	attributes["unlock_in_dashboard"] = schema.BoolAttribute{Optional: true}

	return schema.Schema{Attributes: attributes, Blocks: AlertRuleSetBlocks()}
}

func alertRuleSetState(t *testing.T, m AlertRuleSetResourceModel) tftypes.Value {
	t.Helper()
	ctx := context.Background()

	s := alertRuleSetSchema()
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("writing the model to state: %v", diags)
	}

	return state.Raw
}

func requireSameState(t *testing.T, want, got AlertRuleSetResourceModel) {
	t.Helper()

	wantState, gotState := alertRuleSetState(t, want), alertRuleSetState(t, got)
	if !wantState.Equal(gotState) {
		diffs, _ := wantState.Diff(gotState)
		t.Fatalf("state differs, which Terraform reports as a change:\n%v", diffs)
	}
}

// alertRuleSetResponse answers a write the way the API does, in its stored shape.
func alertRuleSetResponse(t *testing.T, kind string, teamID *string, blocks []client.AlertRuleBlockV3, expressions []client.ExpressionPayloadV3) client.AlertRuleSetV3 {
	t.Helper()

	raw, err := json.Marshal(blocks)
	require.NoError(t, err)
	var stored []client.AlertRuleBlockV3
	require.NoError(t, json.Unmarshal(raw, &stored))

	asStored := func(groups []client.ConditionGroupPayloadV3) []client.ConditionGroupPayloadV3 {
		for gi := range groups {
			for ci := range groups[gi].Conditions {
				if groups[gi].Conditions[ci].Operation == "one_of" {
					groups[gi].Conditions[ci].Operation = "contains_one_of"
				}
				for pi, binding := range groups[gi].Conditions[ci].ParamBindings {
					if binding.ArrayValue != nil && len(*binding.ArrayValue) == 0 {
						groups[gi].Conditions[ci].ParamBindings[pi].ArrayValue = nil
					}
				}
			}
		}
		return lo.CoalesceSliceOrEmpty(groups)
	}
	for bi := range stored {
		for ri := range stored[bi].Branches {
			branch := &stored[bi].Branches[ri]
			branch.ConditionGroups = asStored(branch.ConditionGroups)
			if branch.Branches != nil {
				for ni := range *branch.Branches {
					(*branch.Branches)[ni].ConditionGroups = asStored((*branch.Branches)[ni].ConditionGroups)
				}
				if len(*branch.Branches) == 0 {
					branch.Branches = nil
				}
			}
		}
	}

	expressions = slices.Clone(expressions)
	slices.Reverse(expressions)

	return client.AlertRuleSetV3{
		Id:          "01SET",
		Kind:        client.AlertRuleSetV3Kind(kind),
		TeamId:      teamID,
		Version:     1,
		Blocks:      stored,
		Expressions: expressions,
	}
}

func applyAlertRuleSet(t *testing.T, plan AlertRuleSetResourceModel) AlertRuleSetResourceModel {
	t.Helper()

	payload, err := plan.ToCreatePayload()
	require.NoError(t, err)

	return AlertRuleSetResourceModel{}.FromAPI(
		alertRuleSetResponse(t, string(payload.Kind), payload.TeamId, payload.Blocks, *payload.Expressions), &plan)
}

func applied(plan AlertRuleSetResourceModel) AlertRuleSetResourceModel {
	plan.ID = types.StringValue("01SET")
	plan.Version = types.Int64Value(1)

	return plan
}

func conditionGroup(subject, operation string, bindings ...IncidentEngineParamBinding) IncidentEngineConditionGroup {
	return IncidentEngineConditionGroup{Conditions: IncidentEngineConditions{{
		Subject:       types.StringValue(subject),
		Operation:     types.StringValue(operation),
		ParamBindings: IncidentEngineParamBindings(lo.CoalesceSliceOrEmpty(bindings)),
	}}}
}

func alertRuleSetModel(kind string, teamID types.String, blocks ...AlertRuleBlockModel) AlertRuleSetResourceModel {
	return AlertRuleSetResourceModel{
		ID:                types.StringUnknown(),
		UnlockInDashboard: types.BoolNull(),
		Kind:              types.StringValue(kind),
		TeamID:            teamID,
		Version:           types.Int64Unknown(),
		Blocks:            lo.CoalesceSliceOrEmpty(blocks),
		NamedExpressions:  []NamedExpression{},
	}
}

func pageOnHighPriority() AlertRuleBlockModel {
	return AlertRuleBlockModel{
		ID: types.StringValue("page-owning-team"),
		Branches: []AlertRuleBranchModel{
			{
				ID: types.StringValue("high-priority"),
				ConditionGroups: IncidentEngineConditionGroups{
					conditionGroup("alert.attributes.priority", "one_of", bindValues("P1", "P2")),
					conditionGroup("alert.title", "contains", bindValueLiteral("database")),
				},
				Branches: []AlertRuleNestedBranchModel{{
					ID:              types.StringValue("high-priority-grafana"),
					ConditionGroups: IncidentEngineConditionGroups{conditionGroup("alert.source", "one_of", bindValues("grafana"))},
				}},
			},
			{
				ID:       types.StringValue("otherwise"),
				Branches: []AlertRuleNestedBranchModel{},
			},
		},
	}
}

func TestAlertRuleSetSchemaIsValid(t *testing.T) {
	s := alertRuleSetSchema()
	if diags := s.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("the schema is not a valid implementation: %s", diags)
	}

	alertRuleSetState(t, alertRuleSetModel("team", types.StringValue("01TEAM"), pageOnHighPriority()))
}

func TestAlertRuleSetRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		plan AlertRuleSetResourceModel
	}{
		{
			name: "required rules",
			plan: alertRuleSetModel("required", types.StringNull(), pageOnHighPriority()),
		},
		{
			name: "default rules with no blocks",
			plan: alertRuleSetModel("default", types.StringNull()),
		},
		{
			name: "team rules",
			plan: alertRuleSetModel("team", types.StringValue("01TEAM"), pageOnHighPriority()),
		},
		{
			name: "blocks, branches and condition groups keep their order",
			plan: alertRuleSetModel("team", types.StringValue("01TEAM"),
				AlertRuleBlockModel{ID: types.StringValue("zulu"), Branches: []AlertRuleBranchModel{{
					ID: types.StringValue("zulu-if"),
					ConditionGroups: IncidentEngineConditionGroups{
						conditionGroup("alert.title", "is_set"),
						conditionGroup("alert.description", "is_set"),
						conditionGroup("alert.source", "is_set"),
					},
					Branches: []AlertRuleNestedBranchModel{
						{ID: types.StringValue("zulu-nested-b"), ConditionGroups: IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")}},
						{ID: types.StringValue("zulu-nested-a")},
					},
				}}},
				AlertRuleBlockModel{ID: types.StringValue("alpha"), Branches: []AlertRuleBranchModel{
					{ID: types.StringValue("alpha-if"), ConditionGroups: IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")}, Branches: []AlertRuleNestedBranchModel{}},
					{ID: types.StringValue("alpha-else"), Branches: []AlertRuleNestedBranchModel{}},
				}},
			),
		},
		{
			name: "actions on a branch and a nested branch",
			plan: alertRuleSetModel("team", types.StringValue("01TEAM"),
				AlertRuleBlockModel{ID: types.StringValue("page"), Branches: []AlertRuleBranchModel{{
					ID:      types.StringValue("always"),
					Actions: []AlertRuleActionV3Model{escalateAction(AlertRuleEscalateActionV3Model{Users: v3Values("U1")})},
					Branches: []AlertRuleNestedBranchModel{{
						ID:      types.StringValue("investigate"),
						Actions: []AlertRuleActionV3Model{{ID: types.StringValue("look"), Type: types.StringValue("investigate")}},
					}},
				}}},
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := applyAlertRuleSet(t, tc.plan)
			requireSameState(t, applied(tc.plan), got)

			payload, err := got.ToCreatePayload()
			require.NoError(t, err)
			refreshed := AlertRuleSetResourceModel{}.FromAPI(
				alertRuleSetResponse(t, string(payload.Kind), payload.TeamId, payload.Blocks, *payload.Expressions), &got)
			requireSameState(t, got, refreshed)
		})
	}
}

func TestAlertRuleSetCreatePayload(t *testing.T) {
	plan := alertRuleSetModel("team", types.StringValue("01TEAM"), pageOnHighPriority())
	payload, err := plan.ToCreatePayload()
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"kind": "team",
		"team_id": "01TEAM",
		"expressions": [],
		"blocks": [{
			"id": "page-owning-team",
			"branches": [
				{
					"id": "high-priority",
					"condition_groups": [
						{"conditions": [{
							"subject": "alert.attributes.priority",
							"operation": "one_of",
							"param_bindings": [{"array_value": [{"literal": "P1"}, {"literal": "P2"}]}]
						}]},
						{"conditions": [{
							"subject": "alert.title",
							"operation": "contains",
							"param_bindings": [{"array_value": [], "value": {"literal": "database"}}]
						}]}
					],
					"actions": [],
					"branches": [{
						"id": "high-priority-grafana",
						"condition_groups": [{"conditions": [{
							"subject": "alert.source",
							"operation": "one_of",
							"param_bindings": [{"array_value": [{"literal": "grafana"}]}]
						}]}],
						"actions": []
					}]
				},
				{"id": "otherwise", "condition_groups": [], "actions": []}
			]
		}]
	}`, string(lo.Must(json.Marshal(payload))))

	t.Run("required and default rules send no team", func(t *testing.T) {
		payload, err := alertRuleSetModel("default", types.StringNull()).ToCreatePayload()
		require.NoError(t, err)
		assert.JSONEq(t, `{"kind": "default", "blocks": [], "expressions": []}`, string(lo.Must(json.Marshal(payload))))
	})
}

func TestAlertRuleSetUpdatePayload(t *testing.T) {
	payload, err := alertRuleSetModel("team", types.StringValue("01TEAM")).ToUpdatePayload(4)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version": 4, "blocks": [], "expressions": []}`, string(lo.Must(json.Marshal(payload))))
}

func TestAlertRuleSetEmptyConditionGroups(t *testing.T) {
	block := func(conditionGroups IncidentEngineConditionGroups) AlertRuleBlockModel {
		return AlertRuleBlockModel{ID: types.StringValue("block"), Branches: []AlertRuleBranchModel{{
			ID:              types.StringValue("always"),
			ConditionGroups: conditionGroups,
			Branches: []AlertRuleNestedBranchModel{{
				ID:              types.StringValue("nested"),
				ConditionGroups: conditionGroups,
			}},
		}}}
	}

	for name, conditionGroups := range map[string]IncidentEngineConditionGroups{
		"omitted": nil,
		"empty":   {},
	} {
		t.Run(name, func(t *testing.T) {
			plan := alertRuleSetModel("required", types.StringNull(), block(conditionGroups))
			requireSameState(t, applied(plan), applyAlertRuleSet(t, plan))
		})
	}

	t.Run("imported", func(t *testing.T) {
		plan := alertRuleSetModel("required", types.StringNull(), block(IncidentEngineConditionGroups{}))
		payload, err := plan.ToCreatePayload()
		require.NoError(t, err)

		imported := AlertRuleSetResourceModel{}.FromAPI(
			alertRuleSetResponse(t, "required", nil, payload.Blocks, nil), nil)
		assert.Nil(t, imported.Blocks[0].Branches[0].ConditionGroups)
		assert.Nil(t, imported.Blocks[0].Branches[0].Branches[0].ConditionGroups)
	})
}

func TestAlertRuleSetMatchesPriorBranchesByID(t *testing.T) {
	plan := alertRuleSetModel("required", types.StringNull(), pageOnHighPriority())

	moved := plan
	moved.Blocks = []AlertRuleBlockModel{{
		ID:       types.StringValue("first"),
		Branches: []AlertRuleBranchModel{{ID: types.StringValue("new"), Branches: []AlertRuleNestedBranchModel{}}},
	}, pageOnHighPriority()}
	payload, err := moved.ToCreatePayload()
	require.NoError(t, err)

	got := AlertRuleSetResourceModel{}.FromAPI(alertRuleSetResponse(t, "required", nil, payload.Blocks, nil), &plan)

	assert.Nil(t, got.Blocks[0].Branches[0].ConditionGroups, "a branch the prior doesn't have reads as omitted")

	got.Blocks = got.Blocks[1:]
	requireSameState(t, applied(plan), got)
}

// An import reads condition bindings in the shorthand, and a refresh keeps a long-form config.
func TestAlertRuleSetConditionBindingSpelling(t *testing.T) {
	withBindings := func(literal, reference, literals IncidentEngineParamBinding) AlertRuleSetResourceModel {
		return alertRuleSetModel("required", types.StringNull(), AlertRuleBlockModel{
			ID: types.StringValue("block"),
			Branches: []AlertRuleBranchModel{{
				ID:       types.StringValue("branch"),
				Branches: []AlertRuleNestedBranchModel{},
				ConditionGroups: IncidentEngineConditionGroups{
					conditionGroup("alert.title", "contains", literal),
					conditionGroup("alert.attributes.team", "is", reference),
					conditionGroup("alert.attributes.priority", "is_one_of", literals),
				},
			}},
		})
	}
	shorthand := withBindings(bindValueLiteral("database"), bindValueReference("alert.attributes.owner"), bindValues("P1", "P2"))
	longForm := withBindings(bindValue(litValue("database")), bindValue(refValue("alert.attributes.owner")), bindArray(litValue("P1"), litValue("P2")))

	t.Run("import reads the shorthand", func(t *testing.T) {
		payload, err := longForm.ToCreatePayload()
		require.NoError(t, err)

		imported := AlertRuleSetResourceModel{}.FromAPI(alertRuleSetResponse(t, "required", nil, payload.Blocks, nil), nil)
		requireSameState(t, applied(shorthand), imported)
	})

	t.Run("refresh keeps the long form", func(t *testing.T) {
		got := applyAlertRuleSet(t, longForm)
		requireSameState(t, applied(longForm), got)

		payload, err := got.ToCreatePayload()
		require.NoError(t, err)
		refreshed := AlertRuleSetResourceModel{}.FromAPI(alertRuleSetResponse(t, "required", nil, payload.Blocks, nil), &got)
		requireSameState(t, got, refreshed)
	})
}

func TestAlertRuleSetExpressionsRoundTrip(t *testing.T) {
	plan := alertRuleSetModel("team", types.StringValue("01TEAM"), AlertRuleBlockModel{
		ID: types.StringValue("page-team"),
		Branches: []AlertRuleBranchModel{{
			ID:              types.StringValue("has-path"),
			ConditionGroups: IncidentEngineConditionGroups{conditionGroup(`expressions["team_path"]`, "is_set")},
			Branches:        []AlertRuleNestedBranchModel{},
		}},
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

	payload, err := plan.ToCreatePayload()
	require.NoError(t, err)
	assert.Equal(t, []string{"team", "team_path"}, lo.Map(*payload.Expressions,
		func(e client.ExpressionPayloadV3, _ int) string { return e.Reference }))

	requireSameState(t, applied(plan), applyAlertRuleSet(t, plan))
}

func TestValidateAlertRuleSet(t *testing.T) {
	branch := func(id string, groups ...IncidentEngineConditionGroup) AlertRuleBranchModel {
		return AlertRuleBranchModel{ID: types.StringValue(id), ConditionGroups: groups}
	}
	block := func(id string, branches ...AlertRuleBranchModel) AlertRuleBlockModel {
		return AlertRuleBlockModel{ID: types.StringValue(id), Branches: branches}
	}
	isSet := conditionGroup("alert.title", "is_set")

	for _, tc := range []struct {
		name  string
		model AlertRuleSetResourceModel
		want  []path.Path
	}{
		{
			name:  "team rules with a team",
			model: alertRuleSetModel("team", types.StringValue("01TEAM")),
		},
		{
			name:  "team rules without a team",
			model: alertRuleSetModel("team", types.StringNull()),
			want:  []path.Path{path.Root("team_id")},
		},
		{
			name:  "required rules with a team",
			model: alertRuleSetModel("required", types.StringValue("01TEAM")),
			want:  []path.Path{path.Root("team_id")},
		},
		{
			name:  "default rules without a team",
			model: alertRuleSetModel("default", types.StringNull()),
		},
		{
			name:  "a team Terraform hasn't settled",
			model: alertRuleSetModel("team", types.StringUnknown()),
		},
		{
			name: "an if and an else",
			model: alertRuleSetModel("required", types.StringNull(),
				block("block", branch("if", isSet), branch("else"))),
		},
		{
			name: "an else with conditions",
			model: alertRuleSetModel("required", types.StringNull(),
				block("block", branch("if", isSet), branch("else", isSet))),
			want: []path.Path{path.Root("block").AtListIndex(0).AtName("branch").AtListIndex(1).AtName("condition_groups")},
		},
		{
			name: "ids repeated across blocks, branches and nested branches",
			model: alertRuleSetModel("required", types.StringNull(),
				block("page", AlertRuleBranchModel{
					ID:       types.StringValue("page"),
					Branches: []AlertRuleNestedBranchModel{{ID: types.StringValue("nested")}},
				}),
				block("other", branch("nested")),
			),
			want: []path.Path{
				path.Root("block").AtListIndex(0).AtName("branch").AtListIndex(0).AtName("id"),
				path.Root("block").AtListIndex(1).AtName("branch").AtListIndex(0).AtName("id"),
			},
		},
		{
			name: "actions are checked on branches and nested branches",
			model: alertRuleSetModel("required", types.StringNull(),
				block("page", AlertRuleBranchModel{
					ID:      types.StringValue("always"),
					Actions: []AlertRuleActionV3Model{{Type: types.StringValue("escalate")}},
					Branches: []AlertRuleNestedBranchModel{{
						ID: types.StringValue("nested"),
						Actions: []AlertRuleActionV3Model{{
							Type:     types.StringValue("escalate"),
							Escalate: &AlertRuleEscalateActionV3Model{EscalationPaths: v3ExpressionRef("undeclared")},
						}},
					}},
				}),
			),
			want: []path.Path{
				path.Root("block").AtListIndex(0).AtName("branch").AtListIndex(0).AtName("actions").AtListIndex(0).AtName("escalate"),
				path.Root("block").AtListIndex(0).AtName("branch").AtListIndex(0).AtName("branch").AtListIndex(0).
					AtName("actions").AtListIndex(0).AtName("escalate").AtName("escalation_paths").AtName("expression_ref"),
			},
		},
		{
			name: "a duplicate expression name",
			model: func() AlertRuleSetResourceModel {
				m := alertRuleSetModel("required", types.StringNull())
				expression := NamedExpression{
					Name:       types.StringValue("team"),
					StartFrom:  types.StringValue("alert"),
					Operations: []Operation{{First: &EmptyOpts{}}},
				}
				m.NamedExpressions = []NamedExpression{expression, expression}
				return m
			}(),
			want: []path.Path{path.Root("named_expression").AtListIndex(1).AtName("name")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			ValidateAlertRuleSet(tc.model, &diags)

			got := lo.FilterMap(diags.Errors(), func(d diag.Diagnostic, _ int) (path.Path, bool) {
				withPath, ok := d.(diag.DiagnosticWithPath)
				if !ok {
					return path.Empty(), false
				}
				return withPath.Path(), true
			})
			assert.Equal(t, len(tc.want), len(diags.Errors()), "errors: %v", diags)
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}
