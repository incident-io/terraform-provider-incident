package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentAlertSourceDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentAlertSourceDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentAlertSourceDataSource{}
)

func NewIncidentAlertSourceDataSource() datasource.DataSource {
	return &IncidentAlertSourceDataSource{}
}

type IncidentAlertSourceDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentAlertSourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source"
}

// Schema mirrors incident_alert_source's, with everything Computed but the two lookup
// fields: a data source says what the API holds rather than what an author asked for, and a
// configuration reading one should be able to use the same attribute paths it writes.
//
// The attribute bindings are not here. A source doesn't hold them — each is its own object,
// and its own incident_alert_source_attribute resource and data source — so returning them
// would mean a call per source and a shape the resource doesn't have.
func (d *IncidentAlertSourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "id"),
		},
		"name": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "name"),
		},
		"source_type": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: EnumValuesDescription("AlertSourceV3", "source_type"),
		},
		"secret_token": schema.StringAttribute{
			Computed:            true,
			Sensitive:           true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "secret_token"),
		},
		"alert_events_url": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "alert_events_url"),
		},
		"email_address": schema.StringAttribute{
			Computed: true,
			// The source carries this at the top level, but the API documents it on the
			// email options it comes from.
			MarkdownDescription: apischema.Docstring("AlertSourceEmailOptionsV3", "email_address"),
		},
		"owning_team_ids": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "owning_team_ids"),
		},
		"is_private": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "is_private"),
		},
		"fixed_team_id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "fixed_team_id"),
		},
		"title": models.TemplatedTextDataSourceAttribute(
			apischema.Docstring("AlertSourceV3", "title"), "plain_single_line"),
		"description": models.TemplatedTextDataSourceAttribute(
			apischema.Docstring("AlertSourceV3", "description"), "rich"),

		"priority":         models.BindingDataSourceAttribute(apischema.Docstring("AlertSourceV3", "priority")),
		"visible_to_teams": models.BindingDataSourceAttribute(apischema.Docstring("AlertSourceV3", "visible_to_teams")),

		"jira_options":        jiraOptionsDataSourceAttribute(),
		"heartbeat_options":   heartbeatOptionsDataSourceAttribute(),
		"email_options":       emailOptionsDataSourceAttribute(),
		"http_custom_options": httpCustomOptionsDataSourceAttribute(),
		"rate_limit_sharding": rateLimitShardingDataSourceAttribute(),

		"filter_condition_groups": models.ConditionGroupsDataSourceAttribute(),

		"auto_resolve_timeout_minutes": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "auto_resolve_timeout_minutes"),
		},
		"auto_resolve_incident_alerts": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "auto_resolve_incident_alerts"),
		},
		"disabled": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "disabled"),
		},
		"version": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceV3", "version"),
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an alert source by `id` or `name`, in the same shape as the " +
			"`incident_alert_source` resource. Exactly one lookup field should be set.\n\n" +
			"The attributes a source populates are not returned here: each is its own object, " +
			"read with the `incident_alert_source_attribute` data source.",
		Attributes: attributes,
		Blocks: map[string]schema.Block{
			"named_expression": models.NamedExpressionBlockDataSource(),
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional and
// Computed so either can be used, which means setting both would otherwise silently ignore
// one of them.
func (d *IncidentAlertSourceDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var id, name types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the same apply,
	// or either attribute behind an unresolved conditional — is non-null, so judging it here
	// would reject a config that's actually fine.
	if id.IsUnknown() || name.IsUnknown() {
		return
	}

	switch {
	case !id.IsNull() && !name.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case id.IsNull() && name.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
	}
}

func (d *IncidentAlertSourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data alertSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	source := d.lookup(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// The config is the prior the projection reconciles against, which keeps the author's
	// spelling of a binding where the API returns an equivalent one. A data source's config
	// holds only the lookup field, so in practice this reconciles against nothing and the
	// API's spelling is what lands — which is correct for a data source.
	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceFromAPI(*source, &data, &resp.Diagnostics))...)
}

// lookup finds the source by whichever field is set. There is no list endpoint that filters,
// so a name lookup reads every source and matches here — which is what a configuration had
// to do by hand before this data source took a name.
func (d *IncidentAlertSourceDataSource) lookup(
	ctx context.Context, data alertSourceModel, diags *diag.Diagnostics,
) *client.AlertSourceV3 {
	if !data.ID.IsNull() {
		result, err := d.client.AlertSourcesV3ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to read alert source, got error: %s", err))
			return nil
		}
		if result.JSON200 == nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to read alert source, unexpected response: %s", result.Status()))
			return nil
		}

		return &result.JSON200.AlertSource
	}

	result, err := d.client.AlertSourcesV3ListWithResponse(ctx)
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to list alert sources, got error: %s", err))
		return nil
	}
	if result.JSON200 == nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to list alert sources, unexpected response: %s", result.Status()))
		return nil
	}

	name := data.Name.ValueString()
	var found *client.AlertSourceV3
	for idx, source := range result.JSON200.AlertSources {
		if source.Name != name {
			continue
		}
		// Names aren't unique, so a second match means the lookup doesn't identify one
		// source: better to say so than to return whichever came back first.
		if found != nil {
			diags.AddError(
				"Ambiguous lookup",
				fmt.Sprintf("More than one alert source is called %q. Look it up by id instead.", name),
			)
			return nil
		}
		found = &result.JSON200.AlertSources[idx]
	}

	if found == nil {
		diags.AddError("Not Found", fmt.Sprintf("Unable to find alert source with name: %s", name))
		return nil
	}

	return found
}
