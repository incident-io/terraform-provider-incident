package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var (
	_ datasource.DataSource                   = &IncidentIncidentTypeDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentIncidentTypeDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentIncidentTypeDataSource{}
)

func NewIncidentIncidentTypeDataSource() datasource.DataSource {
	return &IncidentIncidentTypeDataSource{}
}

type IncidentIncidentTypeDataSource struct {
	dataSourceConfigurer
}

type IncidentIncidentTypeDataSourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	CreateInTriage       types.String `tfsdk:"create_in_triage"`
	IsDefault            types.Bool   `tfsdk:"is_default"`
	PrivateIncidentsOnly types.Bool   `tfsdk:"private_incidents_only"`
	OwningTeamIDs        types.Set    `tfsdk:"owning_team_ids"`
}

func (d *IncidentIncidentTypeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_type"
}

func (d *IncidentIncidentTypeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Incident Types V1"),
			"Use this data source to look up an existing incident type, either by `id` or by `name`.\n\n"+
				"Incident types can't be created from Terraform - they're configured in your settings - so this is "+
				"how you get hold of one's ID, such as for an alert route or a workflow that only applies to incidents "+
				"of one type."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTypeV1", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTypeV1", "name"),
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTypeV1", "description"),
				Computed:            true,
			},
			"create_in_triage": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("IncidentTypeV1", "create_in_triage"),
				Computed:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTypeV1", "is_default"),
				Computed:            true,
			},
			"private_incidents_only": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTypeV1", "private_incidents_only"),
				Computed:            true,
			},
			"owning_team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTypeV1", "owning_team_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *IncidentIncidentTypeDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *IncidentIncidentTypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	if data.ID.IsUnknown() || data.Name.IsUnknown() {
		return
	}

	switch {
	case !data.ID.IsNull() && !data.Name.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case data.ID.IsNull() && data.Name.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
	}
}

func (d *IncidentIncidentTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentIncidentTypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var incidentType *client.IncidentTypeV1
	switch {
	case !data.ID.IsNull():
		result, err := d.client.IncidentTypesV1ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident type, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident type, unexpected response: %s", result.Status()))
			return
		}

		incidentType = &result.JSON200.IncidentType

	case !data.Name.IsNull():
		found, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident type by name, got error: %s", err))
			return
		}

		incidentType = found
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, d.buildModel(ctx, *incidentType, &resp.Diagnostics))...)
}

// findByName looks an incident type up by name, which the API has no endpoint for, so it
// lists them and filters. Names are not unique as far as the API is concerned, so a name
// matching more than one type is reported rather than resolved arbitrarily.
func (d *IncidentIncidentTypeDataSource) findByName(ctx context.Context, name string) (*client.IncidentTypeV1, error) {
	result, err := d.client.IncidentTypesV1ListWithResponse(ctx)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("%s", result.Body)
	}
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response listing incident types: %s", result.Status())
	}

	matches := lo.Filter(result.JSON200.IncidentTypes, func(incidentType client.IncidentTypeV1, _ int) bool {
		return incidentType.Name == name
	})

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no incident type found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d incident types named %q; look it up by id instead", len(matches), name)
	}
}

func (d *IncidentIncidentTypeDataSource) buildModel(ctx context.Context, incidentType client.IncidentTypeV1, diags *diag.Diagnostics) *IncidentIncidentTypeDataSourceModel {
	owningTeamIDs := types.SetNull(types.StringType)
	if incidentType.OwningTeamIds != nil {
		set, setDiags := types.SetValueFrom(ctx, types.StringType, *incidentType.OwningTeamIds)
		diags.Append(setDiags...)
		owningTeamIDs = set
	}

	return &IncidentIncidentTypeDataSourceModel{
		ID:                   types.StringValue(incidentType.Id),
		Name:                 types.StringValue(incidentType.Name),
		Description:          types.StringValue(incidentType.Description),
		CreateInTriage:       types.StringValue(string(incidentType.CreateInTriage)),
		IsDefault:            types.BoolValue(incidentType.IsDefault),
		PrivateIncidentsOnly: types.BoolValue(incidentType.PrivateIncidentsOnly),
		OwningTeamIDs:        owningTeamIDs,
	}
}
