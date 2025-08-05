package provider

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oklog/ulid/v2"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ resource.Resource                = &IncidentEscalationPathResource{}
	_ resource.ResourceWithImportState = &IncidentEscalationPathResource{}
)

type IncidentEscalationPathResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

type IncidentEscalationPathResourceModel struct {
	ID           types.String                           `tfsdk:"id"`
	Name         types.String                           `tfsdk:"name"`
	Path         []IncidentEscalationPathNode           `tfsdk:"path"`
	WorkingHours []models.IncidentWeekdayIntervalConfig `tfsdk:"working_hours"`
	TeamIDs      types.Set                              `tfsdk:"team_ids"`
}

type IncidentEscalationPathNode struct {
	ID            types.String                             `tfsdk:"id"`
	Type          types.String                             `tfsdk:"type"`
	IfElse        *IncidentEscalationPathNodeIfElse        `tfsdk:"if_else"`
	Level         *IncidentEscalationPathNodeLevel         `tfsdk:"level"`
	Repeat        *IncidentEscalationPathNodeRepeat        `tfsdk:"repeat"`
	NotifyChannel *IncidentEscalationPathNodeNotifyChannel `tfsdk:"notify_channel"`
}

type IncidentEscalationPathNodeIfElse struct {
	Conditions models.IncidentEngineConditions `tfsdk:"conditions"`
	ElsePath   []IncidentEscalationPathNode    `tfsdk:"else_path"`
	ThenPath   []IncidentEscalationPathNode    `tfsdk:"then_path"`
}

type IncidentEscalationPathNodeLevel struct {
	Targets                          []IncidentEscalationPathTarget      `tfsdk:"targets"`
	RoundRobinConfig                 *IncidentEscalationRoundRobinConfig `tfsdk:"round_robin_config"`
	TimeToAckIntervalCondition       types.String                        `tfsdk:"time_to_ack_interval_condition"`
	TimeToAckSeconds                 types.Int64                         `tfsdk:"time_to_ack_seconds"`
	TimeToAckWeekdayIntervalConfigID types.String                        `tfsdk:"time_to_ack_weekday_interval_config_id"`

	AckMode types.String `tfsdk:"ack_mode"`
}

type IncidentEscalationPathNodeNotifyChannel struct {
	Targets                          []IncidentEscalationPathTarget `tfsdk:"targets"`
	TimeToAckIntervalCondition       types.String                   `tfsdk:"time_to_ack_interval_condition"`
	TimeToAckSeconds                 types.Int64                    `tfsdk:"time_to_ack_seconds"`
	TimeToAckWeekdayIntervalConfigID types.String                   `tfsdk:"time_to_ack_weekday_interval_config_id"`
}

type IncidentEscalationPathNodeRepeat struct {
	RepeatTimes types.Int64  `tfsdk:"repeat_times"`
	ToNode      types.String `tfsdk:"to_node"`
}

type IncidentEscalationRoundRobinConfig struct {
	Enabled            types.Bool  `tfsdk:"enabled"`
	RotateAfterSeconds types.Int64 `tfsdk:"rotate_after_seconds"`
}

type IncidentEscalationPathTarget struct {
	ID           types.String `tfsdk:"id"`
	Type         types.String `tfsdk:"type"`
	Urgency      types.String `tfsdk:"urgency"`
	ScheduleMode types.String `tfsdk:"schedule_mode"`
}

func NewIncidentEscalationPathResource() resource.Resource {
	return &IncidentEscalationPathResource{}
}

func (r *IncidentEscalationPathResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path"
}

func (r *IncidentEscalationPathResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Escalations V2"), `We'd generally recommend building escalation paths in our [web dashboard](https://app.incident.io/~/on-call/escalation-paths), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing escalation path and copy the resulting Terraform without persisting it.`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "name"),
				Required:            true,
			},
			"path": schema.ListNestedAttribute{
				MarkdownDescription: fmt.Sprintf("%s\n%s",
					apischema.Docstring("EscalationPathV2", "path"),
					"\n-->**Note** Although the `if_else` block is recursive, currently a maximum of 3 levels are supported. "+
						"Attempting to configure more than 3 levels of nesting will result in a schema error.\n"),
				Required:     true,
				NestedObject: r.getPathSchema(4),
			},
			"working_hours": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "working_hours"),
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: models.IncidentWeekdayIntervalConfig{}.Attributes(),
				},
			},
			"team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "team_ids"),
				Optional:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

