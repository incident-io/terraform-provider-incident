package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Types

type IncidentEngineConditionGroups = []IncidentEngineConditionGroup

type IncidentEngineConditionGroup struct {
	Conditions []IncidentEngineCondition `tfsdk:"conditions"`
}

type IncidentEngineCondition struct {
	Subject       types.String                 `tfsdk:"subject"`
	Operation     types.String                 `tfsdk:"operation"`
	ParamBindings []IncidentEngineParamBinding `tfsdk:"param_bindings"`
}

type IncidentEngineParamBinding struct {
	ArrayValue []IncidentEngineParamBindingValue `tfsdk:"array_value"`
	Value      *IncidentEngineParamBindingValue  `tfsdk:"value"`
}

type IncidentEngineParamBindingValue struct {
	Literal   types.String `tfsdk:"literal"`
	Reference types.String `tfsdk:"reference"`
}

type IncidentEngineExpressions []IncidentEngineExpression

type IncidentEngineExpression struct {
	ElseBranch    *IncidentEngineElseBranch           `tfsdk:"else_branch"`
	ID            types.String                        `tfsdk:"id"`
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
	Parse    *IncidentEngineExpressionParseOpts    `tfsdk:"parse"`

	OperationType types.String `tfsdk:"operation_type"`
}

type IncidentEngineExpressionBranchesOpts struct {
	Branches []IncidentEngineBranch    `tfsdk:"branches"`
	Returns  IncidentEngineReturnsMeta `tfsdk:"returns"`
}

type IncidentEngineBranch struct {
	ConditionGroups []IncidentEngineConditionGroup `tfsdk:"condition_groups"`
	Result          IncidentEngineParamBinding     `tfsdk:"result"`
}

type IncidentEngineReturnsMeta struct {
	Array types.Bool   `tfsdk:"array"`
	Type  types.String `tfsdk:"type"`
}

type IncidentEngineExpressionFilterOpts struct {
	ConditionGroups []IncidentEngineConditionGroup `tfsdk:"condition_groups"`
}

type IncidentEngineExpressionNavigateOpts struct {
	Reference types.String `tfsdk:"reference"`
}

type IncidentEngineExpressionParseOpts struct {
	Returns IncidentEngineReturnsMeta `tfsdk:"returns"`
	Source  types.String              `tfsdk:"source"`
}

// Attributes

var paramBindingValueAttributes = map[string]schema.Attribute{
	"literal": schema.StringAttribute{
		Optional: true,
	},
	"reference": schema.StringAttribute{
		Optional: true,
	},
}

var paramBindingAttributes = map[string]schema.Attribute{
	"array_value": schema.SetNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: paramBindingValueAttributes,
		},
	},
	"value": schema.SingleNestedAttribute{
		Optional:   true,
		Attributes: paramBindingValueAttributes,
	},
}

var paramBindingsAttribute = schema.ListNestedAttribute{
	Required: true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: paramBindingAttributes,
	},
}

var conditionsAttribute = schema.SetNestedAttribute{
	Required: true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"operation": schema.StringAttribute{
				Required: true,
			},
			"param_bindings": paramBindingsAttribute,
			"subject": schema.StringAttribute{
				Required: true,
			},
		},
	},
}

var conditionGroupsAttribute = schema.SetNestedAttribute{
	Required: true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"conditions": conditionsAttribute,
		},
	},
}

var returnsAttribute = schema.SingleNestedAttribute{
	Required: true,
	Attributes: map[string]schema.Attribute{
		"array": schema.BoolAttribute{
			Required: true,
		},
		"type": schema.StringAttribute{
			Required: true,
		},
	},
}

var expressionsAttribute = schema.SetNestedAttribute{
	Required: true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"label": schema.StringAttribute{
				Required: true,
			},
			"reference": schema.StringAttribute{
				Required: true,
			},
			"root_reference": schema.StringAttribute{
				Required: true,
			},
			"else_branch": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"result": schema.SingleNestedAttribute{
						Required:   true,
						Attributes: paramBindingAttributes,
					},
				},
			},
			"operations": schema.ListNestedAttribute{
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"branches": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"branches": schema.ListNestedAttribute{
									Required: true,
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"condition_groups": conditionGroupsAttribute,
											"result": schema.SingleNestedAttribute{
												Required:   true,
												Attributes: paramBindingAttributes,
											},
										},
									},
								},
								"returns": returnsAttribute,
							},
						},
						"filter": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"condition_groups": conditionGroupsAttribute,
							},
						},
						"navigate": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"reference": schema.StringAttribute{
									Required: true,
								},
							},
						},
						"operation_type": schema.StringAttribute{
							Required: true,
						},
						"parse": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"returns": returnsAttribute,
								"source": schema.StringAttribute{
									Required: true,
								},
							},
						},
					},
				},
			},
		},
	},
}
