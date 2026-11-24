package models

import (
	"context"
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

func groupingResourceModel(kind string, teamID types.String, def *TeamGroupingPreferenceSettingsModel, branches ...GroupingBranchModel) AlertGroupingPreferenceResourceModel {
	return AlertGroupingPreferenceResourceModel{
		ID:                      types.StringUnknown(),
		UnlockInDashboard:       types.BoolNull(),
		Kind:                    types.StringValue(kind),
		TeamID:                  teamID,
		Version:                 types.Int64Unknown(),
		GroupingPreferenceModel: groupingPreferenceModel(def, branches...),
	}
}

func groupingBranch(id types.String, settings *TeamGroupingPreferenceSettingsModel) GroupingBranchModel {
	return GroupingBranchModel{
		ID:              id,
		ConditionGroups: IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")},
		Settings:        settings,
	}
}

// The resource embeds GroupingPreferenceModel, so the framework has to read and write its
// fields as the resource's own.
func TestAlertGroupingPreferenceResourceModelFitsItsSchema(t *testing.T) {
	ctx := context.Background()
	attributes := map[string]schema.Attribute{
		"id":                  schema.StringAttribute{Computed: true},
		"unlock_in_dashboard": schema.BoolAttribute{Optional: true},
		"kind":                schema.StringAttribute{Required: true},
		"team_id":             schema.StringAttribute{Optional: true},
		"version":             schema.Int64Attribute{Computed: true},
		"default":             GroupingDefaultAttribute("The default."),
	}
	s := schema.Schema{Attributes: attributes, Blocks: GroupingPreferenceBlocks()}

	want := AlertGroupingPreferenceResourceModel{}.FromAPI(client.AlertGroupingPreferenceV3{
		Id:      "01PREFERENCE",
		Kind:    client.AlertGroupingPreferenceV3KindTeam,
		TeamId:  lo.ToPtr("01TEAM"),
		Version: 4,
		Default: client.AlertGroupingDefaultBranchV3{Settings: client.AlertGroupingSettingsV3{Enabled: false}},
		Branches: []client.AlertGroupingConditionalBranchV3{{
			Id:              lo.ToPtr("01BRANCH"),
			ConditionGroups: ConditionGroupsToV3Payload(IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")}),
		}},
	}, nil)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	require.False(t, state.Set(ctx, &want).HasError())
	var got AlertGroupingPreferenceResourceModel
	require.False(t, state.Get(ctx, &got).HasError())
	assert.Equal(t, want, got)
}

func TestAlertGroupingPreferenceResourceModelFromAPI(t *testing.T) {
	off := client.AlertGroupingDefaultBranchV3{Settings: client.AlertGroupingSettingsV3{Enabled: false, AiEnabled: lo.ToPtr(false)}}
	prior := groupingResourceModel(GroupingKindTeam, types.StringValue("01TEAM"), groupingOff())
	prior.UnlockInDashboard = types.BoolValue(true)

	team := AlertGroupingPreferenceResourceModel{}.FromAPI(client.AlertGroupingPreferenceV3{
		Id: "01TEAMPREF", Kind: client.AlertGroupingPreferenceV3KindTeam, TeamId: lo.ToPtr("01TEAM"), Version: 2, Default: off,
	}, &prior)
	assert.Equal(t, types.StringValue(GroupingKindTeam), team.Kind)
	assert.Equal(t, types.StringValue("01TEAM"), team.TeamID)
	assert.Equal(t, types.Int64Value(2), team.Version)
	assert.Equal(t, types.BoolValue(true), team.UnlockInDashboard, "no read returns it, so it comes from the prior")

	def := AlertGroupingPreferenceResourceModel{}.FromAPI(client.AlertGroupingPreferenceV3{
		Id: "01DEFAULT", Kind: client.AlertGroupingPreferenceV3KindDefault, Version: 3, Default: off,
	}, nil)
	assert.Equal(t, types.StringValue(GroupingKindDefault), def.Kind)
	assert.Equal(t, types.StringNull(), def.TeamID)
	assert.Equal(t, types.StringValue("01DEFAULT"), def.ID)
	assert.Equal(t, types.BoolNull(), def.UnlockInDashboard)
	assert.NotNil(t, def.Branches, "an import reads no branches as an empty list, as the config's empty block is")
}

// The API accepts empty grouping_keys while grouping is off and answers with none, so the
// read keeps the config's empty set rather than report a change after apply.
func TestGroupingPreferenceKeepsEmptyKeysWhileGroupingIsOff(t *testing.T) {
	plan := groupingOff()
	plan.GroupingKeys = []AlertRouteGroupingKey{}

	got := groupingSettingsFromAPI(client.AlertGroupingSettingsV3{Enabled: false, AiEnabled: lo.ToPtr(false)}, plan)
	assert.Equal(t, plan, got)
}

func TestValidateGroupingPreference(t *testing.T) {
	disabledWith := func(edit func(*TeamGroupingPreferenceSettingsModel)) *TeamGroupingPreferenceSettingsModel {
		settings := groupingOff()
		edit(settings)
		return settings
	}
	enabledWithout := func(edit func(*TeamGroupingPreferenceSettingsModel)) *TeamGroupingPreferenceSettingsModel {
		settings := groupingOn(600)
		edit(settings)
		return settings
	}
	team := types.StringValue("01TEAM")

	for _, tc := range []struct {
		name   string
		model  AlertGroupingPreferenceResourceModel
		errors []path.Path
	}{
		{
			name:  "a team's preference with branches",
			model: groupingResourceModel(GroupingKindTeam, team, groupingOn(1800), groupingBranch(types.StringUnknown(), groupingOff())),
		},
		{
			name:  "the organisation default",
			model: groupingResourceModel(GroupingKindDefault, types.StringNull(), groupingOff()),
		},
		{
			name:  "a team the plan doesn't know yet",
			model: groupingResourceModel(GroupingKindTeam, types.StringUnknown(), groupingOff()),
		},
		{
			name:   "a team's preference without its team",
			model:  groupingResourceModel(GroupingKindTeam, types.StringNull(), groupingOff()),
			errors: []path.Path{path.Root("team_id")},
		},
		{
			name:   "the organisation default with a team",
			model:  groupingResourceModel(GroupingKindDefault, team, groupingOff()),
			errors: []path.Path{path.Root("team_id")},
		},
		{
			name: "enabled settings need their window, on the default and on a branch",
			model: groupingResourceModel(GroupingKindTeam, team,
				enabledWithout(func(s *TeamGroupingPreferenceSettingsModel) { s.WindowSeconds = types.Int64Null() }),
				groupingBranch(types.StringUnknown(), enabledWithout(func(s *TeamGroupingPreferenceSettingsModel) { s.WindowType = types.StringNull() }))),
			errors: []path.Path{
				path.Root("default").AtName("settings").AtName("window_seconds"),
				path.Root("branch").AtListIndex(0).AtName("settings").AtName("window_type"),
			},
		},
		{
			name: "an unknown window may still be set",
			model: groupingResourceModel(GroupingKindTeam, team,
				enabledWithout(func(s *TeamGroupingPreferenceSettingsModel) { s.WindowSeconds = types.Int64Unknown() })),
		},
		{
			name: "disabled settings carry no window, keys or AI grouping",
			model: groupingResourceModel(GroupingKindTeam, team, groupingOff(),
				groupingBranch(types.StringUnknown(), disabledWith(func(s *TeamGroupingPreferenceSettingsModel) {
					s.WindowSeconds = types.Int64Value(60)
					s.WindowType = types.StringValue("fixed")
					s.GroupingKeys = []AlertRouteGroupingKey{{Reference: types.StringValue("alert.title")}}
					s.AIEnabled = types.BoolValue(true)
				}))),
			errors: []path.Path{
				path.Root("branch").AtListIndex(0).AtName("settings").AtName("window_seconds"),
				path.Root("branch").AtListIndex(0).AtName("settings").AtName("window_type"),
				path.Root("branch").AtListIndex(0).AtName("settings").AtName("grouping_keys"),
				path.Root("branch").AtListIndex(0).AtName("settings").AtName("ai_enabled"),
			},
		},
		{
			// The API only refuses keys that are there.
			name: "disabled settings may have empty keys",
			model: groupingResourceModel(GroupingKindTeam, team,
				disabledWith(func(s *TeamGroupingPreferenceSettingsModel) { s.GroupingKeys = []AlertRouteGroupingKey{} })),
		},
		{
			name: "two branches with one ID",
			model: groupingResourceModel(GroupingKindTeam, team, groupingOff(),
				groupingBranch(types.StringValue("01JBRANCHAAAAAAAAAAAAAAAAA"), groupingOff()),
				groupingBranch(types.StringUnknown(), groupingOff()),
				groupingBranch(types.StringValue("01JBRANCHAAAAAAAAAAAAAAAAA"), groupingOff())),
			errors: []path.Path{path.Root("branch").AtListIndex(2).AtName("id")},
		},
		{
			name: "two expressions with one name",
			model: func() AlertGroupingPreferenceResourceModel {
				m := groupingResourceModel(GroupingKindDefault, types.StringNull(), groupingOff())
				expression := NamedExpression{
					Name:       types.StringValue("team"),
					StartFrom:  types.StringValue("alert.attributes.team"),
					Operations: []Operation{{First: &EmptyOpts{}}},
				}
				m.NamedExpressions = []NamedExpression{expression, expression}
				return m
			}(),
			errors: []path.Path{path.Root("named_expression").AtListIndex(1).AtName("name")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			ValidateGroupingPreference(tc.model, &diags)

			got := lo.Map(diags.Errors(), func(d diag.Diagnostic, _ int) path.Path {
				withPath, ok := d.(diag.DiagnosticWithPath)
				require.True(t, ok, "%s has no path", d.Summary())
				return withPath.Path()
			})
			assert.ElementsMatch(t, tc.errors, got)
		})
	}
}
