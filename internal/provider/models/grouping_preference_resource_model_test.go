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

func groupingResourceModel(scope string, teamID types.String, def *TeamGroupingPreferenceSettingsModel, branches ...GroupingBranchModel) GroupingPreferenceResourceModel {
	return GroupingPreferenceResourceModel{
		ID:                      types.StringUnknown(),
		UnlockInDashboard:       types.BoolNull(),
		Scope:                   types.StringValue(scope),
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
func TestGroupingPreferenceResourceModelFitsItsSchema(t *testing.T) {
	ctx := context.Background()
	attributes := map[string]schema.Attribute{
		"id":                  schema.StringAttribute{Computed: true},
		"unlock_in_dashboard": schema.BoolAttribute{Optional: true},
		"scope":               schema.StringAttribute{Required: true},
		"team_id":             schema.StringAttribute{Optional: true},
		"version":             schema.Int64Attribute{Computed: true},
		"default":             GroupingDefaultAttribute("The default."),
	}
	s := schema.Schema{Attributes: attributes, Blocks: GroupingPreferenceBlocks()}

	want := GroupingPreferenceResourceModel{}.FromTeamAPI(client.TeamGroupingPreferenceV3{
		Id:      "01PREFERENCE",
		TeamId:  "01TEAM",
		Version: 4,
		Default: client.TeamGroupingBranchV3{Settings: client.TeamGroupingSettingsV3{Enabled: false}},
		Branches: []client.TeamGroupingConditionalBranchV3{{
			Id:              lo.ToPtr("01BRANCH"),
			ConditionGroups: ConditionGroupsToV3Payload(IncidentEngineConditionGroups{conditionGroup("alert.title", "is_set")}),
		}},
	}, nil)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	require.False(t, state.Set(ctx, &want).HasError())
	var got GroupingPreferenceResourceModel
	require.False(t, state.Get(ctx, &got).HasError())
	assert.Equal(t, want, got)
}

func TestGroupingPreferenceResourceModelFromAPI(t *testing.T) {
	off := client.TeamGroupingBranchV3{Settings: client.TeamGroupingSettingsV3{Enabled: false, AiEnabled: lo.ToPtr(false)}}
	prior := groupingResourceModel(GroupingScopeTeam, types.StringValue("01TEAM"), groupingOff())
	prior.UnlockInDashboard = types.BoolValue(true)

	team := GroupingPreferenceResourceModel{}.FromTeamAPI(client.TeamGroupingPreferenceV3{
		Id: "01TEAMPREF", TeamId: "01TEAM", Version: 2, Default: off,
	}, &prior)
	assert.Equal(t, types.StringValue(GroupingScopeTeam), team.Scope)
	assert.Equal(t, types.StringValue("01TEAM"), team.TeamID)
	assert.Equal(t, types.Int64Value(2), team.Version)
	assert.Equal(t, types.BoolValue(true), team.UnlockInDashboard, "no read returns it, so it comes from the prior")

	def := GroupingPreferenceResourceModel{}.FromDefaultAPI(client.DefaultGroupingPreferenceV3{
		Id: "01DEFAULT", Version: 3, Default: off,
	}, nil)
	assert.Equal(t, types.StringValue(GroupingScopeOrganisation), def.Scope)
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

	got := groupingSettingsFromAPI(client.TeamGroupingSettingsV3{Enabled: false, AiEnabled: lo.ToPtr(false)}, plan)
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
		model  GroupingPreferenceResourceModel
		errors []path.Path
	}{
		{
			name:  "a team's preference with branches",
			model: groupingResourceModel(GroupingScopeTeam, team, groupingOn(1800), groupingBranch(types.StringUnknown(), groupingOff())),
		},
		{
			name:  "the organisation default",
			model: groupingResourceModel(GroupingScopeOrganisation, types.StringNull(), groupingOff()),
		},
		{
			name:  "a team the plan doesn't know yet",
			model: groupingResourceModel(GroupingScopeTeam, types.StringUnknown(), groupingOff()),
		},
		{
			name:   "a team's preference without its team",
			model:  groupingResourceModel(GroupingScopeTeam, types.StringNull(), groupingOff()),
			errors: []path.Path{path.Root("team_id")},
		},
		{
			name:   "the organisation default with a team",
			model:  groupingResourceModel(GroupingScopeOrganisation, team, groupingOff()),
			errors: []path.Path{path.Root("team_id")},
		},
		{
			name: "enabled settings need their window, on the default and on a branch",
			model: groupingResourceModel(GroupingScopeTeam, team,
				enabledWithout(func(s *TeamGroupingPreferenceSettingsModel) { s.WindowSeconds = types.Int64Null() }),
				groupingBranch(types.StringUnknown(), enabledWithout(func(s *TeamGroupingPreferenceSettingsModel) { s.WindowType = types.StringNull() }))),
			errors: []path.Path{
				path.Root("default").AtName("settings").AtName("window_seconds"),
				path.Root("branch").AtListIndex(0).AtName("settings").AtName("window_type"),
			},
		},
		{
			name: "an unknown window may still be set",
			model: groupingResourceModel(GroupingScopeTeam, team,
				enabledWithout(func(s *TeamGroupingPreferenceSettingsModel) { s.WindowSeconds = types.Int64Unknown() })),
		},
		{
			name: "disabled settings carry no window, keys or AI grouping",
			model: groupingResourceModel(GroupingScopeTeam, team, groupingOff(),
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
			model: groupingResourceModel(GroupingScopeTeam, team,
				disabledWith(func(s *TeamGroupingPreferenceSettingsModel) { s.GroupingKeys = []AlertRouteGroupingKey{} })),
		},
		{
			name: "two branches with one ID",
			model: groupingResourceModel(GroupingScopeTeam, team, groupingOff(),
				groupingBranch(types.StringValue("01JBRANCHAAAAAAAAAAAAAAAAA"), groupingOff()),
				groupingBranch(types.StringUnknown(), groupingOff()),
				groupingBranch(types.StringValue("01JBRANCHAAAAAAAAAAAAAAAAA"), groupingOff())),
			errors: []path.Path{path.Root("branch").AtListIndex(2).AtName("id")},
		},
		{
			name: "two expressions with one name",
			model: func() GroupingPreferenceResourceModel {
				m := groupingResourceModel(GroupingScopeOrganisation, types.StringNull(), groupingOff())
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
