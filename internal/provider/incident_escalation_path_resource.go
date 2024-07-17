package provider

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oklog/ulid/v2"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentEscalationPathResource{}
	_ resource.ResourceWithImportState = &IncidentEscalationPathResource{}
)

type IncidentEscalationPathResource struct {
	client *client.ClientWithResponses
}

type IncidentEscalationPathResourceModel struct {
	ID           types.String                    `tfsdk:"id"`
	Name         types.String                    `tfsdk:"name"`
	Path         []IncidentEscalationPathNode    `tfsdk:"path"`
	WorkingHours []IncidentWeekdayIntervalConfig `tfsdk:"working_hours"`
}

type IncidentEscalationPathNode struct {
	ID     types.String                      `tfsdk:"id"`
	Type   types.String                      `tfsdk:"type"`
	IfElse *IncidentEscalationPathNodeIfElse `tfsdk:"if_else"`
	Level  *IncidentEscalationPathNodeLevel  `tfsdk:"level"`
	Repeat *IncidentEscalationPathNodeRepeat `tfsdk:"repeat"`
}

type IncidentEscalationPathNodeIfElse struct {
	Conditions []IncidentEngineCondition    `tfsdk:"conditions"`
	ElsePath   []IncidentEscalationPathNode `tfsdk:"else_path"`
	ThenPath   []IncidentEscalationPathNode `tfsdk:"then_path"`
}

type IncidentEscalationPathNodeLevel struct {
	Targets                          []IncidentEscalationPathTarget      `tfsdk:"targets"`
	RoundRobinConfig                 *IncidentEscalationRoundRobinConfig `tfsdk:"round_robin_config"`
	TimeToAckIntervalCondition       types.String                        `tfsdk:"time_to_ack_interval_condition"`
	TimeToAckSeconds                 types.Int64                         `tfsdk:"time_to_ack_seconds"`
	TimeToAckWeekdayIntervalConfigID types.String                        `tfsdk:"time_to_ack_weekday_interval_config_id"`
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
		MarkdownDescription: apischema.TagDocstring("Escalations V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2ResponseBody", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2ResponseBody", "name"),
				Required:            true,
			},
			"path": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2ResponseBody", "path"),
				Required:            true,
				NestedObject:        r.getPathSchema(3),
			},
			"working_hours": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2ResponseBody", "working_hours"),
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: IncidentWeekdayIntervalConfig{}.Attributes(),
				},
			},
		},
	}
}

