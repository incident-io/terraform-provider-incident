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
	_ datasource.DataSource                   = &IncidentTimestampDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentTimestampDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentTimestampDataSource{}
)

func NewIncidentTimestampDataSource() datasource.DataSource {
	return &IncidentTimestampDataSource{}
}

type IncidentTimestampDataSource struct {
	dataSourceConfigurer
}

type IncidentTimestampDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Rank types.Int64  `tfsdk:"rank"`
}

func (d *IncidentTimestampDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_timestamp"
}

func (d *IncidentTimestampDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Incident Timestamps V2"),
			"Use this data source to look up an existing incident timestamp, either by `id` or by `name`. "+
				"Timestamps can't be created from Terraform, so this is how you get hold of the ID of one "+
				"incident.io sets for you, such as Reported or Closed, or one configured in your settings."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTimestampV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentTimestampV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("IncidentTimestampV2", "rank"),
				Computed:            true,
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are
// Optional and Computed so either can be used, which means setting both would
// otherwise silently ignore one of them.
func (d *IncidentTimestampDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *IncidentTimestampDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — either attribute behind an unresolved
	// conditional, say — is non-null, so judging it here would reject a config
	// that's actually fine.
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

func (d *IncidentTimestampDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentTimestampDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var timestamp *client.IncidentTimestampV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.IncidentTimestampsV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident timestamp, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident timestamp, unexpected response: %s", result.Status()))
			return
		}

		timestamp = &result.JSON200.IncidentTimestamp

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident timestamp by name, got error: %s", err))
			return
		}

		timestamp = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	modelResp := &IncidentTimestampDataSourceModel{
		ID:   types.StringValue(timestamp.Id),
		Name: types.StringValue(timestamp.Name),
		Rank: types.Int64Value(timestamp.Rank),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

// findByName looks for an exact name match in the timestamp list, and requires
// exactly one. The list endpoint has no name filter and isn't paginated, so
// every timestamp comes back in a single request.
func (d *IncidentTimestampDataSource) findByName(ctx context.Context, name string) (*client.IncidentTimestampV2, error) {
	result, err := d.client.IncidentTimestampsV2ListWithResponse(ctx)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("%s", result.Body)
	}
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response listing incident timestamps: %s", result.Status())
	}

	matches := lo.Filter(result.JSON200.IncidentTimestamps, func(timestamp client.IncidentTimestampV2, _ int) bool {
		return timestamp.Name == name
	})

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no incident timestamp found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d incident timestamps named %q; look it up by id instead", len(matches), name)
	}
}
