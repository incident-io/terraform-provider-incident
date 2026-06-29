package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
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
	_ resource.ResourceWithConfigure      = &IncidentAlertRouteV3Resource{}
	_ resource.ResourceWithImportState    = &IncidentAlertRouteV3Resource{}
	_ resource.ResourceWithValidateConfig = &IncidentAlertRouteV3Resource{}
)

type IncidentAlertRouteV3Resource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

func NewIncidentAlertRouteV3Resource() resource.Resource {
	return &IncidentAlertRouteV3Resource{}
}

func (r *IncidentAlertRouteV3Resource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_route_v3"
}

func (r *IncidentAlertRouteV3Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Routes V3"), `This resource uses the v3 alert routes API, which configures grouping via a dedicated `+"`grouping_config`"+`, combines channels and message templates into `+"`message_config`"+`, and nests the incident template under `+"`incident_config`"+`. For the previous API, see `+"`incident_alert_route`"+`.

We'd generally recommend building alert routes in our [web dashboard](https://app.incident.io/~/alerts/configuration), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing alert route and copy the resulting Terraform without persisting it.`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "id"),
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "name"),
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "enabled"),
			},
			"is_private": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "is_private"),
			},
			"alert_sources": schema.SetNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "alert_sources"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"alert_source_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("AlertRouteAlertSourceV3", "alert_source_id"),
						},
						"condition_groups": models.ConditionGroupsAttribute(),
					},
				},
			},
			"condition_groups": models.ConditionGroupsAttribute(),
			"expressions":      models.ExpressionsAttribute(),
			"escalation_config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "escalation_config"),
				Attributes: map[string]schema.Attribute{
					"auto_cancel_escalations": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV3", "auto_cancel_escalations"),
					},
					"escalation_targets": schema.SetNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV3", "escalation_targets"),
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"escalation_paths": schema.SingleNestedAttribute{
									Optional:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteEscalationTargetV3", "escalation_paths"),
									Attributes:          models.ParamBindingAttributes(),
								},
								"users": schema.SingleNestedAttribute{
									Optional:            true,
									MarkdownDescription: apischema.Docstring("AlertRouteEscalationTargetV3", "users"),
									Attributes:          models.ParamBindingAttributes(),
								},
							},
						},
					},
					"when_alert_joins_group": schema.SingleNestedAttribute{
						// Optional + Computed: when grouping is enabled the API always
						// returns a default when_alert_joins_group (e.g. on_each_new_alert)
						// even if the user didn't configure one, so we let the provider
						// accept that computed value rather than erroring on it.
						Optional:            true,
						Computed:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteEscalationConfigV3", "when_alert_joins_group"),
						PlanModifiers: []planmodifier.Object{
							objectplanmodifier.UseStateForUnknown(),
						},
						Attributes: map[string]schema.Attribute{
							"mode": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: EnumValuesDescription("AlertRouteAlertJoinsGroupV3", "mode"),
							},
							"grace_period_seconds": schema.Int64Attribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteAlertJoinsGroupV3", "grace_period_seconds"),
							},
						},
					},
				},
			},
			"grouping_config": schema.SingleNestedAttribute{
				Required:            true,
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
							"group_keys": schema.SetNestedAttribute{
								// Optional: only valid when grouping is enabled. Enforced
								// conditionally in ValidateConfig.
								Optional:            true,
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "group_keys"),
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
								// Optional: required when grouping is enabled, must be
								// unset otherwise. Enforced in ValidateConfig.
								Optional:            true,
								MarkdownDescription: apischema.Docstring("GroupingSettingsV3", "window_seconds"),
							},
							"window_type": schema.StringAttribute{
								// Optional: required when grouping is enabled, must be
								// unset otherwise. Enforced in ValidateConfig.
								Optional:            true,
								MarkdownDescription: EnumValuesDescription("GroupingSettingsV3", "window_type"),
							},
						},
					},
				},
			},
			"message_config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "message_config"),
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
					"message_template": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertMessageConfigV3", "message_template"),
						Attributes:          models.ParamBindingAttributes(),
					},
				},
			},
			"incident_config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "incident_config"),
				Attributes: map[string]schema.Attribute{
					"auto_decline_enabled": schema.BoolAttribute{
						// Optional: required when incident creation is enabled, must be
						// unset otherwise. Enforced in ValidateConfig.
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV3", "auto_decline_enabled"),
					},
					"enabled": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV3", "enabled"),
					},
					"condition_groups": models.ConditionGroupsAttribute(),
					"template": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV3", "template"),
						Attributes: map[string]schema.Attribute{
							"custom_fields": schema.SetNestedAttribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "custom_fields"),
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
											MarkdownDescription: EnumValuesDescription("AlertRouteCustomFieldBindingV3", "merge_strategy"),
										},
									},
								},
							},
							"incident_mode": schema.SingleNestedAttribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "incident_mode"),
								Attributes:          models.ParamBindingAttributes(),
							},
							"incident_type": schema.SingleNestedAttribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "incident_type"),
								Attributes:          models.ParamBindingAttributes(),
							},
							"name": schema.SingleNestedAttribute{
								Required:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "name"),
								Attributes:          models.AutoGeneratedParamBindingAttributes(),
							},
							"severity": schema.SingleNestedAttribute{
								Optional:            true,
								Computed:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "severity"),
								PlanModifiers: []planmodifier.Object{
									objectplanmodifier.UseStateForUnknown(),
								},
								Attributes: map[string]schema.Attribute{
									"binding": schema.SingleNestedAttribute{
										Optional:            true,
										MarkdownDescription: apischema.Docstring("AlertRouteSeverityBindingV3", "binding"),
										Attributes:          models.ParamBindingAttributes(),
									},
									"merge_strategy": schema.StringAttribute{
										Required:            true,
										MarkdownDescription: EnumValuesDescription("AlertRouteSeverityBindingV3", "merge_strategy"),
									},
								},
							},
							"start_in_triage": schema.SingleNestedAttribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "start_in_triage"),
								Attributes:          models.ParamBindingAttributes(),
							},
							"summary": schema.SingleNestedAttribute{
								Required:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV3", "summary"),
								Attributes:          models.AutoGeneratedParamBindingAttributes(),
							},
						},
					},
				},
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("AlertRouteV3", "owning_team_ids"),
			},
		},
	}
}

