package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type IncidentEngineConditionGroups = []IncidentEngineConditions

type IncidentEngineConditions struct {
	Conditions []IncidentEngineCondition `tfsdk:"conditions"`
}

type IncidentEngineCondition struct {
	Operation     IncidentEngineConditionOperation `tfsdk:"operation"`
	ParamBindings []IncidentEngineParamBinding
	Params        []IncidentEngineParam
	Subject       IncidentEngineConditionSubject
}

type IncidentEngineConditionOperation struct {
	Label types.String `tfsdk:"label"`
	Value types.String `tfsdk:"value"`
}

type IncidentEngineParamBinding struct {
	ArrayValue []*IncidentEngineParamBindingValue `tfsdk:"array_value"`
	Value      *IncidentEngineParamBindingValue   `tfsdk:"value"`
}

type IncidentEngineParam struct { // Add the rest ..
	Description types.String `tfsdk:"description"`
}

type IncidentEngineConditionSubject struct {
	Icon      types.String `tfsdk:"icon"`
	Label     types.String `tfsdk:"label"`
	Reference types.String `tfsdk:"reference"`
}

type IncidentEngineParamBindingValue struct { // Add the rest ..
	Label types.String `tfsdk:"label"`
}
