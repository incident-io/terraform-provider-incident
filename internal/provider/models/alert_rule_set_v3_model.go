package models

import (
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var alertRuleSetExpressions = ExpressionNamespace{}

type AlertRuleSetResourceModel struct {
	ID                types.String          `tfsdk:"id"`
	UnlockInDashboard types.Bool            `tfsdk:"unlock_in_dashboard"`
	Kind              types.String          `tfsdk:"kind"`
	TeamID            types.String          `tfsdk:"team_id"`
	Version           types.Int64           `tfsdk:"version"`
	Blocks            []AlertRuleBlockModel `tfsdk:"block"`
	NamedExpressions  []NamedExpression     `tfsdk:"named_expression"`
}

type AlertRuleBlockModel struct {
	ID       types.String           `tfsdk:"id"`
	Branches []AlertRuleBranchModel `tfsdk:"branch"`
}

type AlertRuleBranchModel struct {
	ID              types.String                  `tfsdk:"id"`
	ConditionGroups IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	Actions         []AlertRuleActionV3Model      `tfsdk:"actions"`
	Branches        []AlertRuleNestedBranchModel  `tfsdk:"branch"`
}

type AlertRuleNestedBranchModel struct {
	ID              types.String                  `tfsdk:"id"`
	ConditionGroups IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	Actions         []AlertRuleActionV3Model      `tfsdk:"actions"`
}

func AlertRuleSetAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertRuleSetV3", "id"),
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		// The API can't change a set's kind or team: a new kind fails at plan, a new team replaces the set.
		"kind": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: apischema.EnumValuesDescription("AlertRuleSetV3", "kind") + " A set's kind cannot change.",
			Validators: []validator.String{
				stringvalidator.OneOf(apischema.EnumValues("AlertRuleSetV3", "kind")...),
			},
		},
		"team_id": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRuleSetV3", "team_id") + ", and required for them.",
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"version": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertRuleSetV3", "version"),
		},
	}
}

func AlertRuleSetBlocks() map[string]schema.Block {
	return map[string]schema.Block{
		"block": schema.ListNestedBlock{
			MarkdownDescription: apischema.Docstring("AlertRuleSetV3", "blocks"),
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"id": alertRuleIDAttribute("AlertRuleBlockV3"),
				},
				Blocks: map[string]schema.Block{
					"branch": schema.ListNestedBlock{
						MarkdownDescription: apischema.Docstring("AlertRuleBlockV3", "branches"),
						Validators: []validator.List{
							listvalidator.SizeBetween(1, 2),
						},
						NestedObject: schema.NestedBlockObject{
							Attributes: alertRuleBranchAttributes("AlertRuleBranchV3"),
							Blocks: map[string]schema.Block{
								"branch": schema.ListNestedBlock{
									MarkdownDescription: apischema.Docstring("AlertRuleBranchV3", "branches"),
									NestedObject: schema.NestedBlockObject{
										Attributes: alertRuleBranchAttributes("AlertRuleNestedBranchV3"),
									},
								},
							},
						},
					},
				},
			},
		},
		"named_expression": NamedExpressionBlock(),
	}
}

func alertRuleIDAttribute(definition string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: apischema.Docstring(definition, "id"),
		Validators: []validator.String{
			stringvalidator.LengthBetween(1, 128),
		},
	}
}

func alertRuleBranchAttributes(definition string) map[string]schema.Attribute {
	conditions := ConditionsAttribute()
	conditions.Validators = []validator.List{listvalidator.SizeAtLeast(1)}

	return map[string]schema.Attribute{
		"id": alertRuleIDAttribute(definition),
		"condition_groups": schema.ListNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring(definition, "condition_groups"),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"conditions": conditions,
				},
			},
		},
		"actions": AlertRuleActionsAttribute(),
	}
}

