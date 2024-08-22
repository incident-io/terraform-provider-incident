package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// buildModel converts from the response type to the terraform model/schema type.
func (r *IncidentWorkflowResource) buildModel(workflow client.Workflow) *IncidentWorkflowResourceModel {
	model := &IncidentWorkflowResourceModel{
		ID:                      types.StringValue(workflow.Id),
		Name:                    types.StringValue(workflow.Name),
		Trigger:                 types.StringValue(workflow.Trigger.Name),
		ConditionGroups:         r.buildConditionGroupsFromV5(workflow.ConditionGroups),
		Steps:                   r.buildSteps(workflow.Steps),
		Expressions:             r.buildExpressions(workflow.Expressions),
		OnceFor:                 r.buildOnceFor(workflow.OnceFor),
		RunsOnIncidentModes:     r.buildRunsOnIncidentModes(workflow.RunsOnIncidentModes),
		IncludePrivateIncidents: types.BoolValue(workflow.IncludePrivateIncidents),
		ContinueOnStepError:     types.BoolValue(workflow.ContinueOnStepError),
		RunsOnIncidents:         types.StringValue(string(workflow.RunsOnIncidents)),
		State:                   types.StringValue(string(workflow.State)),
	}
	if workflow.Folder != nil {
		model.Folder = types.StringValue(*workflow.Folder)
	}
	if workflow.Delay != nil {
		model.Delay = &IncidentWorkflowDelay{
			ConditionsApplyOverDelay: types.BoolValue(workflow.Delay.ConditionsApplyOverDelay),
			ForSeconds:               types.Int64Value(workflow.Delay.ForSeconds),
		}
	}
	return model
}

func (r *IncidentWorkflowResource) buildOnceFor(onceFor []client.EngineReferenceV2) []basetypes.StringValue {
	out := []basetypes.StringValue{}

	for _, ref := range onceFor {
		out = append(out, types.StringValue(ref.Key))
	}

	return out
}