// Terraform doesn't support recursive schemas so we have to manually unpack the schema to
// a finite depth to allow recursing back into our nodes.
//
// We support a maximum nesting depth of 3 levels of if_else nodes.
// The schema definition should use a depth of 4 if we want to support 3 levels of
// nesting, as it's zero-indexed.
func (r *IncidentEscalationPathResource) getPathSchema(depth int) schema.NestedAttributeObject {
	result := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "id"),
				Computed:            true,
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "type"),
				Required:            true,
			},
			"level": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "level"),
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"targets": schema.ListNestedAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "targets"),
						Required:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "id"),
									Required:            true,
								},
								"type": schema.StringAttribute{
									MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "type"),
									Required:            true,
								},
								"urgency": schema.StringAttribute{
									MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "urgency"),
									Required:            true,
								},
								"schedule_mode": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "schedule_mode"),
									Optional:            true,
									Computed:            true,
								},
							},
						},
					},
					"round_robin_config": schema.SingleNestedAttribute{
						Optional: true,
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								MarkdownDescription: apischema.Docstring("EscalationPathRoundRobinConfigV2", "enabled"),
								Required:            true,
							},
							"rotate_after_seconds": schema.Int64Attribute{
								MarkdownDescription: apischema.Docstring("EscalationPathRoundRobinConfigV2", "rotate_after_seconds"),
								Optional:            true,
							},
						},
					},
					"time_to_ack_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_seconds"),
						Optional:            true,
					},
					"time_to_ack_interval_condition": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeLevelV2", "time_to_ack_interval_condition"),
						Optional: true,
					},
					"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeLevelV2", "time_to_ack_weekday_interval_config_id"),
						Optional: true,
					},
					"ack_mode": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeLevelV2", "ack_mode"),
						Optional: true,
						Computed: true,
						Default:  stringdefault.StaticString("all"),
					},
				},
			},
			"repeat": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "repeat"),
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"repeat_times": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2", "repeat_times"),
						Required:            true,
					},
					"to_node": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2", "to_node"),
						Required:            true,
					},
				},
			},
			"notify_channel": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "notify_channel"),
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"targets": schema.ListNestedAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "targets"),
						Required:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "id"),
									Required:            true,
								},
								"type": schema.StringAttribute{
									MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "type"),
									Required:            true,
								},
								"urgency": schema.StringAttribute{
									MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "urgency"),
									Required:            true,
								},
								"schedule_mode": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "schedule_mode"),
									Optional:            true,
									Computed:            true,
								},
							},
						},
					},
					"time_to_ack_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "time_to_ack_seconds"),
						Optional:            true,
					},
					"time_to_ack_interval_condition": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeNotifyChannelV2", "time_to_ack_interval_condition"),
						Optional: true,
					},
					"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeNotifyChannelV2", "time_to_ack_weekday_interval_config_id"),
						Optional: true,
					},
				},
			},
		},
	}

	// Only include if_else attribute if we haven't reached the maximum nesting depth (3 levels)
	if depth > 0 {
		result.Attributes["if_else"] = schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "if_else"),
			Optional:            true,
			Attributes: map[string]schema.Attribute{
				"conditions": models.ConditionsAttribute(),
				"else_path": schema.ListNestedAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathNodeIfElseV2", "else_path"),
					Optional:            true,
					NestedObject:        r.getPathSchema(depth - 1),
				},
				"then_path": schema.ListNestedAttribute{
					MarkdownDescription: "Then path nodes",
					Required:            true,
					NestedObject:        r.getPathSchema(depth - 1),
				},
			},
		}
	}

	return result
}

