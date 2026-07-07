package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ resource.ResourceWithConfigure      = &IncidentAlertRouteResource{}
	_ resource.ResourceWithImportState    = &IncidentAlertRouteResource{}
	_ resource.ResourceWithValidateConfig = &IncidentAlertRouteResource{}
)

// Deprecation messages for the deprecated attributes, each pointing at its
// replacement. They are emitted whenever the attribute is set, nudging users
// onto the replacement blocks (opted into via the top-level grouping_config).
const (
	deprecatedChannelConfig         = "Deprecated: use `message_config.destinations` instead."
	deprecatedMessageTemplate       = "Deprecated: use `message_config.template` instead."
	deprecatedIncidentTemplate      = "Deprecated: use `incident_config.template` instead."
	deprecatedGroupingKeys          = "Deprecated: configure alert grouping via the top-level `grouping_config` instead."
	deprecatedGroupingWindowSeconds = "Deprecated: configure alert grouping via the top-level `grouping_config` instead."
	deprecatedDeferTimeSeconds      = "Deprecated: configure alert grouping via the top-level `grouping_config` instead."
	deprecatedAutoRelateGrouped     = "Deprecated: configure alert grouping via the top-level `grouping_config` instead."
)

type IncidentAlertRouteResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

func NewIncidentAlertRouteResource() resource.Resource {
	return &IncidentAlertRouteResource{}
}

func (r *IncidentAlertRouteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_route"
}

