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
	_ datasource.DataSource                   = &IncidentAlertRouteDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentAlertRouteDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentAlertRouteDataSource{}
)

func NewIncidentAlertRouteDataSource() datasource.DataSource {
	return &IncidentAlertRouteDataSource{}
}

type IncidentAlertRouteDataSource struct {
	dataSourceConfigurer
}

// alertRouteDataSourceModel is an alert route in the shape incident_alert_route writes it
// with grouping_config set. The resource's model also carries the deprecated attributes of
// its earlier shape, which the current API never fills, so rather than report a block of
// nulls this model keeps only the current ones.
type alertRouteDataSourceModel struct {
	ID               types.String                             `tfsdk:"id"`
	Name             types.String                             `tfsdk:"name"`
	Enabled          types.Bool                               `tfsdk:"enabled"`
	IsPrivate        types.Bool                               `tfsdk:"is_private"`
	AlertSources     []models.AlertRouteAlertSourceModel      `tfsdk:"alert_sources"`
	ConditionGroups  models.IncidentEngineConditionGroups     `tfsdk:"condition_groups"`
	Expressions      models.IncidentEngineExpressions         `tfsdk:"expressions"`
	EscalationConfig *models.AlertRouteEscalationConfigModel  `tfsdk:"escalation_config"`
	GroupingConfig   *models.AlertRouteV3GroupingConfigModel  `tfsdk:"grouping_config"`
	MessageConfig    *models.AlertRouteV3MessageConfigModel   `tfsdk:"message_config"`
	IncidentConfig   *alertRouteDataSourceIncidentConfigModel `tfsdk:"incident_config"`
	OwningTeamIDs    types.Set                                `tfsdk:"owning_team_ids"`
}

// alertRouteDataSourceIncidentConfigModel is models.AlertRouteIncidentConfigModel without
// the grouping attributes that moved to grouping_config.
type alertRouteDataSourceIncidentConfigModel struct {
	Enabled            types.Bool                                `tfsdk:"enabled"`
	AutoDeclineEnabled types.Bool                                `tfsdk:"auto_decline_enabled"`
	ConditionGroups    models.IncidentEngineConditionGroups      `tfsdk:"condition_groups"`
	Template           *models.AlertRouteV3IncidentTemplateModel `tfsdk:"template"`
	IncidentTemplate   *models.IncidentEngineParamBinding        `tfsdk:"incident_template"`
}

// alertRouteDataSourceModelFromAPI converts through the resource's mapping so the two
// agree on every binding and condition, then drops the attributes this model leaves out.
func alertRouteDataSourceModelFromAPI(route client.AlertRouteV3) alertRouteDataSourceModel {
	full := models.AlertRouteResourceModel{}.FromAPIV3(route)

	return alertRouteDataSourceModel{
		ID:               full.ID,
		Name:             full.Name,
		Enabled:          full.Enabled,
		IsPrivate:        full.IsPrivate,
		AlertSources:     full.AlertSources,
		ConditionGroups:  full.ConditionGroups,
		Expressions:      full.Expressions,
		EscalationConfig: full.EscalationConfig,
		GroupingConfig:   full.GroupingConfig,
		MessageConfig:    full.MessageConfig,
		IncidentConfig: &alertRouteDataSourceIncidentConfigModel{
			Enabled:            full.IncidentConfig.Enabled,
			AutoDeclineEnabled: full.IncidentConfig.AutoDeclineEnabled,
			ConditionGroups:    full.IncidentConfig.ConditionGroups,
			Template:           full.IncidentConfig.Template,
			IncidentTemplate:   full.IncidentConfig.IncidentTemplate,
		},
		OwningTeamIDs: full.OwningTeamIDs,
	}
}

func (d *IncidentAlertRouteDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_route"
}

