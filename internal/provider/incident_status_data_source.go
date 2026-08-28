package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ datasource.DataSource                   = &IncidentStatusDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentStatusDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentStatusDataSource{}
)

func NewIncidentStatusDataSource() datasource.DataSource {
	return &IncidentStatusDataSource{}
}

type IncidentStatusDataSource struct {
	client *client.ClientWithResponses
}

type IncidentStatusDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Category    types.String `tfsdk:"category"`
	Rank        types.Int64  `tfsdk:"rank"`
}

func (d *IncidentStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status"
}

func (d *IncidentStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *IncidentStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Incident Statuses V1"),
			"Use this data source to look up an existing incident status, either by `id` or by `name`. "+
				"This is useful for referencing statuses that incident.io manages for you, such as Triage or Closed, "+
				"which can't be created as an `incident_status` resource."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "name"),
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "description"),
				Computed:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("IncidentStatusV1", "category"),
				Computed:            true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "rank"),
				Computed:            true,
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are
// Optional and Computed so either can be used, which means setting both would
// otherwise silently ignore one of them.
func (d *IncidentStatusDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *IncidentStatusDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the
	// same apply, or either attribute behind an unresolved conditional — is
	// non-null, so judging it here would reject a config that's actually fine.
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

func (d *IncidentStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentStatusDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var status *client.IncidentStatusV1
	switch {
	case !data.ID.IsNull():
		result, err := d.client.IncidentStatusesV1ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident status, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident status, unexpected response: %s", result.Status()))
			return
		}

		status = &result.JSON200.IncidentStatus

	case !data.Name.IsNull():
		result, err := d.client.IncidentStatusesV1ListWithResponse(ctx)
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list incident statuses, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list incident statuses, unexpected response: %s", result.Status()))
			return
		}

		name := data.Name.ValueString()
		found, ok := lo.Find(result.JSON200.IncidentStatuses, func(status client.IncidentStatusV1) bool {
			return status.Name == name
		})
		if !ok {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find incident status with name: %s", name))
			return
		}

		status = &found

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	modelResp := &IncidentStatusDataSourceModel{
		ID:          types.StringValue(status.Id),
		Name:        types.StringValue(status.Name),
		Description: types.StringValue(status.Description),
		Category:    types.StringValue(string(status.Category)),
		Rank:        types.Int64Value(status.Rank),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}
