package provider

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/pkg/errors"

	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

// forceCoerce converts between two API client types which we are certain are
// identical, but the Go type system does not know that.
func forceCoerce[T any](input any) T {
	// This is a horrible hack to work around the schema having a bunch of
	// duplicated types. Until we've sorted that out, this to-and-from JSONs
	jsoned, err := json.Marshal(input)
	if err != nil {
		panic(errors.Wrap(err, "failed to marshal input"))
	}
	var res T
	if err := json.Unmarshal(jsoned, &res); err != nil {
		panic(errors.Wrap(err, "failed to unmarshal input"))
	}
	return res
}

// buildModel converts from the response type to the terraform model/schema type.
func (r *IncidentWorkflowResource) buildModel(workflow client.Workflow) *IncidentWorkflowResourceModel {
	model := &IncidentWorkflowResourceModel{
		ID:                      types.StringValue(workflow.Id),
		Name:                    types.StringValue(workflow.Name),
		Trigger:                 types.StringValue(workflow.Trigger.Name),
		ConditionGroups:         models.IncidentEngineConditionGroups{}.FromAPI(forceCoerce[[]client.ConditionGroupV2](workflow.ConditionGroups)),
		Steps:                   buildSteps(workflow.Steps),
		Expressions:             models.IncidentEngineExpressions{}.FromAPI(forceCoerce[[]client.ExpressionV2](workflow.Expressions)),
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

func buildSteps(steps []client.StepConfig) []IncidentWorkflowStep {
	out := []IncidentWorkflowStep{}

	for _, s := range steps {
		out = append(out, IncidentWorkflowStep{
			ForEach:       types.StringPointerValue(s.ForEach),
			ID:            types.StringValue(s.Id),
			Name:          types.StringValue(s.Name),
			ParamBindings: models.IncidentEngineParamBindings{}.FromAPI(s.ParamBindings),
		})
	}

	return out
}