func (r *IncidentEscalationPathResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentEscalationPathResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var workingHours *[]client.WeekdayIntervalConfigV2
	if len(data.WorkingHours) > 0 {
		workingHours = &[]client.WeekdayIntervalConfigV2{}
		for _, wh := range data.WorkingHours {
			*workingHours = append(*workingHours, wh.ToClientV2())
		}
	}

	var teamIDs *[]string
	if !data.TeamIDs.IsUnknown() && !data.TeamIDs.IsNull() {
		ids := []string{}
		diags := data.TeamIDs.ElementsAs(ctx, &ids, false)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		teamIDs = &ids
	}

	result, err := r.client.EscalationsV2CreatePathWithResponse(ctx, client.EscalationsV2CreatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         r.toPathPayload(data.Path),
		WorkingHours: workingHours,
		TeamIds:      teamIDs,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create escalation path, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON201.EscalationPath.Id, resp.Diagnostics, client.EscalationPath, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("created an escalation path resource with id=%s", result.JSON201.EscalationPath.Id))
	data = r.buildModel(result.JSON201.EscalationPath)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentEscalationPathResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationsV2ShowPathWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path, got error: %s", err))
		return
	}

	if result.StatusCode() == 404 {
		resp.Diagnostics.AddWarning("Not Found", fmt.Sprintf("Unable to read escalation path, got status code: %d", result.StatusCode()))
		resp.State.RemoveResource(ctx)
		return
	}

	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path, got status code: %d", result.StatusCode()))
		return
	}

	data = r.buildModel(result.JSON200.EscalationPath)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentEscalationPathResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var workingHours *[]client.WeekdayIntervalConfigV2
	if len(data.WorkingHours) > 0 {
		workingHours = &[]client.WeekdayIntervalConfigV2{}
		for _, wh := range data.WorkingHours {
			*workingHours = append(*workingHours, wh.ToClientV2())
		}
	}

	var teamIDs *[]string
	if !data.TeamIDs.IsUnknown() && !data.TeamIDs.IsNull() {
		ids := []string{}
		diags := data.TeamIDs.ElementsAs(ctx, &ids, false)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		teamIDs = &ids
	}

	result, err := r.client.EscalationsV2UpdatePathWithResponse(ctx, data.ID.ValueString(), client.EscalationsV2UpdatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         r.toPathPayload(data.Path),
		WorkingHours: workingHours,
		TeamIds:      teamIDs,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update escalation path, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON200.EscalationPath.Id, resp.Diagnostics, client.EscalationPath, r.terraformVersion)

	data = r.buildModel(result.JSON200.EscalationPath)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentEscalationPathResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.EscalationsV2DestroyPathWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete escalation path, got error: %s", err))
		return
	}
}

func (r *IncidentEscalationPathResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResource(ctx, r.client, req.ID, resp.Diagnostics, client.EscalationPath, r.terraformVersion)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentEscalationPathResource) buildModel(ep client.EscalationPathV2) *IncidentEscalationPathResourceModel {
	var workingHours []models.IncidentWeekdayIntervalConfig
	if ep.WorkingHours != nil {
		workingHours = lo.Map(*ep.WorkingHours, func(wh client.WeekdayIntervalConfigV2, _ int) models.IncidentWeekdayIntervalConfig {
			return models.IncidentWeekdayIntervalConfig{}.FromClientV2(wh)
		})
	}

	var teamIDsSet types.Set
	if ep.TeamIds != nil {
		if len(ep.TeamIds) > 0 {
			elements := make([]attr.Value, len(ep.TeamIds))
			for i, id := range ep.TeamIds {
				elements[i] = types.StringValue(id)
			}
			teamIDsSet = types.SetValueMust(types.StringType, elements)
		} else {
			teamIDsSet = types.SetValueMust(types.StringType, []attr.Value{})
		}
	} else {
		teamIDsSet = types.SetNull(types.StringType)
	}

	return &IncidentEscalationPathResourceModel{
		ID:           types.StringValue(ep.Id),
		Name:         types.StringValue(ep.Name),
		Path:         r.toPathModel(ep.Path),
		WorkingHours: workingHours,
		TeamIDs:      teamIDsSet,
	}
}

