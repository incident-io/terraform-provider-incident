package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/samber/lo"

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
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list alert sources, got error: %s", err))
		return
	}

	// Filter alert sources based on provided criteria
	var filteredSources []client.AlertSourceV2
	for _, source := range result.JSON200.AlertSources {
		// Apply filters if they are provided
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

	var visibleToTeams *models.IncidentEngineParamBinding
	if source.Template.VisibleToTeams != nil {
		visibleToTeams = lo.ToPtr(models.IncidentEngineParamBinding{}.FromAPI(*source.Template.VisibleToTeams))
	}

	return &IncidentAlertSourcesDataSourceItemModel{
		ID:          types.StringValue(source.Id),
		Name:        types.StringValue(source.Name),
		SourceType:  types.StringValue(string(source.SourceType)),
		SecretToken: types.StringPointerValue(source.SecretToken),
		Template: &models.AlertTemplateModel{
			Title:          models.IncidentEngineParamBindingValue{}.FromAPI(source.Template.Title),
			Description:    models.IncidentEngineParamBindingValue{}.FromAPI(source.Template.Description),
			Attributes:     models.AlertTemplateAttributesModel{}.FromAPI(source.Template.Attributes),
			Expressions:    models.IncidentEngineExpressions{}.FromAPI(source.Template.Expressions),
			IsPrivate:      types.BoolValue(source.Template.IsPrivate),
			VisibleToTeams: visibleToTeams,
		},
		JiraOptions:  models.AlertSourceJiraOptionsModel{}.FromAPI(source.JiraOptions),
		EmailAddress: types.StringPointerValue(emailAddress),
	}
}

func (d *IncidentAlertSourcesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Alert Sources V2"),
		Attributes: map[string]schema.Attribute{
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
							Sensitive:           true,
						},
						"template": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("AlertSourceV2", "template"),
							Attributes: map[string]schema.Attribute{
								"expressions": models.ExpressionsDataSourceAttribute(),
								"title": schema.SingleNestedAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "title"),
									Attributes:          models.ParamBindingValueDataSourceAttributes(),
								},
								"description": schema.SingleNestedAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "description"),
									Attributes:          models.ParamBindingValueDataSourceAttributes(),
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
													"array_value": schema.ListNestedAttribute{
														Computed:            true,
														MarkdownDescription: "The array of literal or reference parameter values",
														NestedObject: schema.NestedAttributeObject{
															Attributes: models.ParamBindingValueDataSourceAttributes(),
														},
													},
													"value": schema.SingleNestedAttribute{
														Computed:            true,
														MarkdownDescription: "The literal or reference parameter value",
														Attributes:          models.ParamBindingValueDataSourceAttributes(),
													},
													"merge_strategy": schema.StringAttribute{
														Computed:            true,
														MarkdownDescription: "Merge strategy for this attribute when alert updates",
													},
												},
											},
										},
									},
								},
								"is_private": schema.BoolAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplateV2", "is_private"),
								},
								"visible_to_teams": schema.SingleNestedAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplateV2", "visible_to_teams"),
									Attributes:          models.ParamBindingDataSourceAttributes(),
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
