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
		ConditionGroups:         buildConditionGroups(workflow.ConditionGroups),
		Steps:                   buildSteps(workflow.Steps),
		Expressions:             buildExpressions(workflow.Expressions),
		OnceFor:                 buildOnceFor(workflow.OnceFor),
		RunsOnIncidentModes:     buildRunsOnIncidentModes(workflow.RunsOnIncidentModes),
		IncludePrivateIncidents: types.BoolValue(workflow.IncludePrivateIncidents),
		ContinueOnStepError:     types.BoolValue(workflow.ContinueOnStepError),
		RunsOnIncidents:         types.StringValue(string(workflow.RunsOnIncidents)),
		State:                   types.StringValue(string(workflow.State)),
	}
	if workflow.Folder != nil {
		model.Folder = types.StringValue(*workflow.Folder)
	}
	if workflow.Shortform != nil {
		model.Shortform = types.StringValue(*workflow.Shortform)
	}
	if workflow.Delay != nil {
		model.Delay = &IncidentWorkflowDelay{
			ConditionsApplyOverDelay: types.BoolValue(workflow.Delay.ConditionsApplyOverDelay),
			ForSeconds:               types.Int64Value(workflow.Delay.ForSeconds),
		}
	}
	return model
}

func buildOnceFor(onceFor []client.EngineReferenceV2) []basetypes.StringValue {
	out := []basetypes.StringValue{}

	for _, ref := range onceFor {
		out = append(out, types.StringValue(ref.Key))
	}

	return out
}

func buildRunsOnIncidentModes(modes []client.WorkflowRunsOnIncidentModes) []basetypes.StringValue {
	out := []basetypes.StringValue{}

	for _, mode := range modes {
		out = append(out, types.StringValue(string(mode)))
	}

	return out
}

func buildConditionGroups(groups []client.ConditionGroupV2) IncidentEngineConditionGroups {
	out := IncidentEngineConditionGroups{}

	for _, g := range groups {
		out = append(out, IncidentEngineConditionGroup{
			Conditions: buildConditions(g.Conditions),
		})
	}

	return out
}

func buildConditions(conditions []client.ConditionV2) []IncidentEngineCondition {
	out := []IncidentEngineCondition{}

	for _, c := range conditions {
		out = append(out, IncidentEngineCondition{
			Subject:       types.StringValue(c.Subject.Reference),
			Operation:     types.StringValue(c.Operation.Value),
			ParamBindings: buildParamBindings(c.ParamBindings),
		})
	}

	return out
}

func buildSteps(steps []client.StepConfig) []IncidentWorkflowStep {
	out := []IncidentWorkflowStep{}

	for _, s := range steps {
		out = append(out, IncidentWorkflowStep{
			ForEach:       types.StringPointerValue(s.ForEach),
			ID:            types.StringValue(s.Id),
			Name:          types.StringValue(s.Name),
			ParamBindings: buildParamBindings(s.ParamBindings),
		})
	}

	return out
}

func buildParamBindings(pbs []client.EngineParamBindingV2) []IncidentEngineParamBinding {
	out := []IncidentEngineParamBinding{}

	for _, pb := range pbs {
		out = append(out, IncidentEngineParamBinding{}.FromEngineParamBindingV2(pb))
	}

	return out
}

func buildExpressions(expressions []client.ExpressionV2) IncidentEngineExpressions {
	out := IncidentEngineExpressions{}

	for _, e := range expressions {
		expression := IncidentEngineExpression{
			Label:         types.StringValue(e.Label),
			Operations:    buildOperations(e.Operations),
			Reference:     types.StringValue(e.Reference),
			RootReference: types.StringValue(e.RootReference),
		}
		if e.ElseBranch != nil {
			expression.ElseBranch = &IncidentEngineElseBranch{
				Result: IncidentEngineParamBinding{}.FromEngineParamBindingV2(e.ElseBranch.Result),
			}
		}
		out = append(out, expression)
	}

	return out
}

func buildOperations(operations []client.ExpressionOperationV2) []IncidentEngineExpressionOperation {
	out := []IncidentEngineExpressionOperation{}

	for _, o := range operations {
		operation := IncidentEngineExpressionOperation{
			OperationType: types.StringValue(string(o.OperationType)),
		}
		if o.Branches != nil {
			operation.Branches = &IncidentEngineExpressionBranchesOpts{
				Branches: buildBranches(o.Branches.Branches),
				Returns:  buildReturns(o.Branches.Returns),
			}
		}
		if o.Filter != nil {
			operation.Filter = &IncidentEngineExpressionFilterOpts{
				ConditionGroups: buildConditionGroups(o.Filter.ConditionGroups),
			}
		}
		if o.Navigate != nil {
			operation.Navigate = &IncidentEngineExpressionNavigateOpts{
				Reference: types.StringValue(o.Navigate.Reference),
			}
		}
		if o.Parse != nil {
			operation.Parse = &IncidentEngineExpressionParseOpts{
				Returns: buildReturns(o.Parse.Returns),
				Source:  types.StringValue(o.Parse.Source),
			}
		}
		out = append(out, operation)
	}

	return out
}

func buildBranches(branches []client.ExpressionBranchV2) []IncidentEngineBranch {
	out := []IncidentEngineBranch{}

	for _, b := range branches {
		out = append(out, IncidentEngineBranch{
			ConditionGroups: buildConditionGroups(b.ConditionGroups),
			Result:          IncidentEngineParamBinding{}.FromEngineParamBindingV2(b.Result),
		})
	}

	return out
}

func buildReturns(returns client.ReturnsMetaV2) IncidentEngineReturnsMeta {
	return IncidentEngineReturnsMeta{
		Array: types.BoolValue(returns.Array),
		Type:  types.StringValue(returns.Type),
	}
}