func (r *IncidentEscalationPathResource) toPathModel(nodes []client.EscalationPathNodeV2) []IncidentEscalationPathNode {
	out := []IncidentEscalationPathNode{}
	for _, node := range nodes {
		elem := IncidentEscalationPathNode{
			ID:   types.StringValue(node.Id),
			Type: types.StringValue(string(node.Type)),
		}
		if node.IfElse != nil {
			elem.IfElse = &IncidentEscalationPathNodeIfElse{
				Conditions: lo.Map(node.IfElse.Conditions, func(cond client.ConditionV2, _ int) models.IncidentEngineCondition {
					return models.IncidentEngineCondition{
						Subject:   types.StringValue(cond.Subject.Reference),
						Operation: types.StringValue(cond.Operation.Value),
						ParamBindings: lo.Map(cond.ParamBindings, func(pb client.EngineParamBindingV2, _ int) models.IncidentEngineParamBinding {
							return models.IncidentEngineParamBinding{}.FromAPI(pb)
						}),
					}
				}),
				ThenPath: r.toPathModel(node.IfElse.ThenPath),
				ElsePath: r.toPathModel(node.IfElse.ElsePath),
			}
		}
		if node.Level != nil {
			elem.Level = &IncidentEscalationPathNodeLevel{
				Targets: lo.Map(node.Level.Targets,
					func(target client.EscalationPathTargetV2, _ int) IncidentEscalationPathTarget {
						scheduleMode := types.StringNull()
						if target.ScheduleMode != nil {
							scheduleMode = types.StringValue(string(*target.ScheduleMode))
						}

						return IncidentEscalationPathTarget{
							ID:           types.StringValue(target.Id),
							Type:         types.StringValue(string(target.Type)),
							Urgency:      types.StringValue(string(target.Urgency)),
							ScheduleMode: scheduleMode,
						}
					}),
			}
			if value := node.Level.RoundRobinConfig; value != nil {
				var rotateAfterSeconds basetypes.Int64Value
				if value.RotateAfterSeconds != nil {
					rotateAfterSeconds = types.Int64Value(*value.RotateAfterSeconds)
				}
				elem.Level.RoundRobinConfig = &IncidentEscalationRoundRobinConfig{
					Enabled:            types.BoolValue(value.Enabled),
					RotateAfterSeconds: rotateAfterSeconds,
				}
			}
			if value := node.Level.TimeToAckSeconds; value != nil {
				elem.Level.TimeToAckSeconds = types.Int64Value(*value)
			}
			if value := node.Level.TimeToAckIntervalCondition; value != nil {
				elem.Level.TimeToAckIntervalCondition = types.StringValue(string(*value))
			}
			if value := node.Level.TimeToAckWeekdayIntervalConfigId; value != nil && *value != "" {
				elem.Level.TimeToAckWeekdayIntervalConfigID = types.StringValue(*value)
			}
			if value := node.Level.AckMode; value != nil {
				elem.Level.AckMode = types.StringValue(string(*value))
			}
		}
		if node.NotifyChannel != nil {
			elem.NotifyChannel = &IncidentEscalationPathNodeNotifyChannel{
				Targets: lo.Map(node.NotifyChannel.Targets,
					func(target client.EscalationPathTargetV2, _ int) IncidentEscalationPathTarget {
						scheduleMode := types.StringNull()
						if target.ScheduleMode != nil {
							scheduleMode = types.StringValue(string(*target.ScheduleMode))
						}

						return IncidentEscalationPathTarget{
							ID:           types.StringValue(target.Id),
							Type:         types.StringValue(string(target.Type)),
							Urgency:      types.StringValue(string(target.Urgency)),
							ScheduleMode: scheduleMode,
						}
					}),
			}
			if value := node.NotifyChannel.TimeToAckSeconds; value != nil {
				elem.NotifyChannel.TimeToAckSeconds = types.Int64Value(*value)
			}
			if value := node.NotifyChannel.TimeToAckIntervalCondition; value != nil {
				elem.NotifyChannel.TimeToAckIntervalCondition = types.StringValue(string(*value))
			}
			if value := node.NotifyChannel.TimeToAckWeekdayIntervalConfigId; value != nil && *value != "" {
				elem.NotifyChannel.TimeToAckWeekdayIntervalConfigID = types.StringValue(*value)
			}
		}
		if node.Repeat != nil {
			elem.Repeat = &IncidentEscalationPathNodeRepeat{
				RepeatTimes: types.Int64Value(node.Repeat.RepeatTimes),
				ToNode:      types.StringValue(node.Repeat.ToNode),
			}
		}

		out = append(out, elem)
	}

	return out
}