func (d *IncidentAlertRouteDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Routes V3"),
			"Use this data source to look up an existing alert route, either by `id` or by `name`, without "+
				"managing it as an `incident_alert_route` resource. Set exactly one of the two lookup attributes; "+
				"setting both, or neither, is rejected at plan time.\n\nThe route is read in the shape "+
				"`incident_alert_route` uses with `grouping_config` set, so its attributes can be passed "+
				"straight into one. The deprecated attributes of the earlier shape are not reported."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "enabled"),
				Computed:            true,
			},
			"is_private": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "is_private"),
				Computed:            true,
			},
			"alert_sources": schema.SetNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "alert_sources"),
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"alert_source_id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AlertRouteAlertSourceV2", "alert_source_id"),
							Computed:            true,
						},
						"condition_groups": models.ConditionGroupsDataSourceAttribute(),
					},
				},
			},
			"condition_groups": models.ConditionGroupsDataSourceAttribute(),
			"expressions":      models.ExpressionsDataSourceAttribute(),
			"escalation_config": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "escalation_config"),
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"auto_cancel_escalations": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV2", "auto_cancel_escalations"),
						Computed:            true,
					},
					"escalation_targets": schema.SetNestedAttribute{
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV2", "escalation_targets"),
						Computed:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"escalation_paths": schema.SingleNestedAttribute{
									MarkdownDescription: apischema.Docstring("AlertRouteEscalationTargetV2", "escalation_paths"),
									Computed:            true,
									Attributes:          models.ParamBindingDataSourceAttributes(),
								},
								"users": schema.SingleNestedAttribute{
									MarkdownDescription: apischema.Docstring("AlertRouteEscalationTargetV2", "users"),
									Computed:            true,
									Attributes:          models.ParamBindingDataSourceAttributes(),
								},
							},
						},
					},
					"when_alert_joins_group": schema.SingleNestedAttribute{
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV3", "when_alert_joins_group") +
							" Null for a route that does not group alerts.",
						Computed: true,
						Attributes: map[string]schema.Attribute{
							"mode": schema.StringAttribute{
								MarkdownDescription: EnumValuesDescription("AlertRouteWhenAlertJoinsGroupV3", "mode"),
								Computed:            true,
							},
							"grace_period_seconds": schema.Int64Attribute{
								MarkdownDescription: apischema.Docstring("AlertRouteWhenAlertJoinsGroupV3", "grace_period_seconds"),
								Computed:            true,
							},
						},
					},
				},
			},
			"grouping_config": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "grouping_config"),
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"default": schema.SingleNestedAttribute{
						MarkdownDescription: apischema.Docstring("AlertGroupingConfigV3", "default"),
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "enabled"),
								Computed:            true,
							},
							"ai_enabled": schema.BoolAttribute{
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "ai_enabled"),
								Computed:            true,
							},
							"grouping_keys": schema.SetNestedAttribute{
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "grouping_keys"),
								Computed:            true,
								NestedObject: schema.NestedAttributeObject{
									Attributes: map[string]schema.Attribute{
										"reference": schema.StringAttribute{
											MarkdownDescription: apischema.Docstring("GroupingKeyV3", "reference"),
											Computed:            true,
										},
									},
								},
							},
							"window_seconds": schema.Int64Attribute{
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "window_seconds"),
								Computed:            true,
							},
							"window_type": schema.StringAttribute{
								MarkdownDescription: EnumValuesDescription("GroupingSettingsV3", "window_type"),
								Computed:            true,
							},
						},
					},
				},
			},
			"message_config": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "message_config"),
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"destinations": schema.SetNestedAttribute{
						MarkdownDescription: apischema.Docstring("AlertMessageConfigV3", "destinations"),
						Computed:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"condition_groups": models.ConditionGroupsDataSourceAttribute(),
								"ms_teams_targets": alertRouteChannelTargetDataSourceAttribute("ms_teams_targets"),
								"slack_targets":    alertRouteChannelTargetDataSourceAttribute("slack_targets"),
							},
						},
					},
					"template": schema.SingleNestedAttribute{
						MarkdownDescription: apischema.Docstring("AlertMessageConfigV3", "template"),
						Computed:            true,
						Attributes:          models.ParamBindingDataSourceAttributes(),
					},
				},
			},
			"incident_config": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "incident_config"),
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "enabled"),
						Computed:            true,
					},
					"auto_decline_enabled": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "auto_decline_enabled"),
						Computed:            true,
					},
					"condition_groups": models.ConditionGroupsDataSourceAttribute(),
					"template": schema.SingleNestedAttribute{
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV3", "template"),
						Computed:            true,
						Attributes:          alertRouteIncidentTemplateDataSourceAttributes(),
					},
					"incident_template": schema.SingleNestedAttribute{
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV3", "incident_template"),
						Computed:            true,
						Attributes:          models.ParamBindingDataSourceAttributes(),
					},
				},
			},
			"owning_team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "owning_team_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

// alertRouteChannelTargetDataSourceAttribute is the computed form of a message
// destination's slack_targets or ms_teams_targets block.
func alertRouteChannelTargetDataSourceAttribute(name string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: apischema.Docstring("AlertMessageDestinationV3", name),
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"binding": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV3", "binding"),
				Computed:            true,
				Attributes:          models.ParamBindingDataSourceAttributes(),
			},
			"channel_visibility": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV3", "channel_visibility"),
				Computed:            true,
			},
			"group_alerts_summary": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV3", "group_alerts_summary"),
				Computed:            true,
			},
		},
	}
}

