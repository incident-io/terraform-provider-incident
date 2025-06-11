package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentAlertSourceDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentAlertSourceDataSource{}
)

func NewIncidentAlertSourceDataSource() datasource.DataSource {
	return &IncidentAlertSourceDataSource{}
}

type IncidentAlertSourceDataSource struct {
	client *client.ClientWithResponses
}

type IncidentAlertSourceDataSourceModel struct {
	ID           types.String                        `tfsdk:"id"`
	Name         types.String                        `tfsdk:"name"`
	SourceType   types.String                        `tfsdk:"source_type"`
	SecretToken  types.String                        `tfsdk:"secret_token"`
	Template     *models.AlertTemplateModel          `tfsdk:"template"`
	JiraOptions  *models.AlertSourceJiraOptionsModel `tfsdk:"jira_options"`
	EmailAddress types.String                        `tfsdk:"email_address"`
}

func (d *IncidentAlertSourceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client.Client
}

func (d *IncidentAlertSourceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source"
}

func (d *IncidentAlertSourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentAlertSourceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var alertSource *client.AlertSourceV2
	if !data.ID.IsNull() {
		result, err := d.client.AlertSourcesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", string(result.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source, got error: %s", err))
			return
		}
		alertSource = &result.JSON200.AlertSource
	} else if !data.Name.IsNull() {
		result, err := d.client.AlertSourcesV2ListWithResponse(ctx)
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", string(result.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list alert sources, got error: %s", err))
			return
		}

		var found *client.AlertSourceV2
		for _, source := range result.JSON200.AlertSources {
			if source.Name == data.Name.ValueString() {
				if found != nil {
					resp.Diagnostics.AddError("Client Error", "Multiple alert sources found with the same name")
					return
				}
				found = &source
			}
		}

		if found == nil {
			resp.Diagnostics.AddError("Client Error", "Alert source not found")
			return
		}
		alertSource = found
	} else {
		resp.Diagnostics.AddError("Client Error", "Either 'id' or 'name' must be specified")
		return
	}

	modelResp := d.buildModel(*alertSource)
	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

func (d *IncidentAlertSourceDataSource) buildModel(source client.AlertSourceV2) *IncidentAlertSourceDataSourceModel {
	var emailAddress *string
	if source.EmailOptions != nil {
		emailAddress = &source.EmailOptions.EmailAddress
	}

	return &IncidentAlertSourceDataSourceModel{
		ID:          types.StringValue(source.Id),
		Name:        types.StringValue(source.Name),
		SourceType:  types.StringValue(string(source.SourceType)),
		SecretToken: types.StringPointerValue(source.SecretToken),
		Template: &models.AlertTemplateModel{
			Title:       models.IncidentEngineParamBindingValue{}.FromAPI(source.Template.Title),
			Description: models.IncidentEngineParamBindingValue{}.FromAPI(source.Template.Description),
			Attributes:  models.AlertTemplateAttributesModel{}.FromAPI(source.Template.Attributes),
			Expressions: models.IncidentEngineExpressions{}.FromAPI(source.Template.Expressions),
		},
		JiraOptions:  models.AlertSourceJiraOptionsModel{}.FromAPI(source.JiraOptions),
		EmailAddress: types.StringPointerValue(emailAddress),
	}
}

