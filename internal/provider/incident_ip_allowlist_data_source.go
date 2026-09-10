package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentIPAllowlistDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentIPAllowlistDataSource{}
)

func NewIncidentIPAllowlistDataSource() datasource.DataSource {
	return &IncidentIPAllowlistDataSource{}
}

type IncidentIPAllowlistDataSource struct {
	dataSourceConfigurer
}

type IncidentIPAllowlistDataSourceModel struct {
	Allowlist []IncidentIPAllowlistItemModel `tfsdk:"allowlist"`
	Enabled   types.Bool                     `tfsdk:"enabled"`
	UpdatedAt timetypes.RFC3339              `tfsdk:"updated_at"`
	Version   types.Int64                    `tfsdk:"version"`
}

type IncidentIPAllowlistItemModel struct {
	Label types.String `tfsdk:"label"`
	Value types.String `tfsdk:"value"`
}

func (d *IncidentIPAllowlistDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_allowlist"
}

func (d *IncidentIPAllowlistDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("IPAllowlists V1"),
			"Use this data source to read the organisation's IP allowlist: whether it is enabled, and the IP addresses or CIDR prefixes that are allowed to reach the dashboard, public API and mobile app."),
		Attributes: map[string]schema.Attribute{
			"allowlist": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IPAllowlistV1", "allowlist"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"label": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IPAllowlistItemV1", "label"),
						},
						"value": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IPAllowlistItemV1", "value"),
						},
					},
				},
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IPAllowlistV1", "enabled"),
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("IPAllowlistV1", "updated_at"),
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IPAllowlistV1", "version"),
			},
		},
	}
}

func (d *IncidentIPAllowlistDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentIPAllowlistDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.IPAllowlistsV1ShowIPAllowlistWithResponse(ctx)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("%s", result.Body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read IP allowlist, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read IP allowlist, unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentIPAllowlistFromAPI(result.JSON200.IpAllowlist))...)
}

func incidentIPAllowlistFromAPI(allowlist client.IPAllowlistV1) IncidentIPAllowlistDataSourceModel {
	items := make([]IncidentIPAllowlistItemModel, 0, len(allowlist.Allowlist))
	for _, item := range allowlist.Allowlist {
		items = append(items, IncidentIPAllowlistItemModel{
			Label: types.StringPointerValue(item.Label),
			Value: types.StringValue(item.Value),
		})
	}

	return IncidentIPAllowlistDataSourceModel{
		Allowlist: items,
		Enabled:   types.BoolValue(allowlist.Enabled),
		UpdatedAt: timetypes.NewRFC3339TimePointerValue(allowlist.UpdatedAt),
		Version:   types.Int64Value(allowlist.Version),
	}
}