func ValidateAlertRuleSet(m AlertRuleSetResourceModel, diags *diag.Diagnostics) {
	if !m.Kind.IsUnknown() && !m.TeamID.IsUnknown() {
		isTeam := m.Kind.ValueString() == string(client.AlertRuleSetV3KindTeam)
		switch {
		case isTeam && m.TeamID.IsNull():
			diags.AddAttributeError(path.Root("team_id"), "Missing team_id",
				"Team rules need the team_id of the team they belong to.")
		case !isTeam && !m.TeamID.IsNull():
			diags.AddAttributeError(path.Root("team_id"), "Unexpected team_id",
				fmt.Sprintf("Only team rules belong to a team. Remove team_id from %s rules.", m.Kind.ValueString()))
		}
	}

	known := KnownExpressionNames(m.NamedExpressions)
	seen := map[string]bool{}
	checkID := func(id types.String, at path.Path) {
		if id.IsNull() || id.IsUnknown() {
			return
		}
		if seen[id.ValueString()] {
			diags.AddAttributeError(at, "Duplicate id", fmt.Sprintf(
				"Another block or branch in this set already uses the id %q. Ids must be unique within a set.",
				id.ValueString()))
		}
		seen[id.ValueString()] = true
	}

	for blockIdx, block := range m.Blocks {
		blockPath := path.Root("block").AtListIndex(blockIdx)
		checkID(block.ID, blockPath.AtName("id"))

		for branchIdx, branch := range block.Branches {
			branchPath := blockPath.AtName("branch").AtListIndex(branchIdx)
			checkID(branch.ID, branchPath.AtName("id"))
			ValidateAlertRuleActions(branch.Actions, branchPath.AtName("actions"), known, diags)

			if branchIdx == 1 && len(branch.ConditionGroups) > 0 {
				diags.AddAttributeError(branchPath.AtName("condition_groups"), "Conditions on an else branch",
					"The second branch of a block is its else, which runs when the first doesn't. Remove its condition_groups.")
			}

			for nestedIdx, nested := range branch.Branches {
				nestedPath := branchPath.AtName("branch").AtListIndex(nestedIdx)
				checkID(nested.ID, nestedPath.AtName("id"))
				ValidateAlertRuleActions(nested.Actions, nestedPath.AtName("actions"), known, diags)
			}
		}
	}

	ValidateExpressions(
		alertRuleSetExpressions, nil, path.Empty(),
		m.NamedExpressions, path.Root("named_expression"), diags)
}

func (m AlertRuleSetResourceModel) ToCreatePayload() (client.AlertRulesCreatePayloadV3, error) {
	expressions, err := m.expressionsToPayload()
	if err != nil {
		return client.AlertRulesCreatePayloadV3{}, err
	}
	blocks, err := alertRuleBlocksToPayload(m.Blocks)
	if err != nil {
		return client.AlertRulesCreatePayloadV3{}, err
	}

	return client.AlertRulesCreatePayloadV3{
		Kind:        client.AlertRulesCreatePayloadV3Kind(m.Kind.ValueString()),
		TeamId:      m.TeamID.ValueStringPointer(),
		Blocks:      blocks,
		Expressions: &expressions,
	}, nil
}

func (m AlertRuleSetResourceModel) ToUpdatePayload(version int64) (client.AlertRulesUpdatePayloadV3, error) {
	expressions, err := m.expressionsToPayload()
	if err != nil {
		return client.AlertRulesUpdatePayloadV3{}, err
	}
	blocks, err := alertRuleBlocksToPayload(m.Blocks)
	if err != nil {
		return client.AlertRulesUpdatePayloadV3{}, err
	}

	return client.AlertRulesUpdatePayloadV3{
		Version:     &version,
		Blocks:      blocks,
		Expressions: &expressions,
	}, nil
}

func (m AlertRuleSetResourceModel) expressionsToPayload() ([]client.ExpressionPayloadV3, error) {
	expressions, _, err := ExpressionsToPayload(alertRuleSetExpressions, nil, m.NamedExpressions)

	return expressions, err
}

