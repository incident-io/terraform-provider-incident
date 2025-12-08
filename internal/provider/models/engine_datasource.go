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
		"array_value": schema.ListNestedAttribute{
			MarkdownDescription: apischema.Docstring("EngineParamBindingV2", "array_value"),
			Computed:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: ParamBindingValueDataSourceAttributes(),
			},
		},
		"value": schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("EngineParamBindingV2", "value"),
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

func ConditionsDataSourceAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring("ConditionGroupV2", "conditions"),
		Computed:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"operation": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ConditionV2", "operation"),
					Computed:            true,
				},
				"param_bindings": ParamBindingsDataSourceAttribute(),
				"subject": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ConditionV2", "subject"),
					Computed:            true,
				},
			},
		},
	}
}

func ConditionGroupsDataSourceAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring("ExpressionFilterOptsV2", "condition_groups"),
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
		MarkdownDescription: apischema.Docstring("ExpressionOperationV2", "returns"),
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

func ExpressionsDataSourceAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring("WorkflowV2", "expressions"),
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
					MarkdownDescription: apischema.Docstring("ExpressionV2", "else_branch"),
					Computed:            true,
					Attributes: map[string]schema.Attribute{
						"result": schema.SingleNestedAttribute{
							MarkdownDescription: apischema.Docstring("ExpressionElseBranchV2", "result"),
							Computed:            true,
							Attributes:          ParamBindingDataSourceAttributes(),
						},
					},
				},
				"operations": schema.ListNestedAttribute{
					MarkdownDescription: apischema.Docstring("ExpressionV2", "operations"),
					Computed:            true,
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"branches": schema.SingleNestedAttribute{
								MarkdownDescription: apischema.Docstring("ExpressionOperationV2", "branches"),
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"branches": schema.ListNestedAttribute{
										MarkdownDescription: apischema.Docstring("ExpressionBranchesOptsV2", "branches"),
										Computed:            true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"condition_groups": ConditionGroupsDataSourceAttribute(),
												"result": schema.SingleNestedAttribute{
													MarkdownDescription: apischema.Docstring("ExpressionBranchV2", "result"),
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
								MarkdownDescription: apischema.Docstring("ExpressionOperationV2", "filter"),
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"condition_groups": ConditionGroupsDataSourceAttribute(),
								},
							},
							"navigate": schema.SingleNestedAttribute{
								MarkdownDescription: apischema.Docstring("ExpressionOperationV2", "navigate"),
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"reference": schema.StringAttribute{
										MarkdownDescription: apischema.Docstring("ExpressionNavigateOptsV2", "reference"),
										Computed:            true,
									},
								},
							},
							"operation_type": schema.StringAttribute{
								MarkdownDescription: apischema.Docstring("ExpressionOperationV2", "operation_type"),
								Computed:            true,
							},
							"parse": schema.SingleNestedAttribute{
								MarkdownDescription: apischema.Docstring("ExpressionOperationV2", "parse"),
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"returns": ReturnsDataSourceAttribute(),
									"source": schema.StringAttribute{
										MarkdownDescription: apischema.Docstring("ExpressionParseOptsV2", "source"),
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
