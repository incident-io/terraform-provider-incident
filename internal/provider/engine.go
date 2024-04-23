package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type IncidentEngineConditionGroups = []IncidentEngineConditionGroup

type IncidentEngineConditionGroup struct {
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

type IncidentEngineExpression struct {
	ElseBranch    *IncidentEngineElseBranch           `tfsdk:"else_branch"`
	ID            types.String                        `tfsdk:"id"` // rmv?
	Label         types.String                        `tfsdk:"label"`
	Operations    []IncidentEngineExpressionOperation `tfsdk:"operations"`
	Reference     types.String                        `tfsdk:"reference"`
	RootReference types.String                        `tfsdk:"root_reference"`
}

type IncidentEngineElseBranch struct {
	Result IncidentEngineParamBinding `tfsdk:"result"`
}

type IncidentEngineExpressionOperation struct {
	Branches *IncidentEngineExpressionBranchesOpts `tfsdk:"branches"`
	Filter   *IncidentEngineExpressionFilterOpts   `tfsdk:"filter"`
	Navigate *IncidentEngineExpressionNavigateOpts `tfsdk:"navigate"`

	OperationType types.String                       `tfsdk:"operation_type"`
	Parse         *IncidentEngineExpressionParseOpts `tfsdk:"parse"`
}

type IncidentEngineExpressionBranchesOpts struct {
	Branches []IncidentEngineBranch    `tfsdk:"branches"`
	Returns  IncidentEngineReturnsMeta `tfsdk:"returns"`
}

type IncidentEngineBranch struct {
	Conditions []IncidentEngineCondition  `tfsdk:"conditions"`
	Result     IncidentEngineParamBinding `tfsdk:"result"`
}

type IncidentEngineReturnsMeta struct {
	Array types.Bool   `tfsdk:"array"`
	Type  types.String `tfsdk:"type"`
}

type IncidentEngineExpressionFilterOpts struct {
	Conditions []IncidentEngineCondition `tfsdk:"conditions"`
}

type IncidentEngineExpressionNavigateOpts struct {
	Reference types.String `tfsdk:"reference"`
}

type IncidentEngineExpressionParseOpts struct {
	Returns IncidentEngineReturnsMeta `tfsdk:"returns"`
	Source  types.String              `tfsdk:"source"`
}
