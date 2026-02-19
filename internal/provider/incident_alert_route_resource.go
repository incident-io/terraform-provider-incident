package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Routes V2"), `We'd generally recommend building alert routes in our [web dashboard](https://app.incident.io/~/alerts/configuration), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing alert route and copy the resulting Terraform without persisting it.`),
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
			"channel_config": schema.SetNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "channel_config"),
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
				},
			},
			"incident_config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "incident_config"),
				Attributes: map[string]schema.Attribute{
					"auto_decline_enabled": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "auto_decline_enabled"),
					},
					"auto_relate_grouped_alerts": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "auto_relate_grouped_alerts"),
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"enabled": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "enabled"),
					},
					"condition_groups": models.ConditionGroupsAttribute(),
					"grouping_keys": schema.SetNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "grouping_keys"),
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
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "grouping_window_seconds"),
					},
					"defer_time_seconds": schema.Int64Attribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "defer_time_seconds"),
					},
				},
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "owning_team_ids"),
			},
			"message_template": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "message_template"),
				Attributes:          models.ParamBindingAttributes(),
			},
			"incident_template": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertRouteV2", "incident_template"),
				Attributes: map[string]schema.Attribute{
					"custom_fields": schema.SetNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "custom_fields"),
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
									MarkdownDescription: EnumValuesDescription("AlertRouteCustomFieldBindingV2", "merge_strategy"),
								}},
						},
					},
					"incident_mode": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "incident_mode"),
						Attributes:          models.ParamBindingAttributes(),
					},
					"incident_type": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "incident_type"),
						Attributes:          models.ParamBindingAttributes(),
					},
					"name": schema.SingleNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "name"),
						Attributes:          models.AutoGeneratedParamBindingAttributes(),
					},
					"severity": schema.SingleNestedAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "severity"),
						PlanModifiers: []planmodifier.Object{
							objectplanmodifier.UseStateForUnknown(),
						},
						Attributes: map[string]schema.Attribute{
							"binding": schema.SingleNestedAttribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteSeverityBindingV2", "binding"),
								Attributes:          models.ParamBindingAttributes(),
							},
							"merge_strategy": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: EnumValuesDescription("AlertRouteSeverityBindingV2", "merge_strategy"),
							},
						},
					},
					"start_in_triage": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "start_in_triage"),
						Attributes:          models.ParamBindingAttributes(),
					},
					"summary": schema.SingleNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "summary"),
						Attributes:          models.AutoGeneratedParamBindingAttributes(),
					},
					"workspace": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "workspace"),
						Attributes:          models.ParamBindingAttributes(),
					},
				},
			},
		},
	}
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

func (r *IncidentAlertRouteResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
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

		// If it has branches, root_reference must be "." or empty.
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

func (r *IncidentAlertRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AlertRouteResourceModel
	var plan models.AlertRouteResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := data.ToCreatePayload()

	result, err := r.client.AlertRoutesV2CreateWithResponse(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert route, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON201.AlertRoute.Id, resp.Diagnostics, client.AlertRoute, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("Created an alert route with id=%s", result.JSON201.AlertRoute.Id))

	data = models.AlertRouteResourceModel{}.FromAPIWithPlan(result.JSON201.AlertRoute, &plan)
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

	result, err := r.client.AlertRoutesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Alert route with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route, got error: %s", err))
		return
	}

	data = models.AlertRouteResourceModel{}.FromAPIWithPlan(result.JSON200.AlertRoute, &state)
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

	result, err := r.client.AlertRoutesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		// Check if error message contains any indication of a 404 not found
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

	updateResult, err := r.client.AlertRoutesV2UpdateWithResponse(ctx, data.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert route, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON200.AlertRoute.Id, resp.Diagnostics, client.AlertRoute, r.terraformVersion)

	data = models.AlertRouteResourceModel{}.FromAPIWithPlan(updateResult.JSON200.AlertRoute, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AlertRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.AlertRoutesV2DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert route, got error: %s", err))
		return
	}
}

func (r *IncidentAlertRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResource(ctx, r.client, req.ID, resp.Diagnostics, client.AlertRoute, r.terraformVersion)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
