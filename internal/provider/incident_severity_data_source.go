package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var (
	_ datasource.DataSource                   = &IncidentSeverityDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentSeverityDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentSeverityDataSource{}
)

func NewIncidentSeverityDataSource() datasource.DataSource {
	return &IncidentSeverityDataSource{}
}

type IncidentSeverityDataSource struct {
	dataSourceConfigurer
}

type IncidentSeverityDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Rank        types.Int64  `tfsdk:"rank"`
}

func (d *IncidentSeverityDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_severity"
}

func (d *IncidentSeverityDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Severities V1"),
			"Use this data source to look up an existing incident severity, either by `id` or by `name`."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SeverityV1", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SeverityV1", "name"),
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SeverityV1", "description"),
				Computed:            true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("SeverityV1", "rank"),
				Computed:            true,
			},
		},
	}
}

func (d *IncidentSeverityDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *IncidentSeverityDataSourceModel
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

func (d *IncidentSeverityDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentSeverityDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var severity *client.SeverityV1
	switch {
	case !data.ID.IsNull():
		result, err := d.client.SeveritiesV1ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident severity, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident severity, unexpected response: %s", result.Status()))
			return
		}

		severity = &result.JSON200.Severity

	case !data.Name.IsNull():
		found, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident severity by name, got error: %s", err))
			return
		}

		severity = found

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	modelResp := &IncidentSeverityDataSourceModel{
		ID:          types.StringValue(severity.Id),
		Name:        types.StringValue(severity.Name),
		Description: types.StringValue(severity.Description),
		Rank:        types.Int64Value(severity.Rank),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

func (d *IncidentSeverityDataSource) findByName(ctx context.Context, name string) (*client.SeverityV1, error) {
	result, err := d.client.SeveritiesV1ListWithResponse(ctx)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("%s", result.Body)
	}
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response listing incident severities: %s", result.Status())
	}

	matches := lo.Filter(result.JSON200.Severities, func(severity client.SeverityV1, _ int) bool {
		return severity.Name == name
	})

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no incident severity found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d incident severities named %q; look it up by id instead", len(matches), name)
	}
}