func (r *IncidentAlertRouteV3Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentAlertRouteV3Resource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	// window_seconds and window_type are optional in the schema (the API omits
	// them when grouping is disabled), but are required when grouping is enabled.
	var groupingEnabled types.Bool
	if d := req.Config.GetAttribute(ctx, path.Root("grouping_config").AtName("default").AtName("enabled"), &groupingEnabled); !d.HasError() &&
		!groupingEnabled.IsNull() && !groupingEnabled.IsUnknown() && groupingEnabled.ValueBool() {
		var windowSeconds types.Int64
		if d := req.Config.GetAttribute(ctx, path.Root("grouping_config").AtName("default").AtName("window_seconds"), &windowSeconds); !d.HasError() && windowSeconds.IsNull() {
			resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("grouping_config").AtName("default").AtName("window_seconds"),
				"Missing required attribute",
				"`window_seconds` is required when `grouping_config.default.enabled` is true.",
			))
		}
		var windowType types.String
		if d := req.Config.GetAttribute(ctx, path.Root("grouping_config").AtName("default").AtName("window_type"), &windowType); !d.HasError() && windowType.IsNull() {
			resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("grouping_config").AtName("default").AtName("window_type"),
				"Missing required attribute",
				"`window_type` is required when `grouping_config.default.enabled` is true.",
			))
		}
	}

	// auto_decline_enabled is optional in the schema (the API omits it when
	// incident creation is disabled), but is required when it's enabled.
	var incidentEnabled types.Bool
	if d := req.Config.GetAttribute(ctx, path.Root("incident_config").AtName("enabled"), &incidentEnabled); !d.HasError() &&
		!incidentEnabled.IsNull() && !incidentEnabled.IsUnknown() && incidentEnabled.ValueBool() {
		var autoDecline types.Bool
		if d := req.Config.GetAttribute(ctx, path.Root("incident_config").AtName("auto_decline_enabled"), &autoDecline); !d.HasError() && autoDecline.IsNull() {
			resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("incident_config").AtName("auto_decline_enabled"),
				"Missing required attribute",
				"`auto_decline_enabled` is required when `incident_config.enabled` is true.",
			))
		}
	}

	var expressions []models.IncidentEngineExpression

	diags := req.Config.GetAttribute(ctx, path.Root("expressions"), &expressions)
	if diags.HasError() {
		// If expressions is unknown (e.g., depends on another resource), skip validation.
		return
	}

	// Validate that branches operations have valid root references:
	// Branches operations require root_reference to be "." (the whole scope), with conditions
	// referencing absolute paths like "alert.attributes.xxx".
	for i, expr := range expressions {
		hasBranches := false
		for _, op := range expr.Operations {
			if op.Branches != nil {
				hasBranches = true
				break
			}
		}

		if !hasBranches {
			continue
		}

		rootRef := expr.RootReference.ValueString()
		if rootRef != "" && rootRef != "." {
			resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("expressions").AtListIndex(i).AtName("root_reference"),
				"Invalid root_reference for branches operation",
				fmt.Sprintf(
					"Expression %q uses a branches (if/else) operation, which requires "+
						"root_reference to be \".\" (the whole scope). Got %q instead.\n\n"+
						"When using branches operations, set root_reference = \".\" and have "+
						"conditions reference absolute paths like \"alert.attributes.xxx\".",
					expr.Label.ValueString(),
					rootRef,
				),
			))
		}
	}
}

func (r *IncidentAlertRouteV3Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AlertRouteV3ResourceModel
	var plan models.AlertRouteV3ResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := data.ToCreatePayload()

	result, err := r.client.AlertRoutesV3CreateWithResponse(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert route, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON201.AlertRoute.Id, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("Created an alert route with id=%s", result.JSON201.AlertRoute.Id))

	data = models.AlertRouteV3ResourceModel{}.FromAPIWithPlan(result.JSON201.AlertRoute, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteV3Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AlertRouteV3ResourceModel
	var state models.AlertRouteV3ResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertRoutesV3ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Alert route with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route, got error: %s", err))
		return
	}

	data = models.AlertRouteV3ResourceModel{}.FromAPIWithPlan(result.JSON200.AlertRoute, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteV3Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AlertRouteV3ResourceModel
	var plan models.AlertRouteV3ResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertRoutesV3ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Alert route with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route before updating, got error: %s", err))
		return
	}

	currentVersion := result.JSON200.AlertRoute.Version

	payload := data.ToUpdatePayload()
	payload.Version = currentVersion + 1

	updateResult, err := r.client.AlertRoutesV3UpdateWithResponse(ctx, data.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert route, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON200.AlertRoute.Id, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)

	data = models.AlertRouteV3ResourceModel{}.FromAPIWithPlan(updateResult.JSON200.AlertRoute, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteV3Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AlertRouteV3ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.AlertRoutesV3DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert route, got error: %s", err))
		return
	}
}

func (r *IncidentAlertRouteV3Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResource(ctx, r.client, req.ID, &resp.Diagnostics, client.AlertRoute, r.terraformVersion)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