func (r *IncidentEscalationPathResource) toPathPayload(path []IncidentEscalationPathNode) []client.EscalationPathNodePayloadV2 {
	out := []client.EscalationPathNodePayloadV2{}
	for _, node := range path {
		nodeID := node.ID.ValueString()
		if nodeID == "" {
			nodeID = ulid.Make().String()
		}

		elem := client.EscalationPathNodePayloadV2{
			Id:   nodeID,
			Type: client.EscalationPathNodePayloadV2Type(node.Type.ValueString()),
		}
		if !reflect.ValueOf(node.IfElse).IsZero() {
			elem.IfElse = &client.EscalationPathNodeIfElsePayloadV2{
				Conditions: lo.ToPtr(node.IfElse.Conditions.ToPayload()),
				ThenPath:   r.toPathPayload(node.IfElse.ThenPath),
				ElsePath:   r.toPathPayload(node.IfElse.ElsePath),
			}
		}
		if !reflect.ValueOf(node.Level).IsZero() {
			var intervalCondition *client.EscalationPathNodeLevelV2TimeToAckIntervalCondition
			if value := node.Level.TimeToAckIntervalCondition.ValueStringPointer(); value != nil {
				intervalCondition = lo.ToPtr(client.EscalationPathNodeLevelV2TimeToAckIntervalCondition(*value))
			}

			elem.Level = &client.EscalationPathNodeLevelV2{
				Targets: lo.Map(node.Level.Targets, func(target IncidentEscalationPathTarget, _ int) client.EscalationPathTargetV2 {
					targetPayload := client.EscalationPathTargetV2{
						Id:      target.ID.ValueString(),
						Type:    client.EscalationPathTargetV2Type(target.Type.ValueString()),
						Urgency: client.EscalationPathTargetV2Urgency(target.Urgency.ValueString()),
					}

					if target.ScheduleMode.ValueString() != "" {
						targetPayload.ScheduleMode = lo.ToPtr(client.EscalationPathTargetV2ScheduleMode(target.ScheduleMode.ValueString()))
					}

					return targetPayload
				}),
				TimeToAckIntervalCondition: intervalCondition,
				TimeToAckSeconds: node.Level.
					TimeToAckSeconds.ValueInt64Pointer(),
				TimeToAckWeekdayIntervalConfigId: node.Level.
					TimeToAckWeekdayIntervalConfigID.ValueStringPointer(),
			}

			if node.Level.RoundRobinConfig != nil {
				elem.Level.RoundRobinConfig = &client.EscalationPathRoundRobinConfigV2{
					Enabled:            node.Level.RoundRobinConfig.Enabled.ValueBool(),
					RotateAfterSeconds: node.Level.RoundRobinConfig.RotateAfterSeconds.ValueInt64Pointer(),
				}
			}

			if !node.Level.AckMode.IsNull() {
				val := node.Level.AckMode.ValueString()

				ptr := client.EscalationPathNodeLevelV2AckMode(val)
				elem.Level.AckMode = &ptr
			}
		}
		if !reflect.ValueOf(node.NotifyChannel).IsZero() {
			var intervalCondition *client.EscalationPathNodeNotifyChannelV2TimeToAckIntervalCondition
			if value := node.NotifyChannel.TimeToAckIntervalCondition.ValueStringPointer(); value != nil {
				intervalCondition = lo.ToPtr(client.EscalationPathNodeNotifyChannelV2TimeToAckIntervalCondition(*value))
			}

			elem.NotifyChannel = &client.EscalationPathNodeNotifyChannelV2{
				Targets: lo.Map(node.NotifyChannel.Targets, func(target IncidentEscalationPathTarget, _ int) client.EscalationPathTargetV2 {
					targetPayload := client.EscalationPathTargetV2{
						Id:      target.ID.ValueString(),
						Type:    client.EscalationPathTargetV2Type(target.Type.ValueString()),
						Urgency: client.EscalationPathTargetV2Urgency(target.Urgency.ValueString()),
					}

					if target.ScheduleMode.ValueString() != "" {
						targetPayload.ScheduleMode = lo.ToPtr(client.EscalationPathTargetV2ScheduleMode(target.ScheduleMode.ValueString()))
					}

					return targetPayload
				}),
				TimeToAckIntervalCondition: intervalCondition,
				TimeToAckSeconds: node.NotifyChannel.
					TimeToAckSeconds.ValueInt64Pointer(),
				TimeToAckWeekdayIntervalConfigId: node.NotifyChannel.
					TimeToAckWeekdayIntervalConfigID.ValueStringPointer(),
			}
		}
		if !reflect.ValueOf(node.Repeat).IsZero() {
			elem.Repeat = &client.EscalationPathNodeRepeatV2{
				RepeatTimes: node.Repeat.RepeatTimes.ValueInt64(),
				ToNode:      node.Repeat.ToNode.ValueString(),
			}
		}

		out = append(out, elem)
	}

	return out
}
