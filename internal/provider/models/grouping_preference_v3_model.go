package models

import (
	"encoding/json"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// GroupingPreferenceModel is what a grouping preference holds. A team's preference and the
// organisation default share it, so a resource for either embeds it next to its own fields.
type GroupingPreferenceModel struct {
	Default          *TeamGroupingPreferenceBranchModel `tfsdk:"default"`
	Branches         []GroupingBranchModel              `tfsdk:"branch"`
	NamedExpressions []NamedExpression                  `tfsdk:"named_expression"`
}

// GroupingBranchModel groups the alerts its conditions match. Alert rules use the first
// branch that matches, else the default.
type GroupingBranchModel struct {
	ID              types.String                         `tfsdk:"id"`
	ConditionGroups IncidentEngineConditionGroups        `tfsdk:"condition_groups"`
	Settings        *TeamGroupingPreferenceSettingsModel `tfsdk:"settings"`
}

var ulidPattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

func GroupingPreferenceBlocks() map[string]schema.Block {
	// The API refuses a branch that tests nothing: the default is for alerts no branch matches.
	conditionGroups := branchConditionGroupsAttribute("TeamGroupingConditionalBranchV3")
	conditionGroups.Optional, conditionGroups.Required = false, true
	conditionGroups.Validators = []validator.List{listvalidator.SizeAtLeast(1)}

	return map[string]schema.Block{
		"branch": schema.ListNestedBlock{
			MarkdownDescription: apischema.Docstring("TeamGroupingPreferenceV3", "branches"),
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					// No UseStateForUnknown: it copies the ID at the same index, so inserting or
					// removing a branch would hand its alert groups to its neighbour. The
					// resource carries IDs over with CarryBranchIDs instead.
					"id": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: apischema.Docstring("TeamGroupingConditionalBranchV3", "id"),
						Validators: []validator.String{
							stringvalidator.RegexMatches(ulidPattern, "must be a ULID"),
						},
					},
					"condition_groups": conditionGroups,
					"settings":         groupingSettingsAttribute(),
				},
			},
		},
		"named_expression": NamedExpressionBlock(),
	}
}

func (m GroupingPreferenceModel) ToTeamCreatePayload(teamID string) (client.TeamGroupingPreferencesCreatePayloadV3, error) {
	branches, expressions, err := m.branchesAndExpressionsToPayload()

	return client.TeamGroupingPreferencesCreatePayloadV3{
		TeamId:      teamID,
		Default:     m.Default.toPayload(),
		Branches:    &branches,
		Expressions: &expressions,
	}, err
}

func (m GroupingPreferenceModel) ToTeamUpdatePayload(version int64) (client.TeamGroupingPreferencesUpdatePayloadV3, error) {
	branches, expressions, err := m.branchesAndExpressionsToPayload()

	return client.TeamGroupingPreferencesUpdatePayloadV3{
		Version:     version,
		Default:     m.Default.toPayload(),
		Branches:    &branches,
		Expressions: &expressions,
	}, err
}

func (m GroupingPreferenceModel) ToDefaultCreatePayload() (client.DefaultGroupingPreferencesCreatePayloadV3, error) {
	branches, expressions, err := m.branchesAndExpressionsToPayload()

	return client.DefaultGroupingPreferencesCreatePayloadV3{
		Default:     m.Default.toPayload(),
		Branches:    &branches,
		Expressions: &expressions,
	}, err
}

func (m GroupingPreferenceModel) ToDefaultUpdatePayload(version int64) (client.DefaultGroupingPreferencesUpdatePayloadV3, error) {
	branches, expressions, err := m.branchesAndExpressionsToPayload()

	return client.DefaultGroupingPreferencesUpdatePayloadV3{
		Version:     &version,
		Default:     m.Default.toPayload(),
		Branches:    &branches,
		Expressions: &expressions,
	}, err
}

// branchesAndExpressionsToPayload always returns both lists, empty rather than nil: an
// update that omits them keeps what the dashboard last wrote, and Terraform owns them.
func (m GroupingPreferenceModel) branchesAndExpressionsToPayload() ([]client.TeamGroupingConditionalBranchV3, []client.ExpressionPayloadV3, error) {
	branches := lo.Map(m.Branches, func(branch GroupingBranchModel, _ int) client.TeamGroupingConditionalBranchV3 {
		payload := client.TeamGroupingConditionalBranchV3{
			ConditionGroups: ConditionGroupsToV3Payload(branch.ConditionGroups),
			Settings:        branch.Settings.toPayload(),
		}
		// A new branch has no ID until the server assigns one.
		if !branch.ID.IsNull() && !branch.ID.IsUnknown() {
			payload.Id = branch.ID.ValueStringPointer()
		}

		return payload
	})
	expressions, _, err := ExpressionsToPayload(ExpressionNamespace{}, nil, m.NamedExpressions)

	return branches, expressions, err
}

// FromAPI takes the parts the team and default preferences share. prior is the plan on
// create and update, the prior state on read, and nil on import.
//
// Branches are matched to the prior by position, as the API keeps their order.
func (GroupingPreferenceModel) FromAPI(
	def client.TeamGroupingBranchV3,
	branches []client.TeamGroupingConditionalBranchV3,
	expressions []client.ExpressionPayloadV3,
	prior *GroupingPreferenceModel,
) GroupingPreferenceModel {
	if prior == nil {
		prior = &GroupingPreferenceModel{}
	}

	result := GroupingPreferenceModel{
		Default: groupingDefaultFromAPI(def, prior.Default),
		Branches: lo.Map(branches, func(branch client.TeamGroupingConditionalBranchV3, idx int) GroupingBranchModel {
			priorBranch, _ := lo.Nth(prior.Branches, idx)
			conditionGroups := ConditionGroupsFromV3Payload(branch.ConditionGroups)
			conditionGroups.ReconcileSpelling(priorBranch.ConditionGroups)

			return GroupingBranchModel{
				ID:              types.StringPointerValue(branch.Id),
				ConditionGroups: conditionGroups,
				Settings:        groupingSettingsFromAPI(branch.Settings, priorBranch.Settings),
			}
		}),
	}

	_, result.NamedExpressions = ExpressionsFromPayload(expressions, ExpressionNamespace{}, nil, prior.NamedExpressions)
	if result.NamedExpressions == nil {
		result.NamedExpressions = []NamedExpression{}
	}

	return result
}

// CarryBranchIDs copies each state branch's ID onto the planned branch it will become, as
// the API keys alert groups on it. A branch with the same conditions matches wherever it
// moved to. One whose conditions changed keeps the ID at its index, if no other branch
// took it. Anything unmatched stays unknown, and the API assigns it an ID.
func CarryBranchIDs(plan, state []GroupingBranchModel) {
	used := map[int]bool{}
	for i := range plan {
		if plan[i].ID.IsUnknown() {
			continue
		}
		for j := range state {
			if state[j].ID.Equal(plan[i].ID) {
				used[j] = true
			}
		}
	}

	conditions := func(branch GroupingBranchModel) string {
		return string(lo.Must(json.Marshal(ConditionGroupsToV3Payload(branch.ConditionGroups))))
	}
	for i := range plan {
		if !plan[i].ID.IsUnknown() {
			continue
		}
		for j := range state {
			if !used[j] && conditions(state[j]) == conditions(plan[i]) {
				plan[i].ID, used[j] = state[j].ID, true
				break
			}
		}
	}

	for i := range plan {
		if plan[i].ID.IsUnknown() && i < len(state) && !used[i] {
			plan[i].ID, used[i] = state[i].ID, true
		}
	}
}