func (d *IncidentAlertSourceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Alert Sources V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "id"),
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "name"),
			},
			"source_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "source_type"),
			},
			"secret_token": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "secret_token"),
			},
			"template": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "template"),
				Attributes: map[string]schema.Attribute{
					"expressions": schema.SetNestedAttribute{
						Computed:            true,
						MarkdownDescription: "The expressions to be prepared for use by steps and conditions",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"label": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("ExpressionV2", "label"),
								},
								"reference": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("ExpressionV2", "reference"),
								},
								"root_reference": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("ExpressionV2", "root_reference"),
								},
								"else_branch": schema.SingleNestedAttribute{
									Computed:            true,
									MarkdownDescription: "The else branch to resort to if all operations fail",
									Attributes: map[string]schema.Attribute{
										"result": schema.SingleNestedAttribute{
											Computed:            true,
											MarkdownDescription: "The result assumed if the else branch is reached",
											Attributes: map[string]schema.Attribute{
												"array_value": schema.SetNestedAttribute{
													Computed:            true,
													MarkdownDescription: "The array of literal or reference parameter values",
													NestedObject: schema.NestedAttributeObject{
														Attributes: map[string]schema.Attribute{
															"literal": schema.StringAttribute{
																Computed:            true,
																MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
															},
															"reference": schema.StringAttribute{
																Computed:            true,
																MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
															},
														},
													},
												},
												"value": schema.SingleNestedAttribute{
													Computed:            true,
													MarkdownDescription: "The literal or reference parameter value",
													Attributes: map[string]schema.Attribute{
														"literal": schema.StringAttribute{
															Computed:            true,
															MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
														},
														"reference": schema.StringAttribute{
															Computed:            true,
															MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
														},
													},
												},
											},
										},
									},
								},
								"operations": schema.ListNestedAttribute{
									Computed:            true,
									MarkdownDescription: "The operations to execute in sequence for this expression",
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"operation_type": schema.StringAttribute{
												Computed:            true,
												MarkdownDescription: "Indicates which operation type to execute",
											},
											"branches": schema.SingleNestedAttribute{
												Computed:            true,
												MarkdownDescription: "An operation type that allows for a value to be set conditionally by a series of logical branches",
												Attributes: map[string]schema.Attribute{
													"returns": schema.SingleNestedAttribute{
														Computed:            true,
														MarkdownDescription: "The return type of an operation",
														Attributes: map[string]schema.Attribute{
															"array": schema.BoolAttribute{
																Computed:            true,
																MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "array"),
															},
															"type": schema.StringAttribute{
																Computed:            true,
																MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "type"),
															},
														},
													},
													"branches": schema.ListNestedAttribute{
														Computed:            true,
														MarkdownDescription: apischema.Docstring("ExpressionBranchesOptsV2", "branches"),
														NestedObject: schema.NestedAttributeObject{
															Attributes: map[string]schema.Attribute{
																"condition_groups": schema.SetNestedAttribute{
																	Computed:            true,
																	MarkdownDescription: "Groups of prerequisite conditions. All conditions in at least one group must be satisfied",
																	NestedObject: schema.NestedAttributeObject{
																		Attributes: map[string]schema.Attribute{
																			"conditions": schema.SetNestedAttribute{
																				Computed:            true,
																				MarkdownDescription: "The prerequisite conditions that must all be satisfied",
																				NestedObject: schema.NestedAttributeObject{
																					Attributes: map[string]schema.Attribute{
																						"operation": schema.StringAttribute{
																							Computed:            true,
																							MarkdownDescription: "The logical operation to be applied",
																						},
																						"subject": schema.StringAttribute{
																							Computed:            true,
																							MarkdownDescription: "The subject of the condition, on which the operation is applied",
																						},
																						"param_bindings": schema.ListNestedAttribute{
																							Computed:            true,
																							MarkdownDescription: apischema.Docstring("ConditionV2", "param_bindings"),
																							NestedObject: schema.NestedAttributeObject{
																								Attributes: map[string]schema.Attribute{
																									"array_value": schema.SetNestedAttribute{
																										Computed:            true,
																										MarkdownDescription: "The array of literal or reference parameter values",
																										NestedObject: schema.NestedAttributeObject{
																											Attributes: map[string]schema.Attribute{
																												"literal": schema.StringAttribute{
																													Computed:            true,
																													MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
																												},
																												"reference": schema.StringAttribute{
																													Computed:            true,
																													MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
																												},
																											},
																										},
																									},
																									"value": schema.SingleNestedAttribute{
																										Computed:            true,
																										MarkdownDescription: "The literal or reference parameter value",
																										Attributes: map[string]schema.Attribute{
																											"literal": schema.StringAttribute{
																												Computed:            true,
																												MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
																											},
																											"reference": schema.StringAttribute{
																												Computed:            true,
																												MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
																											},
																										},
																									},
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																},
																"result": schema.SingleNestedAttribute{
																	Computed:            true,
																	MarkdownDescription: "The result assumed if the condition groups are satisfied",
																	Attributes: map[string]schema.Attribute{
																		"array_value": schema.SetNestedAttribute{
																			Computed:            true,
																			MarkdownDescription: "The array of literal or reference parameter values",
																			NestedObject: schema.NestedAttributeObject{
																				Attributes: map[string]schema.Attribute{
																					"literal": schema.StringAttribute{
																						Computed:            true,
																						MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
																					},
																					"reference": schema.StringAttribute{
																						Computed:            true,
																						MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
																					},
																				},
																			},
																		},
																		"value": schema.SingleNestedAttribute{
																			Computed:            true,
																			MarkdownDescription: "The literal or reference parameter value",
																			Attributes: map[string]schema.Attribute{
																				"literal": schema.StringAttribute{
																					Computed:            true,
																					MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
																				},
																				"reference": schema.StringAttribute{
																					Computed:            true,
																					MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
											"filter": schema.SingleNestedAttribute{
												Computed:            true,
												MarkdownDescription: "An operation type that allows values to be filtered out by conditions",
												Attributes: map[string]schema.Attribute{
													"condition_groups": schema.SetNestedAttribute{
														Computed:            true,
														MarkdownDescription: "Groups of prerequisite conditions. All conditions in at least one group must be satisfied",
														NestedObject: schema.NestedAttributeObject{
															Attributes: map[string]schema.Attribute{
																"conditions": schema.SetNestedAttribute{
																	Computed:            true,
																	MarkdownDescription: "The prerequisite conditions that must all be satisfied",
																	NestedObject: schema.NestedAttributeObject{
																		Attributes: map[string]schema.Attribute{
																			"operation": schema.StringAttribute{
																				Computed:            true,
																				MarkdownDescription: "The logical operation to be applied",
																			},
																			"subject": schema.StringAttribute{
																				Computed:            true,
																				MarkdownDescription: "The subject of the condition, on which the operation is applied",
																			},
																			"param_bindings": schema.ListNestedAttribute{
																				Computed:            true,
																				MarkdownDescription: apischema.Docstring("ConditionV2", "param_bindings"),
																				NestedObject: schema.NestedAttributeObject{
																					Attributes: map[string]schema.Attribute{
																						"array_value": schema.SetNestedAttribute{
																							Computed:            true,
																							MarkdownDescription: "The array of literal or reference parameter values",
																							NestedObject: schema.NestedAttributeObject{
																								Attributes: map[string]schema.Attribute{
																									"literal": schema.StringAttribute{
																										Computed:            true,
																										MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
																									},
																									"reference": schema.StringAttribute{
																										Computed:            true,
																										MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
																									},
																								},
																							},
																						},
																						"value": schema.SingleNestedAttribute{
																							Computed:            true,
																							MarkdownDescription: "The literal or reference parameter value",
																							Attributes: map[string]schema.Attribute{
																								"literal": schema.StringAttribute{
																									Computed:            true,
																									MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
																								},
																								"reference": schema.StringAttribute{
																									Computed:            true,
																									MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
											"navigate": schema.SingleNestedAttribute{
												Computed:            true,
												MarkdownDescription: "An operation type that allows attributes of a type to be accessed by reference",
												Attributes: map[string]schema.Attribute{
													"reference": schema.StringAttribute{
														Computed: true,
													},
												},
											},
											"parse": schema.SingleNestedAttribute{
												Computed:            true,
												MarkdownDescription: "An operation type that allows a value to parsed from within a JSON object",
												Attributes: map[string]schema.Attribute{
													"returns": schema.SingleNestedAttribute{
														Computed:            true,
														MarkdownDescription: "The return type of an operation",
														Attributes: map[string]schema.Attribute{
															"array": schema.BoolAttribute{
																Computed:            true,
																MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "array"),
															},
															"type": schema.StringAttribute{
																Computed:            true,
																MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "type"),
															},
														},
													},
													"source": schema.StringAttribute{
														Computed:            true,
														MarkdownDescription: "The ES5 Javascript expression to execute",
													},
												},
											},
										},
									},
								},
							},
						},
					},
					"title": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "title"),
						Attributes: map[string]schema.Attribute{
							"literal": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
							},
							"reference": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
							},
						},
					},
					"description": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "description"),
						Attributes: map[string]schema.Attribute{
							"literal": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
							},
							"reference": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
							},
						},
					},
					"attributes": schema.SetNestedAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "attributes"),
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"alert_attribute_id": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplateAttributePayloadV2", "alert_attribute_id"),
								},
								"binding": schema.SingleNestedAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplateAttributePayloadV2", "binding"),
									Attributes: map[string]schema.Attribute{
										"array_value": schema.SetNestedAttribute{
											Computed:            true,
											MarkdownDescription: "The array of literal or reference parameter values",
											NestedObject: schema.NestedAttributeObject{
												Attributes: map[string]schema.Attribute{
													"literal": schema.StringAttribute{
														Computed:            true,
														MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
													},
													"reference": schema.StringAttribute{
														Computed:            true,
														MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
													},
												},
											},
										},
										"value": schema.SingleNestedAttribute{
											Computed:            true,
											MarkdownDescription: "The literal or reference parameter value",
											Attributes: map[string]schema.Attribute{
												"literal": schema.StringAttribute{
													Computed:            true,
													MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
												},
												"reference": schema.StringAttribute{
													Computed:            true,
													MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"jira_options": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "jira_options"),
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"project_ids": schema.ListAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: apischema.Docstring("AlertSourceJiraOptionsV2", "project_ids"),
					},
				},
			},
			"email_address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceEmailOptionsV2", "email_address"),
			},
		},
	}
}