func (r *IncidentWorkflowResource) buildRunsOnIncidentModes(modes []client.WorkflowRunsOnIncidentModes) []basetypes.StringValue {
	out := []basetypes.StringValue{}

	for _, mode := range modes {
		out = append(out, types.StringValue(string(mode)))
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditionGroupsFromV3(groups []client.ConditionGroupV3) IncidentEngineConditionGroups {
	var out IncidentEngineConditionGroups

	for _, g := range groups {
		out = append(out, IncidentEngineConditionGroup{
			Conditions: r.buildConditionsFromV3(g.Conditions),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditionGroupsFromV4(groups []client.ConditionGroupV4) IncidentEngineConditionGroups {
	var out IncidentEngineConditionGroups

	for _, g := range groups {
		out = append(out, IncidentEngineConditionGroup{
			Conditions: r.buildConditionsFromV4(g.Conditions),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditionGroupsFromV5(groups []client.ConditionGroupV5) IncidentEngineConditionGroups {
	var out IncidentEngineConditionGroups

	for _, g := range groups {
		out = append(out, IncidentEngineConditionGroup{
			Conditions: r.buildConditionsFromV5(g.Conditions),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditionsFromV3(conditions []client.ConditionV3) []IncidentEngineCondition {
	out := []IncidentEngineCondition{}

	for _, c := range conditions {
		out = append(out, IncidentEngineCondition{
			Subject:       types.StringValue(c.Subject.Reference),
			Operation:     types.StringValue(c.Operation.Value),
			ParamBindings: r.buildParamBindingsFromV4(c.ParamBindings),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditionsFromV4(conditions []client.ConditionV4) []IncidentEngineCondition {
	out := []IncidentEngineCondition{}

	for _, c := range conditions {
		out = append(out, IncidentEngineCondition{
			Subject:       types.StringValue(c.Subject.Reference),
			Operation:     types.StringValue(c.Operation.Value),
			ParamBindings: r.buildParamBindingsFromV5(c.ParamBindings),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditionsFromV5(conditions []client.ConditionV5) []IncidentEngineCondition {
	out := []IncidentEngineCondition{}

	for _, c := range conditions {
		out = append(out, IncidentEngineCondition{
			Subject:       types.StringValue(c.Subject.Reference),
			Operation:     types.StringValue(c.Operation.Value),
			ParamBindings: r.buildParamBindingsFromV7(c.ParamBindings),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildSteps(steps []client.StepConfig) []IncidentWorkflowStep {
	out := []IncidentWorkflowStep{}

	for _, s := range steps {
		out = append(out, IncidentWorkflowStep{
			ForEach:       types.StringPointerValue(s.ForEach),
			ID:            types.StringValue(s.Id),
			Name:          types.StringValue(s.Name),
			ParamBindings: r.buildParamBindingsFromV2(s.ParamBindings),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBindingsFromV2(pbs []client.EngineParamBindingV2) []IncidentEngineParamBinding {
	out := []IncidentEngineParamBinding{}

	for _, pb := range pbs {
		out = append(out, r.buildParamBindingFromV2(pb))
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBindingsFromV4(pbs []client.EngineParamBindingV4) []IncidentEngineParamBinding {
	out := []IncidentEngineParamBinding{}

	for _, pb := range pbs {
		out = append(out, r.buildParamBindingFromV4(pb))
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBindingsFromV5(pbs []client.EngineParamBindingV5) []IncidentEngineParamBinding {
	out := []IncidentEngineParamBinding{}

	for _, pb := range pbs {
		out = append(out, r.buildParamBindingFromV5(pb))
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBindingsFromV7(pbs []client.EngineParamBindingV7) []IncidentEngineParamBinding {
	out := []IncidentEngineParamBinding{}

	for _, pb := range pbs {
		out = append(out, r.buildParamBindingFromV7(pb))
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBindingFromV7(pb client.EngineParamBindingV7) IncidentEngineParamBinding {
	return IncidentEngineParamBinding{}.FromClientV7(pb)
}

func (r *IncidentWorkflowResource) buildParamBindingFromV6(pb client.EngineParamBindingV6) IncidentEngineParamBinding {
	return IncidentEngineParamBinding{}.FromClientV6(pb)
}

func (r *IncidentWorkflowResource) buildParamBindingFromV5(pb client.EngineParamBindingV5) IncidentEngineParamBinding {
	return IncidentEngineParamBinding{}.FromClientV5(pb)
}

func (r *IncidentWorkflowResource) buildParamBindingFromV4(pb client.EngineParamBindingV4) IncidentEngineParamBinding {
	return IncidentEngineParamBinding{}.FromClientV4(pb)
}

func (r *IncidentWorkflowResource) buildParamBindingFromV3(pb client.EngineParamBindingV3) IncidentEngineParamBinding {
	return IncidentEngineParamBinding{}.FromClientV3(pb)
}

func (r *IncidentWorkflowResource) buildParamBindingFromV2(pb client.EngineParamBindingV2) IncidentEngineParamBinding {
	return IncidentEngineParamBinding{}.FromClientV2(pb)
}

func (r *IncidentWorkflowResource) buildExpressions(expressions []client.ExpressionV3) IncidentEngineExpressions {
	out := IncidentEngineExpressions{}

	for _, e := range expressions {
		expression := IncidentEngineExpression{
			Label:         types.StringValue(e.Label),
			Operations:    r.buildOperations(e.Operations),
			Reference:     types.StringValue(e.Reference),
			RootReference: types.StringValue(e.RootReference),
		}
		if e.ElseBranch != nil {
			expression.ElseBranch = &IncidentEngineElseBranch{
				Result: r.buildParamBindingFromV3(e.ElseBranch.Result),
			}
		}
		out = append(out, expression)
	}

	return out
}

func (r *IncidentWorkflowResource) buildOperations(operations []client.ExpressionOperationV3) []IncidentEngineExpressionOperation {
	out := []IncidentEngineExpressionOperation{}

	for _, o := range operations {
		operation := IncidentEngineExpressionOperation{
			OperationType: types.StringValue(string(o.OperationType)),
		}
		if o.Branches != nil {
			operation.Branches = &IncidentEngineExpressionBranchesOpts{
				Branches: r.buildBranches(o.Branches.Branches),
				Returns:  r.buildReturns(o.Branches.Returns),
			}
		}
		if o.Filter != nil {
			operation.Filter = &IncidentEngineExpressionFilterOpts{
				ConditionGroups: r.buildConditionGroupsFromV3(o.Filter.ConditionGroups),
			}
		}
		if o.Navigate != nil {
			operation.Navigate = &IncidentEngineExpressionNavigateOpts{
				Reference: types.StringValue(o.Navigate.Reference),
			}
		}
		if o.Parse != nil {
			operation.Parse = &IncidentEngineExpressionParseOpts{
				Returns: r.buildReturns(o.Parse.Returns),
				Source:  types.StringValue(o.Parse.Source),
			}
		}
		out = append(out, operation)
	}

	return out
}

func (r *IncidentWorkflowResource) buildBranches(branches []client.ExpressionBranchV3) []IncidentEngineBranch {
	out := []IncidentEngineBranch{}

	for _, b := range branches {
		out = append(out, IncidentEngineBranch{
			ConditionGroups: r.buildConditionGroupsFromV4(b.ConditionGroups),
			Result:          r.buildParamBindingFromV6(b.Result),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildReturns(returns client.ReturnsMetaV2) IncidentEngineReturnsMeta {
	return IncidentEngineReturnsMeta{
		Array: types.BoolValue(returns.Array),
		Type:  types.StringValue(returns.Type),
	}
}
