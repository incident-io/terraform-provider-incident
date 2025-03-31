package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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

func (r *IncidentAlertRouteResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.AlertRouteResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func NewIncidentAlertRouteResource() resource.Resource {
	return &IncidentAlertRouteResource{}
}

func (r *IncidentAlertRouteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_route"
}

func (r *IncidentAlertRouteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Alert Routes V2"),
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
								"id": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "ID of the grouping key",
								},
								"reference": schema.SingleNestedAttribute{
									Required:            true,
									MarkdownDescription: "Reference to the attribute",
									Attributes: map[string]schema.Attribute{
										"array": schema.BoolAttribute{
											Required:            true,
											MarkdownDescription: "Whether this is an array",
										},
										"key": schema.StringAttribute{
											Required:            true,
											MarkdownDescription: "Key of the attribute",
										},
										"label": schema.StringAttribute{
											Required:            true,
											MarkdownDescription: "Label for the attribute",
										},
										"type": schema.StringAttribute{
											Required:            true,
											MarkdownDescription: "Type of the attribute",
										},
									},
								},
							},
						},
					},
					"defer_time_seconds": schema.Int64Attribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentConfigV2", "defer_time_seconds"),
					},
				},
			},
			"incident_template": schema.SingleNestedAttribute{
				Optional:            true,
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
							},
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
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "name"),
						Attributes:          models.ParamBindingAttributes(),
					},
					"name_autogenerated": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "name_autogenerated"),
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"severity": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "severity"),
						Attributes: map[string]schema.Attribute{
							"binding": schema.SingleNestedAttribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteSeverityBindingV2", "binding"),
								Attributes:          models.ParamBindingAttributes(),
							},
							"merge_strategy": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: apischema.Docstring("AlertRouteSeverityBindingV2", "merge_strategy"),
							},
						},
					},
					"start_in_triage": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "start_in_triage"),
						Attributes:          models.ParamBindingAttributes(),
					},
					"summary": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "summary"),
						Attributes:          models.ParamBindingAttributes(),
					},
					"summary_autogenerated": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertRouteIncidentTemplateV2", "summary_autogenerated"),
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
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

func (r *IncidentAlertRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AlertRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert the resource data to an API payload
	payload := data.ToCreatePayload()

	// Call the API to create the resource
	result, err := r.client.AlertRoutesV2CreateWithResponse(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert route, got error: %s", err))
		return
	}

	// Log the creation
	tflog.Trace(ctx, fmt.Sprintf("Created an alert route with id=%s", *result.JSON201.AlertRoute.Id))

	// Update the Terraform state with the response data
	data = models.AlertRouteResourceModel{}.FromAPI(result.JSON201.AlertRoute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AlertRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Call the API to read the resource
	result, err := r.client.AlertRoutesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route, got error: %s", err))
		return
	}

	// Check if the resource is no longer found (404)
	if result.StatusCode() == 404 {
		resp.State.RemoveResource(ctx)
		return
	}

	// Check for other error status codes
	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route, got error: %s", string(result.Body)))
		return
	}

	// Update the Terraform state with the response data
	data = models.AlertRouteResourceModel{}.FromAPI(result.JSON200.AlertRoute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AlertRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// First, get the latest resource state from the API to ensure we have the current version
	result, err := r.client.AlertRoutesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route before updating, got error: %s", err))
		return
	}

	// Check for error status codes
	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert route before updating, got error: %s", string(result.Body)))
		return
	}

	// Get the current version from the API
	currentVersion := int64(0)
	if result.JSON200.AlertRoute.Version != nil {
		currentVersion = *result.JSON200.AlertRoute.Version
	}

	// Convert the resource data to an API payload
	payload := data.ToUpdatePayload()

	// Add the incremented version to the payload
	payload.Version = currentVersion + 1

	// Call the API to update the resource
	updateResult, err := r.client.AlertRoutesV2UpdateWithResponse(ctx, data.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert route, got error: %s", err))
		return
	}

	// Update the Terraform state with the response data
	data = models.AlertRouteResourceModel{}.FromAPI(updateResult.JSON200.AlertRoute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AlertRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Call the API to delete the resource
	_, err := r.client.AlertRoutesV2DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert route, got error: %s", err))
		return
	}
}

func (r *IncidentAlertRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
