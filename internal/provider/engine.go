package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
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
		MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2ResponseBody", "literal"),
		Optional:            true,
	},
	"reference": schema.StringAttribute{
		MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2ResponseBody", "reference"),
		Optional:            true,
	},
}

var paramBindingAttributes = map[string]schema.Attribute{
	"array_value": schema.SetNestedAttribute{
		MarkdownDescription: "The array of literal or reference parameter values",
		Optional:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: paramBindingValueAttributes,
		},
	},
	"value": schema.SingleNestedAttribute{
		MarkdownDescription: "The literal or reference parameter value",
		Optional:            true,
		Attributes:          paramBindingValueAttributes,
	},
}

var paramBindingsAttribute = schema.ListNestedAttribute{
	MarkdownDescription: apischema.Docstring("ConditionV2ResponseBody", "param_bindings"),
	Required:            true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: paramBindingAttributes,
	},
}

var conditionsAttribute = schema.SetNestedAttribute{
	MarkdownDescription: "The prerequisite conditions that must all be satisfied",
	Required:            true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"operation": schema.StringAttribute{
				MarkdownDescription: "The logical operation to be applied",
				Required:            true,
			},
			"param_bindings": paramBindingsAttribute,
			"subject": schema.StringAttribute{
				MarkdownDescription: "The subject of the condition, on which the operation is applied",
				Required:            true,
			},
		},
	},
}

var conditionGroupsAttribute = schema.SetNestedAttribute{
	MarkdownDescription: "Groups of prerequisite conditions. All conditions in at least one group must be satisfied",
	Required:            true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"conditions": conditionsAttribute,
		},
	},
}

var returnsAttribute = schema.SingleNestedAttribute{
	MarkdownDescription: "The return type of an operation",
	Required:            true,
	Attributes: map[string]schema.Attribute{
		"array": schema.BoolAttribute{
			MarkdownDescription: apischema.Docstring("ReturnsMetaV2ResponseBody", "array"),
			Required:            true,
		},
		"type": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("ReturnsMetaV2ResponseBody", "type"),
			Required:            true,
		},
	},
}

var expressionsAttribute = schema.SetNestedAttribute{
	MarkdownDescription: "The expressions to be prepared for use by steps and conditions",
	Required:            true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("ExpressionV2ResponseBody", "id"),
				Computed:            true,
			},
			"label": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("ExpressionV2ResponseBody", "label"),
				Required:            true,
			},
			"reference": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("ExpressionV2ResponseBody", "reference"),
				Required:            true,
			},
			"root_reference": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("ExpressionV2ResponseBody", "root_reference"),
				Required:            true,
			},
			"else_branch": schema.SingleNestedAttribute{
				MarkdownDescription: "The else branch to resort to if all operations fail",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"result": schema.SingleNestedAttribute{
						MarkdownDescription: "The result assumed if the else branch is reached",
						Required:            true,
						Attributes:          paramBindingAttributes,
					},
				},
			},
			"operations": schema.ListNestedAttribute{
				MarkdownDescription: "The operations to execute in sequence for this expression",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"branches": schema.SingleNestedAttribute{
							MarkdownDescription: "An operation type that allows for a value to be set conditionally by a series of logical branches",
							Optional:            true,
							Attributes: map[string]schema.Attribute{
								"branches": schema.ListNestedAttribute{
									MarkdownDescription: apischema.Docstring("ExpressionBranchesOptsV2ResponseBody", "branches"),
									Required:            true,
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"condition_groups": conditionGroupsAttribute,
											"result": schema.SingleNestedAttribute{
												MarkdownDescription: "The result assumed if the condition groups are satisfied",
												Required:            true,
												Attributes:          paramBindingAttributes,
											},
										},
									},
								},
								"returns": returnsAttribute,
							},
						},
						"filter": schema.SingleNestedAttribute{
							MarkdownDescription: "An operation type that allows values to be filtered out by conditions",
							Optional:            true,
							Attributes: map[string]schema.Attribute{
								"condition_groups": conditionGroupsAttribute,
							},
						},
						"navigate": schema.SingleNestedAttribute{
							MarkdownDescription: "An operation type that allows attributes of a type to be accessed by reference",
							Optional:            true,
							Attributes: map[string]schema.Attribute{
								"reference": schema.StringAttribute{
									Required: true,
								},
							},
						},
						"operation_type": schema.StringAttribute{
							MarkdownDescription: "Indicates which operation type to execute",
							Required:            true,
						},
						"parse": schema.SingleNestedAttribute{
							MarkdownDescription: "An operation type that allows a value to parsed from within a JSON object",
							Optional:            true,
							Attributes: map[string]schema.Attribute{
								"returns": returnsAttribute,
								"source": schema.StringAttribute{
									MarkdownDescription: "The ES5 Javascript expression to execute",
									Required:            true,
								},
							},
						},
					},
				},
			},
		},
	},
}
