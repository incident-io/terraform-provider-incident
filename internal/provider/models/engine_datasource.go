package models

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
)

// Data source attribute helpers (computed versions)

func ParamBindingValueDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"literal": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
			Computed:            true,
		},
		"reference": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
			Computed:            true,
		},
	}
}

func ParamBindingDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"array_value": schema.SetNestedAttribute{
			MarkdownDescription: "The array of literal or reference parameter values",
			Computed:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: ParamBindingValueDataSourceAttributes(),
			},
		},
		"value": schema.SingleNestedAttribute{
			MarkdownDescription: "The literal or reference parameter value",
			Computed:            true,
			Attributes:          ParamBindingValueDataSourceAttributes(),
		},
	}
}

func ParamBindingsDataSourceAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring("ConditionV2", "param_bindings"),
		Computed:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: ParamBindingDataSourceAttributes(),
		},
	}
}

func ConditionsDataSourceAttribute() schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		MarkdownDescription: "The prerequisite conditions that must all be satisfied",
		Computed:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"operation": schema.StringAttribute{
					MarkdownDescription: "The logical operation to be applied",
					Computed:            true,
				},
				"param_bindings": ParamBindingsDataSourceAttribute(),
				"subject": schema.StringAttribute{
					MarkdownDescription: "The subject of the condition, on which the operation is applied",
					Computed:            true,
				},
			},
		},
	}
}

func ConditionGroupsDataSourceAttribute() schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		MarkdownDescription: "Groups of prerequisite conditions. All conditions in at least one group must be satisfied",
		Computed:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"conditions": ConditionsDataSourceAttribute(),
			},
		},
	}
}

func ReturnsDataSourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "The return type of an operation",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"array": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "array"),
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "type"),
				Computed:            true,
			},
		},
	}
}

func ExpressionsDataSourceAttribute() schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		MarkdownDescription: "The expressions to be prepared for use by steps and conditions",
		Computed:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"label": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ExpressionV2", "label"),
					Computed:            true,
				},
				"reference": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ExpressionV2", "reference"),
					Computed:            true,
				},
				"root_reference": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ExpressionV2", "root_reference"),
					Computed:            true,
				},
				"else_branch": schema.SingleNestedAttribute{
					MarkdownDescription: "The else branch to resort to if all operations fail",
					Computed:            true,
					Attributes: map[string]schema.Attribute{
						"result": schema.SingleNestedAttribute{
							MarkdownDescription: "The result assumed if the else branch is reached",
							Computed:            true,
							Attributes:          ParamBindingDataSourceAttributes(),
						},
					},
				},
				"operations": schema.ListNestedAttribute{
					MarkdownDescription: "The operations to execute in sequence for this expression",
					Computed:            true,
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"branches": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows for a value to be set conditionally by a series of logical branches",
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"branches": schema.ListNestedAttribute{
										MarkdownDescription: apischema.Docstring("ExpressionBranchesOptsV2", "branches"),
										Computed:            true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"condition_groups": ConditionGroupsDataSourceAttribute(),
												"result": schema.SingleNestedAttribute{
													MarkdownDescription: "The result assumed if the condition groups are satisfied",
													Computed:            true,
													Attributes:          ParamBindingDataSourceAttributes(),
												},
											},
										},
									},
									"returns": ReturnsDataSourceAttribute(),
								},
							},
							"filter": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows values to be filtered out by conditions",
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"condition_groups": ConditionGroupsDataSourceAttribute(),
								},
							},
							"navigate": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows attributes of a type to be accessed by reference",
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"reference": schema.StringAttribute{
										Computed: true,
									},
								},
							},
							"operation_type": schema.StringAttribute{
								MarkdownDescription: "Indicates which operation type to execute",
								Computed:            true,
							},
							"parse": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows a value to parsed from within a JSON object",
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"returns": ReturnsDataSourceAttribute(),
									"source": schema.StringAttribute{
										MarkdownDescription: "The ES5 Javascript expression to execute",
										Computed:            true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