// alertRouteIncidentTemplateDataSourceAttributes is the computed form of the resource's
// incidentTemplateAttributes(false): the inline template under incident_config.
func alertRouteIncidentTemplateDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"custom_fields": schema.SetNestedAttribute{
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "custom_fields"),
			Computed:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"custom_field_id": schema.StringAttribute{
						MarkdownDescription: "ID of the custom field",
						Computed:            true,
					},
					"binding": schema.SingleNestedAttribute{
						MarkdownDescription: "Binding for the custom field",
						Computed:            true,
						Attributes:          models.ParamBindingDataSourceAttributes(),
					},
					"merge_strategy": schema.StringAttribute{
						MarkdownDescription: EnumValuesDescription("AlertRouteCustomFieldBindingV3", "merge_strategy"),
						Computed:            true,
					},
				},
			},
		},
		"incident_mode": schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "incident_mode"),
			Computed:            true,
			Attributes:          models.ParamBindingDataSourceAttributes(),
		},
		"incident_type": schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "incident_type"),
			Computed:            true,
			Attributes:          models.ParamBindingDataSourceAttributes(),
		},
		"name": schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "name"),
			Computed:            true,
			Attributes:          models.AutoGeneratedParamBindingDataSourceAttributes(),
		},
		"severity": schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "severity"),
			Computed:            true,
			Attributes: map[string]schema.Attribute{
				"binding": schema.SingleNestedAttribute{
					MarkdownDescription: apischema.Docstring("AlertRouteSeverityBindingV3", "binding"),
					Computed:            true,
					Attributes:          models.ParamBindingDataSourceAttributes(),
				},
				"merge_strategy": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("AlertRouteSeverityBindingV3", "merge_strategy"),
					Computed:            true,
				},
			},
		},
		"start_in_triage": schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "start_in_triage"),
			Computed:            true,
			Attributes:          models.ParamBindingDataSourceAttributes(),
		},
		"summary": schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "summary"),
			Computed:            true,
			Attributes:          models.AutoGeneratedParamBindingDataSourceAttributes(),
		},
	}
}

func (d *IncidentAlertRouteDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *alertRouteDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	validateLookupByIDOrName(data.ID, data.Name, &resp.Diagnostics)
}

func (d *IncidentAlertRouteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data alertRouteDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()
	if data.ID.IsNull() {
		found, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route by name, got error: %s", err))
			return
		}
		id = found
	}

	result, err := d.client.AlertRoutesV3ShowWithResponse(ctx, id)
	if err == nil && result.JSON200 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		// The current API refuses routes for an organisation still on the previous alert
		// grouping engine, and this data source has no older shape to fall back to.
		if isAPINotYetAvailable(err) {
			resp.Diagnostics.AddError("Alert route data source not available",
				"This organisation has not moved to the new alert grouping engine, so its alert "+
					"routes cannot be read through this data source yet.")
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route, got error: %s", err))
		return
	}

	model := alertRouteDataSourceModelFromAPI(result.JSON200.AlertRoute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// alertRouteListPageSize is the largest page the list endpoint accepts.
const alertRouteListPageSize = 50

// findByName returns the id of the one alert route with this name. The list endpoint has
// no name filter, so every page has to be fetched to know a name doesn't appear on a later
// one, and nothing stops two routes sharing a name, so more than one match is reported
// rather than resolved arbitrarily.
func (d *IncidentAlertRouteDataSource) findByName(ctx context.Context, name string) (string, error) {
	var (
		after   *string
		matches []client.AlertRouteSlimV3
	)

	for {
		result, err := d.client.AlertRoutesV3ListWithResponse(ctx, &client.AlertRoutesV3ListParams{
			PageSize: lo.ToPtr(int64(alertRouteListPageSize)),
			After:    after,
		})
		if err != nil {
			if isAPINotYetAvailable(err) {
				return "", fmt.Errorf("this organisation has not moved to the new alert grouping engine, " +
					"so its alert routes cannot be read through this data source yet")
			}
			return "", err
		}
		if result.JSON200 == nil {
			return "", fmt.Errorf("unexpected response listing alert routes: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.AlertRoutes, func(route client.AlertRouteSlimV3, _ int) bool {
			return route.Name == name
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
		return "", fmt.Errorf("no alert route found with name %q", name)
	case 1:
		return matches[0].Id, nil
	default:
		return "", fmt.Errorf("found %d alert routes named %q; look it up by id instead", len(matches), name)
	}
}
