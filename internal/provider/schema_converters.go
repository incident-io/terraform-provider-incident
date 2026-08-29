package provider

import (
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// buildModel converts from the response type to the terraform model/schema type. prior
// is the plan (create/update) or prior state (read), and nil on import.
func (r *IncidentWorkflowResource) buildModel(workflow client.WorkflowV2, prior *IncidentWorkflowResourceModel) *IncidentWorkflowResourceModel {
	model := &IncidentWorkflowResourceModel{
		ID:                        types.StringValue(workflow.Id),
		Name:                      types.StringValue(workflow.Name),
		Trigger:                   types.StringValue(workflow.Trigger.Name),
		ConditionGroups:           models.IncidentEngineConditionGroups{}.FromAPI(workflow.ConditionGroups),
		Steps:                     buildSteps(workflow.Steps, prior),
		Expressions:               models.IncidentEngineExpressions{}.FromAPI(workflow.Expressions),
		OnceFor:                   buildOnceFor(workflow.OnceFor),
		RunsOnIncidentModes:       buildRunsOnIncidentModes(workflow.RunsOnIncidentModes),
		IncludePrivateIncidents:   types.BoolValue(workflow.IncludePrivateIncidents),
		PrivateIncidentScope:      types.StringValue(string(workflow.PrivateIncidentScope)),
		IncludePrivateEscalations: types.BoolPointerValue(lo.ToPtr(workflow.IncludePrivateEscalations)),
		OwningTeamIDs:             buildOwningTeamIDs(workflow.OwningTeamIds),
		ContinueOnStepError:       types.BoolValue(workflow.ContinueOnStepError),
		RunsOnIncidents:           types.StringValue(string(workflow.RunsOnIncidents)),
		State:                     types.StringValue(string(workflow.State)),
		FormFields:                buildFormFields(workflow.FormFields),
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

	if prior != nil {
		model.ConditionGroups.ReconcileSpelling(prior.ConditionGroups)
		model.Expressions.ReconcileSpelling(prior.Expressions)
		model.FormFields = reconcileFormFields(prior.FormFields, model.FormFields)
	}

	return model
}

// buildFormFields converts workflow form fields from the API response into the
// Terraform model. It returns nil (rather than an empty slice) when the API has
// no form fields, so workflows without any configured form fields don't show a
// perpetual diff against an unset attribute.
func buildFormFields(fields *[]client.WorkflowFormFieldV2) []IncidentWorkflowFormField {
	if fields == nil || len(*fields) == 0 {
		return nil
	}

	out := []IncidentWorkflowFormField{}
	for _, f := range *fields {
		out = append(out, IncidentWorkflowFormField{
			ID:          types.StringValue(f.Id),
			Key:         types.StringValue(f.Key),
			Title:       types.StringValue(f.Title),
			Type:        types.StringValue(f.Type),
			Array:       types.BoolValue(f.Array),
			Required:    types.BoolValue(f.Required),
			Description: types.StringPointerValue(f.Description),
		})
	}

	return out
}

// reconcileFormFields keeps an explicitly empty form_fields list empty in state.
//
// The API can't tell "no form fields" from an empty list, so buildFormFields
// collapses both to nil to keep an unset attribute quiet. That's right for a
// config that omitted form_fields — toPayloadFormFields cleared them, so null is
// what the plan said and what the API now has — but wrong for a user who wrote
// `form_fields = []`, whose plan said empty. Terraform rejects the apply as an
// inconsistent result if we hand back null instead, so put the empty list back.
//
// prior is the planned value on create/update and the prior state on read.
// buildModel skips this on import, where there's no planned value to respect and
// the caller wants whatever the API has.
func reconcileFormFields(prior, built []IncidentWorkflowFormField) []IncidentWorkflowFormField {
	if built == nil && prior != nil && len(prior) == 0 {
		return []IncidentWorkflowFormField{}
	}

	return built
}

func buildOnceFor(onceFor []client.EngineReferenceV2) []basetypes.StringValue {
	out := []basetypes.StringValue{}

	for _, ref := range onceFor {
		out = append(out, types.StringValue(ref.Key))
	}

	return out
}

func buildOwningTeamIDs(teamIDs *[]string) types.Set {
	if teamIDs == nil {
		return types.SetNull(types.StringType)
	}

	elements := make([]attr.Value, len(*teamIDs))
	for i, teamID := range *teamIDs {
		elements[i] = types.StringValue(teamID)
	}

	return types.SetValueMust(types.StringType, elements)
}

func buildRunsOnIncidentModes(modes []client.WorkflowV2RunsOnIncidentModes) types.Set {
	elements := make([]attr.Value, len(modes))
	for i, mode := range modes {
		elements[i] = types.StringValue(string(mode))
	}

	return types.SetValueMust(types.StringType, elements)
}

func buildSteps(steps []client.StepConfigV2, prior *IncidentWorkflowResourceModel) []IncidentWorkflowStep {
	out := []IncidentWorkflowStep{}

	// Keyed by step ID so we survive reordering. Unseen steps keep all their bindings.
	priorSteps := map[string]IncidentWorkflowStep{}
	if prior != nil {
		for _, step := range prior.Steps {
			priorSteps[step.ID.ValueString()] = step
		}
	}

	for _, s := range steps {
		paramBindings := models.IncidentEngineParamBindings{}.FromAPI(s.ParamBindings)
		if priorStep, ok := priorSteps[s.Id]; ok {
			paramBindings = paramBindings.
				TrimAppendedEmpty(len(priorStep.ParamBindings)).
				ReconcileSpelling(priorStep.ParamBindings)
		}

		out = append(out, IncidentWorkflowStep{
			ForEach:       types.StringPointerValue(s.ForEach),
			ID:            types.StringValue(s.Id),
			Name:          types.StringValue(s.Name),
			ParamBindings: paramBindings,
		})
	}

	return out
}

func buildCatalogEntryAttributeValuesFromV3(attributes map[string]client.CatalogEntryEngineParamBindingV3) []CatalogEntryAttributeValue {
	var values []CatalogEntryAttributeValue

	for attributeID, binding := range attributes {
		value := CatalogEntryAttributeValue{
			Attribute:  types.StringValue(attributeID),
			Value:      types.StringNull(),
			ArrayValue: types.ListNull(types.StringType),
		}

		if binding.Value != nil {
			value.Value = types.StringValue(*binding.Value.Literal)
		}

		if binding.ArrayValue != nil {
			elements := []attr.Value{}
			for _, v := range *binding.ArrayValue {
				elements = append(elements, types.StringValue(*v.Literal))
			}
			value.ArrayValue = types.ListValueMust(types.StringType, elements)
		}

		values = append(values, value)
	}

	// Ensure consistent ordering
	sort.Slice(values, func(i, j int) bool {
		return values[i].Attribute.ValueString() < values[j].Attribute.ValueString()
	})

	return values
}
