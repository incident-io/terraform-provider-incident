package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type IncidentEngineConditionGroups = []IncidentEngineConditions

type IncidentEngineConditions struct {
	Conditions []IncidentEngineCondition `tfsdk:"conditions"`
}

type IncidentEngineCondition struct {
	Operation     types.String                 `tfsdk:"operation"`
	ParamBindings []IncidentEngineParamBinding `tfsdk:"param_bindings"`
	Subject       types.String                 `tfsdk:"subject"`
}

type IncidentEngineParamBinding struct {
	ArrayValue []IncidentEngineParamBindingValue `tfsdk:"array_value"`
	Value      *IncidentEngineParamBindingValue  `tfsdk:"value"`
}

type IncidentEngineParamBindingValue struct {
	Literal   types.String `tfsdk:"literal"`
	Reference types.String `tfsdk:"reference"`
}