func (r *IncidentAlertRouteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Alert routes define how alerts are processed: how they're grouped, which channels they post to, who is escalated, and whether they open incidents.

We'd generally recommend building alert routes in our [web dashboard](https://app.incident.io/~/alerts/configuration), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing alert route and copy the resulting Terraform without persisting it.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "id"),
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "name"),
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "enabled"),
			},
			"is_private": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "is_private"),
			},
			"alert_sources": schema.SetNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "alert_sources"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"alert_source_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("AlertRouteAlertSourceV2", "alert_source_id"),
						},
						"condition_groups": models.ConditionGroupsAttribute(),
					},
				},
			},
			// --- v2-only: channel_config (deprecated, see message_config) ---
			"channel_config": schema.SetNestedAttribute{
				Optional:            true,
				DeprecationMessage:  deprecatedChannelConfig,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "channel_config") + "\n\n" + deprecatedChannelConfig,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"condition_groups": models.ConditionGroupsAttribute(),
						"ms_teams_targets": schema.SingleNestedAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AlertRouteChannelConfigV2", "ms_teams_targets"),
							Attributes: map[string]schema.Attribute{
								"binding": schema.SingleNestedAttribute{
									Required:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV2", "binding"),
									Attributes:          models.ParamBindingAttributes(),
								},
								"channel_visibility": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV2", "channel_visibility"),
								},
							},
						},
						"slack_targets": schema.SingleNestedAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AlertRouteChannelConfigV2", "slack_targets"),
							Attributes: map[string]schema.Attribute{
								"binding": schema.SingleNestedAttribute{
									Required:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV2", "binding"),
									Attributes:          models.ParamBindingAttributes(),
								},
								"channel_visibility": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV2", "channel_visibility"),
								},
							},
						},
					},
				},
			},
			"condition_groups": models.ConditionGroupsAttribute(),
			"expressions":      models.ExpressionsAttribute(),
			"escalation_config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "escalation_config"),
				Attributes: map[string]schema.Attribute{
					"auto_cancel_escalations": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV2", "auto_cancel_escalations"),
					},
					"escalation_targets": schema.SetNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV2", "escalation_targets"),
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"escalation_paths": schema.SingleNestedAttribute{
									Optional:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteEscalationTargetV2", "escalation_paths"),
									Attributes:          models.ParamBindingAttributes(),
								},
								"users": schema.SingleNestedAttribute{
									Optional:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteEscalationTargetV2", "users"),
									Attributes:          models.ParamBindingAttributes(),
								},
							},
						},
					},
					// --- v3-only escalation field ---
					"when_alert_joins_group": schema.SingleNestedAttribute{
						// Optional + Computed: v3 only. When grouping is enabled the API
						// returns a default when_alert_joins_group even if the user
						// didn't configure one, so we accept that computed value.
						Optional:            true,
						Computed:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV3", "when_alert_joins_group"),
						PlanModifiers: []planmodifier.Object{
							whenAlertJoinsGroupPlanModifier{},
						},
						Attributes: map[string]schema.Attribute{
							"mode": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: EnumValuesDescription("AlertRouteWhenAlertJoinsGroupV3", "mode"),
							},
							"grace_period_seconds": schema.Int64Attribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteWhenAlertJoinsGroupV3", "grace_period_seconds"),
							},
						},
					},
				},
			},
			// --- v3-only: grouping_config (the schema discriminator) ---
			"grouping_config": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "grouping_config"),
				Attributes: map[string]schema.Attribute{
					"default": schema.SingleNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertGroupingConfigV3", "default"),
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								Required:            true,
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "enabled"),
							},
							"grouping_keys": schema.SetNestedAttribute{
								// Optional: only valid when grouping is enabled. Enforced
								// conditionally in ValidateConfig.
								Optional:            true,
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "grouping_keys"),
								NestedObject: schema.NestedAttributeObject{
									Attributes: map[string]schema.Attribute{
										"reference": schema.StringAttribute{
											Required:            true,
											MarkdownDescription: apischema.Docstring("GroupingKeyV3", "reference"),
										},
									},
								},
							},
							"window_seconds": schema.Int64Attribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "window_seconds"),
							},
							"window_type": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: EnumValuesDescription("GroupingSettingsV3", "window_type"),
							},
						},
					},
				},
			},
			// --- v3-only: message_config (see channel_config / message_template) ---
			"message_config": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "message_config") + " Only used with `grouping_config`.",
				Attributes: map[string]schema.Attribute{
					"destinations": schema.SetNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertMessageConfigV3", "destinations"),
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"condition_groups": models.ConditionGroupsAttribute(),
								"ms_teams_targets": schema.SingleNestedAttribute{
									Optional:            true,
									MarkdownDescription: apischema.Docstring("AlertMessageDestinationV3", "ms_teams_targets"),
									Attributes: map[string]schema.Attribute{
										"binding": schema.SingleNestedAttribute{
											Required:            true,
											MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV3", "binding"),
											Attributes:          models.ParamBindingAttributes(),
										},
										"channel_visibility": schema.StringAttribute{
											Required:            true,
											MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV3", "channel_visibility"),
										},
									},
								},
								"slack_targets": schema.SingleNestedAttribute{
									Optional:            true,
									MarkdownDescription: apischema.Docstring("AlertMessageDestinationV3", "slack_targets"),
									Attributes: map[string]schema.Attribute{
										"binding": schema.SingleNestedAttribute{
											Required:            true,
											MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV3", "binding"),
											Attributes:          models.ParamBindingAttributes(),
										},
										"channel_visibility": schema.StringAttribute{
											Required:            true,
											MarkdownDescription: apischema.Docstring("AlertRouteChannelTargetV3", "channel_visibility"),
										},
									},
								},
							},
						},
					},
					"template": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertMessageConfigV3", "template"),
						Attributes:          models.ParamBindingAttributes(),
					},
				},
			},
			"incident_config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "incident_config"),
				Attributes: map[string]schema.Attribute{
					"auto_decline_enabled": schema.BoolAttribute{
						// Optional: required when incident creation is enabled, must be
						// unset otherwise. Enforced in ValidateConfig.
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "auto_decline_enabled"),
					},
					"auto_relate_grouped_alerts": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						DeprecationMessage:  deprecatedAutoRelateGrouped,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "auto_relate_grouped_alerts") + "\n\n" + deprecatedAutoRelateGrouped,
						PlanModifiers: []planmodifier.Bool{
							autoRelateGroupedAlertsPlanModifier{},
						},
					},
					"enabled": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "enabled"),
					},
					"condition_groups": models.ConditionGroupsAttribute(),
					"grouping_keys": schema.SetNestedAttribute{
						// Optional: required in v2 mode, forbidden in v3. Enforced in
						// ValidateConfig.
						Optional:            true,
						DeprecationMessage:  deprecatedGroupingKeys,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "grouping_keys") + "\n\n" + deprecatedGroupingKeys,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"reference": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "The alert attribute ID to use as a grouping key",
								},
							},
						},
					},
					"grouping_window_seconds": schema.Int64Attribute{
						Optional:            true,
						DeprecationMessage:  deprecatedGroupingWindowSeconds,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "grouping_window_seconds") + "\n\n" + deprecatedGroupingWindowSeconds,
					},
					"defer_time_seconds": schema.Int64Attribute{
						Optional:            true,
						DeprecationMessage:  deprecatedDeferTimeSeconds,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "defer_time_seconds") + "\n\n" + deprecatedDeferTimeSeconds,
					},
					// --- v3-only: incident template moves here ---
					"template": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV3", "template") + " Only used with `grouping_config`.",
						Attributes:          incidentTemplateAttributes(false),
					},
				},
			},
			// --- v2-only: incident_template (deprecated, see incident_config.template) ---
			"incident_template": schema.SingleNestedAttribute{
				Optional:            true,
				DeprecationMessage:  deprecatedIncidentTemplate,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "incident_template") + "\n\n" + deprecatedIncidentTemplate,
				Attributes:          incidentTemplateAttributes(true),
			},
			// --- v2-only: message_template (deprecated, see message_config.template) ---
			"message_template": schema.SingleNestedAttribute{
				Optional:            true,
				DeprecationMessage:  deprecatedMessageTemplate,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "message_template") + "\n\n" + deprecatedMessageTemplate,
				Attributes:          models.ParamBindingAttributes(),
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "owning_team_ids"),
			},
		},
	}
}

