package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentIncidentFormDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentIncidentFormDataSource{}
)

func NewIncidentIncidentFormDataSource() datasource.DataSource {
	return &IncidentIncidentFormDataSource{}
}

type IncidentIncidentFormDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentIncidentFormDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_form"
}

// Schema mirrors the incident_incident_form resource with everything but the ID computed.
// Read shares the resource's model, so every attribute there needs one here or State.Set
// fails on every read.
func (d *IncidentIncidentFormDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Incident Forms V3"),
			"Use this data source to read an existing incident form by ID, including its elements and expressions, "+
				"without managing it as an `incident_incident_form` resource. The ID of a form is in the dashboard's URL "+
				"when editing it. Escalate forms are not available through this data source."),
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardDataSourceAttribute(),
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("IncidentFormV3", "id"),
			},
			"form_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("IncidentFormV3", "form_type"),
			},
			"incident_type_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IncidentFormV3", "incident_type_id"),
			},
			"expressions": models.ExpressionsDataSourceAttribute(),
			"lifecycle_elements": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IncidentFormV3", "lifecycle_elements"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "id"),
						},
						"element_type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: EnumValuesDescription("IncidentFormLifecycleElementV3", "element_type"),
						},
						"custom_field_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "custom_field_id"),
						},
						"incident_role_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "incident_role_id"),
						},
						"incident_timestamp_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "incident_timestamp_id"),
						},
						"show_if_condition_groups": models.ConditionGroupsDataSourceAttribute(),
						"required_if": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: EnumValuesDescription("IncidentFormLifecycleElementV3", "required_if"),
						},
						"required_if_condition_groups": models.ConditionGroupsDataSourceAttribute(),
						"default_value": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "default_value"),
							Attributes:          models.ParamBindingDataSourceAttributes(),
						},
						"placeholder": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "placeholder"),
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "description"),
						},
						"can_select_no_value": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "can_select_no_value"),
						},
						"config": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "config"),
							Attributes: map[string]schema.Attribute{
								"require_comment": schema.BoolAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementConfigV3", "require_comment"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *IncidentIncidentFormDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.IncidentFormResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.IncidentFormsV3ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident form, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to read incident form: unexpected response from API (status %s)", result.Status()))
		return
	}

	// There is no prior state to reconcile against, so the model is whatever the API has.
	model := models.IncidentFormResourceModel{}.FromAPI(result.JSON200.IncidentForm, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
