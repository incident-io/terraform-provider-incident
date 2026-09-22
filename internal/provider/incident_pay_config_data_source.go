package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/timestamptypes"
)

var (
	_ datasource.DataSource                   = &IncidentPayConfigDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentPayConfigDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentPayConfigDataSource{}
)

func NewIncidentPayConfigDataSource() datasource.DataSource {
	return &IncidentPayConfigDataSource{}
}

type IncidentPayConfigDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentPayConfigDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pay_config"
}

func (d *IncidentPayConfigDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Pay Configs V2"),
			"Use this data source to look up an existing pay config, either by `id` or by `name`, without "+
				"managing it as an `incident_pay_config` resource. Set exactly one of the two lookup attributes; "+
				"setting both, or neither, is rejected at plan time.\n\nA lookup sees published configs and "+
				"drafts created through the API. A draft somebody created in the dashboard is private to them "+
				"until a report priced against it is published, so it cannot be found here."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"timezone": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "timezone"),
				Computed:            true,
			},
			"currency": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "currency"),
				Computed:            true,
			},
			"base_rate_cents": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "base_rate_cents"),
				Computed:            true,
			},
			"rate_time_unit": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("PayConfigV2", "rate_time_unit"),
				Computed:            true,
			},
			"weekly_rules": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "weekly_rules") + ". The first rule that covers a shift prices it.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "id"),
							Computed:            true,
						},
						"weekdays": schema.SetAttribute{
							MarkdownDescription: DescribeEnumValues(apischema.Docstring("PayConfigWeeklyRuleV2", "weekdays"), "PayConfigWeeklyRuleV2", "weekdays"),
							Computed:            true,
							ElementType:         types.StringType,
						},
						"start_time": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "start_time") + ", as `HH:MM`.",
							Computed:            true,
						},
						"end_time": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "end_time") + " Written as `HH:MM`.",
							Computed:            true,
						},
						"rate_cents": schema.Int64Attribute{
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "rate_cents"),
							Computed:            true,
						},
					},
				},
			},
			"one_off_rules": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "one_off_rules") + ". These take precedence over the weekly rules.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "id"),
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "name"),
							Computed:            true,
						},
						"start_at": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "start_at"),
							Computed:            true,
							CustomType:          timestamptypes.InstantType{},
						},
						"end_at": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "end_at"),
							Computed:            true,
							CustomType:          timestamptypes.InstantType{},
						},
						"rate_cents": schema.Int64Attribute{
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "rate_cents"),
							Computed:            true,
						},
					},
				},
			},
			"published_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PayConfigV2", "published_at") + " Null until then.",
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
			"created_at": schema.StringAttribute{
				// The API schema has no description for either timestamp, so these are the
				// provider's own words rather than apischema.Docstring.
				MarkdownDescription: "When this pay config was created.",
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "When this pay config, or one of its rules, was last changed.",
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional
// and Computed so either can be used, which means setting both would otherwise silently
// ignore one of them.
func (d *IncidentPayConfigDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.PayConfigModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the same apply,
	// or either attribute behind an unresolved conditional — is non-null, so judging it here
	// would reject a config that's actually fine.
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

func (d *IncidentPayConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.PayConfigModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var config *client.PayConfigV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.PayConfigsV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read pay config, got error: %s", err))
			return
		}

		config = &result.JSON200.PayConfig

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read pay config by name, got error: %s", err))
			return
		}

		config = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	model := models.PayConfigModel{}.FromAPI(*config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// payConfigListPageSize is the largest page the list endpoint accepts.
const payConfigListPageSize = 250

// findByName looks for an exact name match in the pay config list, and requires exactly
// one. The endpoint has no name filter, so every page has to be fetched to know a name
// doesn't appear on a later one. Nothing stops two configs sharing a name, so more than
// one match is reported rather than resolved arbitrarily.
func (d *IncidentPayConfigDataSource) findByName(ctx context.Context, name string) (*client.PayConfigV2, error) {
	var (
		after   *string
		matches []client.PayConfigV2
	)

	for {
		result, err := d.client.PayConfigsV2ListWithResponse(ctx, &client.PayConfigsV2ListParams{
			PageSize: lo.ToPtr(int64(payConfigListPageSize)),
			After:    after,
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing pay configs: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.PayConfigs, func(config client.PayConfigV2, _ int) bool {
			return config.Name == name
		})...)

		// The endpoint returns an after cursor only while another page may exist, so an
		// absent one ends the walk rather than looping on the last page forever.
		if result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no pay config found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d pay configs named %q; look it up by id instead", len(matches), name)
	}
}