// incidentTemplateAttributes returns the shared incident-template attribute set.
// The v2 top-level incident_template carries a workspace binding that the v3
// incident_config.template does not, so includeWorkspace gates that attribute.
func incidentTemplateAttributes(includeWorkspace bool) map[string]schema.Attribute {
	v := "V2"
	if !includeWorkspace {
		v = "V3"
	}
	attrs := map[string]schema.Attribute{
		"custom_fields": schema.SetNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplate"+v, "custom_fields"),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"custom_field_id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "ID of the custom field",
					},
					"binding": schema.SingleNestedAttribute{
						Required:            true,
						MarkdownDescription: "Binding for the custom field",
						Attributes:          models.ParamBindingAttributes(),
					},
					"merge_strategy": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: EnumValuesDescription("AlertRouteCustomFieldBinding"+v, "merge_strategy"),
					},
				},
			},
		},
		"incident_mode": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplate"+v, "incident_mode"),
			Attributes:          models.ParamBindingAttributes(),
		},
		"incident_type": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplate"+v, "incident_type"),
			Attributes:          models.ParamBindingAttributes(),
		},
		"name": schema.SingleNestedAttribute{
			Required:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplate"+v, "name"),
			Attributes:          models.AutoGeneratedParamBindingAttributes(),
		},
		"severity": schema.SingleNestedAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplate"+v, "severity"),
			PlanModifiers: []planmodifier.Object{
				objectplanmodifier.UseStateForUnknown(),
			},
			Attributes: map[string]schema.Attribute{
				"binding": schema.SingleNestedAttribute{
					Optional:            true,
					MarkdownDescription: apischema.Docstring("AlertRouteSeverityBinding"+v, "binding"),
					Attributes:          models.ParamBindingAttributes(),
				},
				"merge_strategy": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: EnumValuesDescription("AlertRouteSeverityBinding"+v, "merge_strategy"),
				},
			},
		},
		"start_in_triage": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplate"+v, "start_in_triage"),
			Attributes:          models.ParamBindingAttributes(),
		},
		"summary": schema.SingleNestedAttribute{
			Required:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplate"+v, "summary"),
			Attributes:          models.AutoGeneratedParamBindingAttributes(),
		},
	}

	if includeWorkspace {
		attrs["workspace"] = schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "workspace"),
			Attributes:          models.ParamBindingAttributes(),
		}
	}

	return attrs
}

