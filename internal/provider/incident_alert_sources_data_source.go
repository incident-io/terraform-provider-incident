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
	_ datasource.DataSource              = &IncidentAlertSourcesDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentAlertSourcesDataSource{}
)

func NewIncidentAlertSourcesDataSource() datasource.DataSource {
	return &IncidentAlertSourcesDataSource{}
}

type IncidentAlertSourcesDataSource struct {
	client *client.ClientWithResponses
}

type IncidentAlertSourcesDataSourceModel struct {
	ID           types.String                              `tfsdk:"id"`
	Name         types.String                              `tfsdk:"name"`
	SourceType   types.String                              `tfsdk:"source_type"`
	AlertSources []IncidentAlertSourcesDataSourceItemModel `tfsdk:"alert_sources"`
}

type IncidentAlertSourcesDataSourceItemModel struct {
	ID           types.String                        `tfsdk:"id"`
	Name         types.String                        `tfsdk:"name"`
	SourceType   types.String                        `tfsdk:"source_type"`
	SecretToken  types.String                        `tfsdk:"secret_token"`
	Template     *models.AlertTemplateModel          `tfsdk:"template"`
	JiraOptions  *models.AlertSourceJiraOptionsModel `tfsdk:"jira_options"`
	EmailAddress types.String                        `tfsdk:"email_address"`
}

func (d *IncidentAlertSourcesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *IncidentAlertSourcesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_sources"
}

func (d *IncidentAlertSourcesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentAlertSourcesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get all alert sources
	result, err := d.client.AlertSourcesV2ListWithResponse(ctx)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("%s", string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list alert sources, got error: %s", err))
		return
	}

	// Filter alert sources based on provided criteria
	var filteredSources []client.AlertSourceV2
	for _, source := range result.JSON200.AlertSources {
		// Apply filters if they are provided
		if !data.ID.IsNull() && source.Id != data.ID.ValueString() {
			continue
		}
		if !data.Name.IsNull() && source.Name != data.Name.ValueString() {
			continue
		}
		if !data.SourceType.IsNull() && string(source.SourceType) != data.SourceType.ValueString() {
			continue
		}

		filteredSources = append(filteredSources, source)
	}

	// Convert filtered alert sources to the model
	var alertSources []IncidentAlertSourcesDataSourceItemModel
	for _, source := range filteredSources {
		alertSources = append(alertSources, *d.buildItemModel(source))
	}

	modelResp := IncidentAlertSourcesDataSourceModel{
		ID:           data.ID,
		Name:         data.Name,
		SourceType:   data.SourceType,
		AlertSources: alertSources,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

func (d *IncidentAlertSourcesDataSource) buildItemModel(source client.AlertSourceV2) *IncidentAlertSourcesDataSourceItemModel {
	var emailAddress *string
	if source.EmailOptions != nil {
		emailAddress = &source.EmailOptions.EmailAddress
	}

	return &IncidentAlertSourcesDataSourceItemModel{
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

func (d *IncidentAlertSourcesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about alert sources in incident.io. You can retrieve all alert sources or filter by specific criteria such as ID, name, or source type.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter alert sources by ID. If provided, only the alert source with this ID will be returned.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter alert sources by name. If provided, only alert sources with this name will be returned.",
			},
			"source_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter alert sources by source type (e.g., 'webhook', 'email', 'jira'). If provided, only alert sources of this type will be returned.",
			},
			"alert_sources": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of alert sources matching the specified criteria. If no filters are provided, all alert sources are returned.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("AlertSourceV2", "id"),
						},
						"name": schema.StringAttribute{
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
				},
			},
		},
	}
}