// Terraform doesn't support recursive schemas so we have to manually unpack the schema to
// a finite depth to allow recursing back into our nodes.
func (r *IncidentEscalationPathResource) getPathSchema(depth int) schema.NestedAttributeObject {
	result := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2ResponseBody", "id"),
				Computed:            true,
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2ResponseBody", "type"),
				Required:            true,
			},
			"level": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2ResponseBody", "level"),
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"targets": schema.ListNestedAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2ResponseBody", "targets"),
						Required:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2ResponseBody", "id"),
									Required:            true,
								},
								"type": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2ResponseBody", "type"),
									Required:            true,
								},
								"urgency": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2ResponseBody", "urgency"),
									Required:            true,
								},
								"schedule_mode": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2ResponseBody", "schedule_mode"),
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
								Required: true,
							},
							"rotate_after_seconds": schema.Int64Attribute{
								Optional: true,
							},
						},
					},
					"time_to_ack_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2ResponseBody", "time_to_ack_seconds"),
						Optional:            true,
					},
					"time_to_ack_interval_condition": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeLevelV2ResponseBody", "time_to_ack_interval_condition"),
						Optional: true,
					},
					"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeLevelV2ResponseBody", "time_to_ack_weekday_interval_config_id"),
						Optional: true,
					},
				},
			},
			"repeat": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2ResponseBody", "repeat"),
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"repeat_times": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2ResponseBody", "repeat_times"),
						Required:            true,
					},
					"to_node": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2ResponseBody", "to_node"),
						Required:            true,
					},
				},
			},
		},
	}

	if depth > 0 {
		result.Attributes["if_else"] = schema.SingleNestedAttribute{
			MarkdownDescription: apischema.Docstring("EscalationPathNodeV2ResponseBody", "if_else"),
			Optional:            true,
			Attributes: map[string]schema.Attribute{
				"conditions": conditionsAttribute,
				"else_path": schema.ListNestedAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathNodeIfElseV2ResponseBody", "else_path"),
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

	result, err := r.client.EscalationsV2CreatePathWithResponse(ctx, client.EscalationsV2CreatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         r.toPathPayload(data.Path),
		WorkingHours: workingHours,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create escalation path, got error: %s", err))
		return
	}

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

	result, err := r.client.EscalationsV2UpdatePathWithResponse(ctx, data.ID.ValueString(), client.EscalationsV2UpdatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         r.toPathPayload(data.Path),
		WorkingHours: workingHours,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update escalation path, got error: %s", err))
		return
	}

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
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentEscalationPathResource) buildModel(ep client.EscalationPathV2) *IncidentEscalationPathResourceModel {
	var workingHours []IncidentWeekdayIntervalConfig
	if ep.WorkingHours != nil {
		workingHours = lo.Map(*ep.WorkingHours, func(wh client.WeekdayIntervalConfigV2, _ int) IncidentWeekdayIntervalConfig {
			return IncidentWeekdayIntervalConfig{}.FromClientV2(wh)
		})
	}

	return &IncidentEscalationPathResourceModel{
		ID:           types.StringValue(ep.Id),
		Name:         types.StringValue(ep.Name),
		Path:         r.toPathModel(ep.Path),
		WorkingHours: workingHours,
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
				Conditions: lo.Map(node.IfElse.Conditions, func(cond client.ConditionV2, _ int) IncidentEngineCondition {
					return IncidentEngineCondition{
						Subject:   types.StringValue(cond.Subject.Reference),
						Operation: types.StringValue(cond.Operation.Value),
						ParamBindings: lo.Map(cond.ParamBindings, func(pb client.EngineParamBindingV2, _ int) IncidentEngineParamBinding {
							return IncidentEngineParamBinding{}.FromClientV2(pb)
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
						return IncidentEscalationPathTarget{
							ID:           types.StringValue(target.Id),
							Type:         types.StringValue(string(target.Type)),
							Urgency:      types.StringValue(string(target.Urgency)),
							ScheduleMode: types.StringValue(string(*target.ScheduleMode)),
						}
					}),
			}
			if value := node.Level.RoundRobinConfig; value != nil {
				elem.Level.RoundRobinConfig = &IncidentEscalationRoundRobinConfig{
					Enabled:            types.BoolValue(value.Enabled),
					RotateAfterSeconds: types.Int64Value(*value.RotateAfterSeconds),
				}
			}
			if value := node.Level.TimeToAckSeconds; value != nil {
				elem.Level.TimeToAckSeconds = types.Int64Value(*value)
			}
			if value := node.Level.TimeToAckIntervalCondition; value != nil {
				elem.Level.TimeToAckIntervalCondition = types.StringValue(
					string(*node.Level.TimeToAckIntervalCondition))
			}
			if value := node.Level.TimeToAckWeekdayIntervalConfigId; value != nil && *value != "" {
				elem.Level.TimeToAckWeekdayIntervalConfigID = types.StringValue(*value)
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
				Conditions: lo.ToPtr(toPayloadConditions(node.IfElse.Conditions)),
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
					return client.EscalationPathTargetV2{
						Id:           target.ID.ValueString(),
						Type:         client.EscalationPathTargetV2Type(target.Type.ValueString()),
						Urgency:      client.EscalationPathTargetV2Urgency(target.Urgency.ValueString()),
						ScheduleMode: lo.ToPtr(client.EscalationPathTargetV2ScheduleMode(target.ScheduleMode.ValueString())),
					}
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