func (r *IncidentAlertRouteResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client.Client
	r.terraformVersion = client.TerraformVersion
}

func (r *IncidentAlertRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AlertRouteResourceModel
	var plan models.AlertRouteResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.IsV3Mode() {
		result, err := r.client.AlertRoutesV3CreateWithResponse(ctx, data.ToCreatePayloadV3())
		if err != nil {
			if isAPINotYetAvailable(err) {
				resp.Diagnostics.AddError(alertRouteV3UnavailableError())
				return
			}
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert route, got error: %s", err))
			return
		}

		claimResource(ctx, r.client, result.JSON201.AlertRoute.Id, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)
		tflog.Trace(ctx, fmt.Sprintf("Created an alert route with id=%s", result.JSON201.AlertRoute.Id))

		data = models.AlertRouteResourceModel{}.FromAPIV3WithPlan(result.JSON201.AlertRoute, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	result, err := r.client.AlertRoutesV2CreateWithResponse(ctx, data.ToCreatePayloadV2())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert route, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON201.AlertRoute.Id, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)
	tflog.Trace(ctx, fmt.Sprintf("Created an alert route with id=%s", result.JSON201.AlertRoute.Id))

	data = models.AlertRouteResourceModel{}.FromAPIV2WithPlan(result.JSON201.AlertRoute, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AlertRouteResourceModel
	var state models.AlertRouteResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The schema in use is recorded in state (grouping_config is set for v3), so
	// Read can dispatch to the matching API without the configuration.
	if data.IsV3Mode() {
		result, err := r.client.AlertRoutesV3ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			if isNotFound(err) {
				tflog.Warn(ctx, fmt.Sprintf("Alert route with ID %s not found: removing from state.", data.ID.ValueString()))
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route, got error: %s", err))
			return
		}

		data = models.AlertRouteResourceModel{}.FromAPIV3WithPlan(result.JSON200.AlertRoute, &state)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	result, err := r.client.AlertRoutesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, fmt.Sprintf("Alert route with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route, got error: %s", err))
		return
	}

	data = models.AlertRouteResourceModel{}.FromAPIV2WithPlan(result.JSON200.AlertRoute, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AlertRouteResourceModel
	var plan models.AlertRouteResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Dispatch on the planned schema. Both APIs address the same underlying
	// alert route, so a v2 -> v3 migration (adding grouping_config) is an update,
	// not a replacement.
	if data.IsV3Mode() {
		showResult, err := r.client.AlertRoutesV3ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			if isNotFound(err) {
				tflog.Warn(ctx, fmt.Sprintf("Alert route with ID %s not found: removing from state.", data.ID.ValueString()))
				resp.State.RemoveResource(ctx)
				return
			}
			if isAPINotYetAvailable(err) {
				resp.Diagnostics.AddError(alertRouteV3UnavailableError())
				return
			}
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route before updating, got error: %s", err))
			return
		}

		payload := data.ToUpdatePayloadV3()
		payload.Version = showResult.JSON200.AlertRoute.Version + 1

		updateResult, err := r.client.AlertRoutesV3UpdateWithResponse(ctx, data.ID.ValueString(), payload)
		if err != nil {
			if isAPINotYetAvailable(err) {
				resp.Diagnostics.AddError(alertRouteV3UnavailableError())
				return
			}
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert route, got error: %s", err))
			return
		}

		claimResource(ctx, r.client, showResult.JSON200.AlertRoute.Id, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)

		data = models.AlertRouteResourceModel{}.FromAPIV3WithPlan(updateResult.JSON200.AlertRoute, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	showResult, err := r.client.AlertRoutesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, fmt.Sprintf("Alert route with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route before updating, got error: %s", err))
		return
	}

	payload := data.ToUpdatePayloadV2()
	payload.Version = showResult.JSON200.AlertRoute.Version + 1

	updateResult, err := r.client.AlertRoutesV2UpdateWithResponse(ctx, data.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert route, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, showResult.JSON200.AlertRoute.Id, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)

	data = models.AlertRouteResourceModel{}.FromAPIV2WithPlan(updateResult.JSON200.AlertRoute, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AlertRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.IsV3Mode() {
		_, err := r.client.AlertRoutesV3DeleteWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert route, got error: %s", err))
		}
		return
	}

	_, err := r.client.AlertRoutesV2DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert route, got error: %s", err))
	}
}