func alertRuleBlocksToPayload(blocks []AlertRuleBlockModel) ([]client.AlertRuleBlockV3, error) {
	var errs []error
	actionsToPayload := func(branchID types.String, actions []AlertRuleActionV3Model) []client.AlertRuleActionV3 {
		payload, err := AlertRuleActionsV3ToPayload(actions)
		if err != nil {
			errs = append(errs, fmt.Errorf("branch %s: %w", branchID.ValueString(), err))
		}
		return payload
	}

	payload := lo.Map(blocks, func(block AlertRuleBlockModel, _ int) client.AlertRuleBlockV3 {
		return client.AlertRuleBlockV3{
			Id: block.ID.ValueString(),
			Branches: lo.Map(block.Branches, func(branch AlertRuleBranchModel, _ int) client.AlertRuleBranchV3 {
				payload := client.AlertRuleBranchV3{
					Id:              branch.ID.ValueString(),
					ConditionGroups: ConditionGroupsToV3Payload(branch.ConditionGroups),
					Actions:         actionsToPayload(branch.ID, branch.Actions),
				}
				// The API omits an empty list here, so send it the same way.
				if len(branch.Branches) > 0 {
					payload.Branches = lo.ToPtr(lo.Map(branch.Branches,
						func(nested AlertRuleNestedBranchModel, _ int) client.AlertRuleNestedBranchV3 {
							return client.AlertRuleNestedBranchV3{
								Id:              nested.ID.ValueString(),
								ConditionGroups: ConditionGroupsToV3Payload(nested.ConditionGroups),
								Actions:         actionsToPayload(nested.ID, nested.Actions),
							}
						}))
				}

				return payload
			}),
		}
	})

	return payload, errors.Join(errs...)
}

// FromAPI takes the plan on create and update, the prior state on read, and nil on import.
func (AlertRuleSetResourceModel) FromAPI(set client.AlertRuleSetV3, prior *AlertRuleSetResourceModel) AlertRuleSetResourceModel {
	priorConditionGroups := map[string]IncidentEngineConditionGroups{}
	priorActions := map[string][]AlertRuleActionV3Model{}
	var priorNamed []NamedExpression
	result := AlertRuleSetResourceModel{UnlockInDashboard: types.BoolNull()}
	if prior != nil {
		result.UnlockInDashboard = prior.UnlockInDashboard
		priorNamed = prior.NamedExpressions
		for _, block := range prior.Blocks {
			for _, branch := range block.Branches {
				priorConditionGroups[branch.ID.ValueString()] = branch.ConditionGroups
				priorActions[branch.ID.ValueString()] = branch.Actions
				for _, nested := range branch.Branches {
					priorConditionGroups[nested.ID.ValueString()] = nested.ConditionGroups
					priorActions[nested.ID.ValueString()] = nested.Actions
				}
			}
		}
	}
	// Keep condition_groups as the config wrote it: the API answers both omitted and [] with [].
	// Where there is no prior spelling to keep, as on import, read bindings in the shorthand.
	conditionGroups := func(id string, groups []client.ConditionGroupPayloadV3) IncidentEngineConditionGroups {
		prior := priorConditionGroups[id]
		if len(groups) == 0 {
			if prior != nil {
				return IncidentEngineConditionGroups{}
			}
			return nil
		}
		out := ConditionGroupsFromV3Payload(groups)
		out.ShortestSpelling()
		out.ReconcileSpelling(prior)

		return out
	}

	result.ID = types.StringValue(set.Id)
	result.Kind = types.StringValue(string(set.Kind))
	result.TeamID = types.StringPointerValue(set.TeamId)
	result.Version = types.Int64Value(set.Version)

	result.Blocks = lo.Map(set.Blocks, func(block client.AlertRuleBlockV3, _ int) AlertRuleBlockModel {
		return AlertRuleBlockModel{
			ID: types.StringValue(block.Id),
			Branches: lo.Map(block.Branches, func(branch client.AlertRuleBranchV3, _ int) AlertRuleBranchModel {
				return AlertRuleBranchModel{
					ID:              types.StringValue(branch.Id),
					ConditionGroups: conditionGroups(branch.Id, branch.ConditionGroups),
					Actions:         AlertRuleActionsV3FromAPI(branch.Actions, priorActions[branch.Id]),
					Branches: lo.Map(lo.FromPtr(branch.Branches),
						func(nested client.AlertRuleNestedBranchV3, _ int) AlertRuleNestedBranchModel {
							return AlertRuleNestedBranchModel{
								ID:              types.StringValue(nested.Id),
								ConditionGroups: conditionGroups(nested.Id, nested.ConditionGroups),
								Actions:         AlertRuleActionsV3FromAPI(nested.Actions, priorActions[nested.Id]),
							}
						}),
				}
			}),
		}
	})

	_, result.NamedExpressions = ExpressionsFromPayload(set.Expressions, alertRuleSetExpressions, nil, priorNamed)
	if result.NamedExpressions == nil {
		result.NamedExpressions = []NamedExpression{}
	}

	return result
}
