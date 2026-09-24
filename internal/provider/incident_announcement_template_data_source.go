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
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentAnnouncementTemplateDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentAnnouncementTemplateDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentAnnouncementTemplateDataSource{}
)

func NewIncidentAnnouncementTemplateDataSource() datasource.DataSource {
	return &IncidentAnnouncementTemplateDataSource{}
}

type IncidentAnnouncementTemplateDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentAnnouncementTemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_announcement_template"
}

func (d *IncidentAnnouncementTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Announcement Templates V2"),
			"Use this data source to look up an existing announcement template, either by `id` or by "+
				"`name`, without managing it as an `incident_announcement_template` resource. Set exactly one "+
				"of the two lookup attributes; setting both, or neither, is rejected at plan time.\n\n"+
				"To find the organisation's default template, look it up by name and check `is_default`."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "is_default"),
				Computed:            true,
			},
			"fields": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "fields"),
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field_type": schema.StringAttribute{
							MarkdownDescription: EnumValuesDescription("AnnouncementTemplateFieldV2", "field_type"),
							Computed:            true,
						},
						"emoji": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldV2", "emoji"),
							Computed:            true,
						},
						"custom_field_id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldV2", "custom_field_id"),
							Computed:            true,
						},
						"incident_role_id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldV2", "incident_role_id"),
							Computed:            true,
						},
						"incident_timestamp_id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldV2", "incident_timestamp_id"),
							Computed:            true,
						},
						"rich_text": schema.SingleNestedAttribute{
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldV2", "rich_text"),
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"type": schema.StringAttribute{
									MarkdownDescription: EnumValuesDescription("AnnouncementTemplateRichTextV2", "type"),
									Computed:            true,
								},
								"contents": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("AnnouncementTemplateRichTextV2", "contents"),
									Computed:            true,
								},
							},
						},
					},
				},
			},
			"actions": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "actions"),
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action_type": schema.StringAttribute{
							MarkdownDescription: EnumValuesDescription("AnnouncementTemplateActionV2", "action_type"),
							Computed:            true,
						},
						"emoji": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateActionV2", "emoji"),
							Computed:            true,
						},
					},
				},
			},
			"owning_team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "owning_team_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional
// and Computed so either can be used, which means setting both would otherwise silently
// ignore one of them.
func (d *IncidentAnnouncementTemplateDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.AnnouncementTemplateModel
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

func (d *IncidentAnnouncementTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.AnnouncementTemplateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var template *client.AnnouncementTemplateV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.AnnouncementTemplatesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement template, got error: %s", err))
			return
		}

		template = &result.JSON200.AnnouncementTemplate

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement template by name, got error: %s", err))
			return
		}

		template = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	model := models.AnnouncementTemplateModel{}.FromAPI(*template)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// findByName looks for an exact name match in the template list. The list isn't paginated,
// as an organisation has a capped number of templates, and names are unique.
func (d *IncidentAnnouncementTemplateDataSource) findByName(ctx context.Context, name string) (*client.AnnouncementTemplateV2, error) {
	result, err := d.client.AnnouncementTemplatesV2ListWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response listing announcement templates: %s", result.Status())
	}

	template, ok := lo.Find(result.JSON200.AnnouncementTemplates, func(template client.AnnouncementTemplateV2) bool {
		return template.Name == name
	})
	if !ok {
		return nil, fmt.Errorf("no announcement template found with name %q", name)
	}

	return &template, nil
}
