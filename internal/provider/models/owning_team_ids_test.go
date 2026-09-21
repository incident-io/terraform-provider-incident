package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestOwningTeamIDsToState(t *testing.T) {
	emptySet := types.SetValueMust(types.StringType, []attr.Value{})
	oneTeam := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("01TEAM")})

	for _, tc := range []struct {
		name    string
		ids     *[]string
		planned types.Set
		want    types.Set
	}{
		{
			name:    "unset stays absent when the API omits the field",
			ids:     nil,
			planned: types.SetNull(types.StringType),
			want:    types.SetNull(types.StringType),
		},
		{
			name:    "an explicit empty set stays empty when the API omits the field",
			ids:     nil,
			planned: emptySet,
			want:    emptySet,
		},
		{
			name:    "an explicit empty set stays empty when the API returns an empty list",
			ids:     lo.ToPtr([]string{}),
			planned: emptySet,
			want:    emptySet,
		},
		{
			name:    "an unknown plan settles as absent when there are no teams",
			ids:     nil,
			planned: types.SetUnknown(types.StringType),
			want:    types.SetNull(types.StringType),
		},
		{
			name:    "teams the API returns win over an absent plan",
			ids:     lo.ToPtr([]string{"01TEAM"}),
			planned: types.SetNull(types.StringType),
			want:    oneTeam,
		},
		{
			name:    "teams the API returns win over an empty plan",
			ids:     lo.ToPtr([]string{"01TEAM"}),
			planned: emptySet,
			want:    oneTeam,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := OwningTeamIDsToState(tc.ids, tc.planned)
			if !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestAlertRouteOwningTeamIDsNullVsEmpty covers issue #595 at the model boundary: the
// API omits owning_team_ids for an unowned route, and the mapping has to answer with
// whatever the practitioner asked for rather than always null.
func TestAlertRouteOwningTeamIDsNullVsEmpty(t *testing.T) {
	apiV2 := client.AlertRouteV2{
		Id:               "01ABC",
		Name:             "route",
		Version:          2,
		AlertSources:     []client.AlertRouteAlertSourceV2{},
		ChannelConfig:    []client.AlertRouteChannelConfigV2{},
		ConditionGroups:  []client.ConditionGroupV2{},
		Expressions:      []client.ExpressionV2{},
		EscalationConfig: client.AlertRouteEscalationConfigV2{EscalationTargets: []client.AlertRouteEscalationTargetV2{}},
		IncidentConfig:   client.AlertRouteIncidentConfigV2{GroupingKeys: []client.GroupingKeyV2{}},
		IncidentTemplate: client.AlertRouteIncidentTemplateV2{},
		// The route is owned by nobody, so the API omits owning_team_ids.
		OwningTeamIds: nil,
	}

	apiV3 := client.AlertRouteV3{
		Id:               "01ABC",
		Name:             "route",
		Version:          3,
		AlertSources:     []client.AlertRouteAlertSourceV3{},
		ConditionGroups:  []client.ConditionGroupV3{},
		Expressions:      []client.ExpressionV3{},
		GroupingConfig:   client.AlertGroupingConfigV3{Default: client.GroupingSettingsV3{}},
		MessageConfig:    client.AlertMessageConfigV3{Destinations: []client.AlertMessageDestinationV3{}},
		EscalationConfig: client.AlertRouteEscalationConfigV3{EscalationTargets: []client.AlertRouteEscalationTargetV3{}},
		IncidentConfig:   client.AlertRouteIncidentConfigV3{},
		OwningTeamIds:    nil,
	}

	planEmpty := &AlertRouteResourceModel{
		OwningTeamIDs: types.SetValueMust(types.StringType, []attr.Value{}),
	}

	// Planned `owning_team_ids = []`: state has to be an empty set, or Terraform fails
	// the apply with "Provider produced inconsistent result after apply".
	for name, got := range map[string]types.Set{
		"v2": AlertRouteResourceModel{}.FromAPIV2WithPlan(apiV2, planEmpty).OwningTeamIDs,
		"v3": AlertRouteResourceModel{}.FromAPIV3WithPlan(apiV3, planEmpty).OwningTeamIDs,
	} {
		if got.IsNull() {
			t.Errorf("%s: owning_team_ids is null, want an empty set", name)
		}
		if elements := got.Elements(); len(elements) != 0 {
			t.Errorf("%s: owning_team_ids has %d elements, want 0", name, len(elements))
		}
	}

	// No plan at all — an import — leaves the attribute absent.
	for name, got := range map[string]types.Set{
		"v2": AlertRouteResourceModel{}.FromAPIV2(apiV2).OwningTeamIDs,
		"v3": AlertRouteResourceModel{}.FromAPIV3(apiV3).OwningTeamIDs,
	} {
		if !got.IsNull() {
			t.Errorf("%s: owning_team_ids is %s, want null", name, got)
		}
	}
}
