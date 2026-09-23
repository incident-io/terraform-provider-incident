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
)

var (
	_ datasource.DataSource                   = &IncidentAnnouncementRuleDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentAnnouncementRuleDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentAnnouncementRuleDataSource{}
)

func NewIncidentAnnouncementRuleDataSource() datasource.DataSource {
	return &IncidentAnnouncementRuleDataSource{}
}

type IncidentAnnouncementRuleDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentAnnouncementRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_announcement_rule"
}

func (d *IncidentAnnouncementRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Announcement Rules V2"),
			"Use this data source to look up an existing announcement rule, either by `id` or by `name`, "+
				"without managing it as an `incident_announcement_rule` resource. Set exactly one of the two "+
				"lookup attributes; setting both, or neither, is rejected at plan time."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"condition_groups": models.ConditionGroupsDataSourceAttribute(),
			"slack_channel_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "slack_channel_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"microsoft_teams_channel_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "microsoft_teams_channel_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"update_sharing_mode": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("AnnouncementRuleV2", "update_sharing_mode"),
				Computed:            true,
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("AnnouncementRuleV2", "mode"),
				Computed:            true,
			},
			"private_incident_scope": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("AnnouncementRuleV2", "private_incident_scope"),
				Computed:            true,
			},
			"conditions_no_longer_apply_behaviour": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("AnnouncementRuleV2", "conditions_no_longer_apply_behaviour"),
				Computed:            true,
			},
			"template_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "template_id"),
				Computed:            true,
			},
			"owning_team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "owning_team_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "created_at"),
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "updated_at"),
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional
// and Computed so either can be used, which means setting both would otherwise silently
// ignore one of them.
func (d *IncidentAnnouncementRuleDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.AnnouncementRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// An unknown value, such as an id from a resource created in the same apply, is
	// non-null, so judging it here would reject a config that's fine.
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

func (d *IncidentAnnouncementRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.AnnouncementRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rule *client.AnnouncementRuleV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.AnnouncementRulesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement rule, got error: %s", err))
			return
		}

		rule = &result.JSON200.AnnouncementRule

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement rule by name, got error: %s", err))
			return
		}

		rule = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	model := models.AnnouncementRuleModel{}.FromAPI(*rule, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// announcementRuleListPageSize is the largest page the list endpoint accepts.
const announcementRuleListPageSize = 250

// findByName looks for an exact name match in the rule list, and requires exactly one. The
// endpoint has no name filter, so every page has to be fetched to know a name doesn't
// appear on a later one. Nothing stops two rules sharing a name, so more than one match is
// reported rather than resolved arbitrarily.
func (d *IncidentAnnouncementRuleDataSource) findByName(ctx context.Context, name string) (*client.AnnouncementRuleV2, error) {
	var (
		after   *string
		matches []client.AnnouncementRuleV2
	)

	for {
		result, err := d.client.AnnouncementRulesV2ListWithResponse(ctx, &client.AnnouncementRulesV2ListParams{
			PageSize: lo.ToPtr(int64(announcementRuleListPageSize)),
			After:    after,
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing announcement rules: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.AnnouncementRules, func(rule client.AnnouncementRuleV2, _ int) bool {
			return rule.Name == name
		})...)

		// The endpoint returns an after cursor whenever a page fills, so an absent one ends
		// the walk rather than looping on the last page forever.
		if result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no announcement rule found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d announcement rules named %q; look it up by id instead", len(matches), name)
	}
}