func (r *IncidentAlertRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The same underlying alert route is readable through both the v2 and v3
	// APIs, and Read infers the schema from state (which is empty on import), so
	// we decide the schema here by probing. Organisations migrated to the new
	// alert grouping engine are imported with the v3 schema; organisations that
	// haven't migrated get a 403 `api_not_yet_available` from the v3 API, so we
	// fall back to the v2 API and import with the v2 schema. We populate the full
	// state via the matching API so the subsequent refresh dispatches correctly.
	id := req.ID

	claimResource(ctx, r.client, id, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)
	if resp.Diagnostics.HasError() {
		return
	}

	v3Result, err := r.client.AlertRoutesV3ShowWithResponse(ctx, id)
	switch {
	case err == nil && v3Result.JSON200 != nil:
		data := models.AlertRouteResourceModel{}.FromAPIV3(v3Result.JSON200.AlertRoute)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	case err != nil && !isAPINotYetAvailable(err):
		// A genuine error rather than the migration gate: surface it rather than
		// masking it behind a v2 fallback.
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to import alert route, got error: %s", err))
		return
	}

	// The organisation hasn't migrated to the new alert grouping engine (the v3
	// API returned `api_not_yet_available`), so import via the v2 API.
	v2Result, err := r.client.AlertRoutesV2ShowWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to import alert route, got error: %s", err))
		return
	}
	if v2Result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to import alert route %q: not found.", id))
		return
	}
	data := models.AlertRouteResourceModel{}.FromAPIV2(v2Result.JSON200.AlertRoute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// isNotFound reports whether err is a 404 from the API.
func isNotFound(err error) bool {
	httpErr := client.HTTPError{}
	return errors.As(err, &httpErr) && httpErr.StatusCode == 404
}

// alertRouteAPINotYetAvailableCode is the error code the v3 alert routes API
// returns (with a 403) for organisations that have not migrated to the new
// alert grouping engine.
const alertRouteAPINotYetAvailableCode = "api_not_yet_available"

// isAPINotYetAvailable reports whether err is the v3 alert routes API's
// organisation-not-migrated gate, so callers can fall back to the v2 API or show
// a clearer error.
func isAPINotYetAvailable(err error) bool {
	httpErr := client.HTTPError{}
	if !errors.As(err, &httpErr) {
		return false
	}

	// incident.io error envelope: {"errors": [{"code": "..."}], ...}. Parse it,
	// falling back to a raw substring match in case the envelope shape differs.
	var body struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(httpErr.Body, &body) == nil {
		for _, e := range body.Errors {
			if e.Code == alertRouteAPINotYetAvailableCode {
				return true
			}
		}
	}

	return bytes.Contains(httpErr.Body, []byte(alertRouteAPINotYetAvailableCode))
}

// alertRouteV3UnavailableError returns the diagnostic shown when a v3-schema
// alert route is used against an organisation that has not migrated to the new
// alert grouping engine.
func alertRouteV3UnavailableError() (summary, detail string) {
	return "Alert route configuration format not available",
		"This alert route uses `grouping_config`, but that configuration format " +
			"isn't available for your organisation yet. Remove `grouping_config` (and " +
			"the other blocks that depend on it) and use the deprecated attributes instead."
}
