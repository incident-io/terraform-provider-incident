package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                   = &IncidentEscalationPathResource{}
	_ resource.ResourceWithImportState    = &IncidentEscalationPathResource{}
	_ resource.ResourceWithValidateConfig = &IncidentEscalationPathResource{}
)

type IncidentEscalationPathResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

type IncidentEscalationPathResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Path         types.List   `tfsdk:"path"`
	WorkingHours types.List   `tfsdk:"working_hours"`
	RepeatConfig types.Object `tfsdk:"repeat_config"`
	TeamIDs      types.Set    `tfsdk:"team_ids"`
}

type IncidentEscalationPathNode struct {
	ID            types.String                             `tfsdk:"id"`
	Type          types.String                             `tfsdk:"type"`
	Delay         *IncidentEscalationPathNodeDelay         `tfsdk:"delay"`
	IfElse        *IncidentEscalationPathNodeIfElse        `tfsdk:"if_else"`
	Level         *IncidentEscalationPathNodeLevel         `tfsdk:"level"`
	Repeat        *IncidentEscalationPathNodeRepeat        `tfsdk:"repeat"`
	NotifyChannel *IncidentEscalationPathNodeNotifyChannel `tfsdk:"notify_channel"`
}

type IncidentEscalationPathNodeIfElse struct {
	Conditions models.IncidentEngineConditions `tfsdk:"conditions"`
	ElsePath   types.List                      `tfsdk:"else_path"`
	ThenPath   types.List                      `tfsdk:"then_path"`
}

type IncidentEscalationPathNodeLevel struct {
	Targets                          types.List                          `tfsdk:"targets"`
	RoundRobinConfig                 *IncidentEscalationRoundRobinConfig `tfsdk:"round_robin_config"`
	TimeToAckIntervalCondition       types.String                        `tfsdk:"time_to_ack_interval_condition"`
	TimeToAckSeconds                 types.Int64                         `tfsdk:"time_to_ack_seconds"`
	TimeToAckWeekdayIntervalConfigID types.String                        `tfsdk:"time_to_ack_weekday_interval_config_id"`

	AckMode types.String `tfsdk:"ack_mode"`
}

type IncidentEscalationPathNodeNotifyChannel struct {
	Targets                          types.List   `tfsdk:"targets"`
	TimeToAckIntervalCondition       types.String `tfsdk:"time_to_ack_interval_condition"`
	TimeToAckSeconds                 types.Int64  `tfsdk:"time_to_ack_seconds"`
	TimeToAckWeekdayIntervalConfigID types.String `tfsdk:"time_to_ack_weekday_interval_config_id"`
}

type IncidentEscalationPathNodeDelay struct {
	DelayIntervalCondition       types.String `tfsdk:"delay_interval_condition"`
	DelaySeconds                 types.Int64  `tfsdk:"delay_seconds"`
	DelayWeekdayIntervalConfigID types.String `tfsdk:"delay_weekday_interval_config_id"`
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
	ID             types.String `tfsdk:"id"`
	Type           types.String `tfsdk:"type"`
	Urgency        types.String `tfsdk:"urgency"`
	ScheduleMode   types.String `tfsdk:"schedule_mode"`
	SelectedRotaID types.String `tfsdk:"selected_rota_id"`
}

type IncidentEscalationPathRepeatConfig struct {
	RepeatAfterSeconds    types.Int64 `tfsdk:"repeat_after_seconds"`
	DelayRepeatOnActivity types.Bool  `tfsdk:"delay_repeat_on_activity"`
}

// targetAttrTypes returns the attribute types for an escalation path target
// object.
func targetAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":               types.StringType,
		"type":             types.StringType,
		"urgency":          types.StringType,
		"schedule_mode":    types.StringType,
		"selected_rota_id": types.StringType,
	}
}

// targetListType returns the list type of escalation path targets.
func targetListType() types.ListType {
	return types.ListType{ElemType: types.ObjectType{AttrTypes: targetAttrTypes()}}
}

// nodeAttrTypes returns the attribute types for an escalation path node object
// at the given recursion depth. It MUST mirror getPathSchema exactly: the
// if_else attribute (which recurses into then_path/else_path) is only present
// when depth > 0, matching the schema.
func nodeAttrTypes(depth int) map[string]attr.Type {
	attrs := map[string]attr.Type{
		"id":   types.StringType,
		"type": types.StringType,
		"level": types.ObjectType{AttrTypes: map[string]attr.Type{
			"targets": targetListType(),
			"round_robin_config": types.ObjectType{AttrTypes: map[string]attr.Type{
				"enabled":              types.BoolType,
				"rotate_after_seconds": types.Int64Type,
			}},
			"time_to_ack_seconds":                    types.Int64Type,
			"time_to_ack_interval_condition":         types.StringType,
			"time_to_ack_weekday_interval_config_id": types.StringType,
			"ack_mode":                               types.StringType,
		}},
		"repeat": types.ObjectType{AttrTypes: map[string]attr.Type{
			"repeat_times": types.Int64Type,
			"to_node":      types.StringType,
		}},
		"notify_channel": types.ObjectType{AttrTypes: map[string]attr.Type{
			"targets":                                targetListType(),
			"time_to_ack_seconds":                    types.Int64Type,
			"time_to_ack_interval_condition":         types.StringType,
			"time_to_ack_weekday_interval_config_id": types.StringType,
		}},
		"delay": types.ObjectType{AttrTypes: map[string]attr.Type{
			"delay_seconds":                    types.Int64Type,
			"delay_interval_condition":         types.StringType,
			"delay_weekday_interval_config_id": types.StringType,
		}},
	}

	if depth > 0 {
		attrs["if_else"] = types.ObjectType{AttrTypes: map[string]attr.Type{
			"conditions": types.ListType{
				ElemType: types.ObjectType{AttrTypes: models.ConditionAttrTypes()},
			},
			"else_path": nodeListType(depth - 1),
			"then_path": nodeListType(depth - 1),
		}}
	}

	return attrs
}

// nodeListType returns the list type of escalation path nodes at the given depth.
func nodeListType(depth int) types.ListType {
	return types.ListType{ElemType: types.ObjectType{AttrTypes: nodeAttrTypes(depth)}}
}

// pathSchemaDepth is the maximum if_else nesting depth supported by the schema.
// The schema is built with this depth (zero-indexed), supporting 5 levels of
// if_else nesting.
const pathSchemaDepth = 5

func NewIncidentEscalationPathResource() resource.Resource {
	return &IncidentEscalationPathResource{}
}

func (r *IncidentEscalationPathResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path"
}

func (r *IncidentEscalationPathResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", "Create and manage escalation paths.", `We'd generally recommend building escalation paths in our [web dashboard](https://app.incident.io/~/on-call/escalation-paths), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing escalation path and copy the resulting Terraform without persisting it.`),
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
					"\n-->**Note** Although the `if_else` block is recursive, currently a maximum of 5 levels are supported. "+
						"Attempting to configure more than 5 levels of nesting will result in a validation error.\n"),
				Required:     true,
				NestedObject: r.getPathSchema(pathSchemaDepth),
			},
			"working_hours": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "working_hours"),
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: models.IncidentWeekdayIntervalConfig{}.Attributes(),
				},
			},
			"repeat_config": schema.SingleNestedAttribute{
				MarkdownDescription: "Controls if an escalation will repeat after acknowledgement, when the alert is unresolved. When configured, it will repeat after the specified delay.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"repeat_after_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRepeatConfigV2", "repeat_after_seconds"),
						Required:            true,
					},
					"delay_repeat_on_activity": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRepeatConfigV2", "delay_repeat_on_activity"),
						Required:            true,
					},
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
// We support a maximum nesting depth of 4 levels of if_else nodes.
// The schema definition should use a depth of 5 if we want to support 4 levels of
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
								"selected_rota_id": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "selected_rota_id"),
									Optional:            true,
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
								"selected_rota_id": schema.StringAttribute{
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "selected_rota_id"),
									Optional:            true,
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
			"delay": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "delay"),
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"delay_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeDelayV2", "delay_seconds"),
						Optional:            true,
					},
					"delay_interval_condition": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeDelayV2", "delay_interval_condition"),
						Optional: true,
					},
					"delay_weekday_interval_config_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring(
							"EscalationPathNodeDelayV2", "delay_weekday_interval_config_id"),
						Optional: true,
					},
				},
			},
		},
	}

	// Only include if_else attribute if we haven't reached the maximum nesting depth (5 levels)
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

func (r *IncidentEscalationPathResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	validateEscalationPathNodes(ctx, data.Path, pathSchemaDepth, &resp.Diagnostics)
}

// decodeNodes decodes a types.List of escalation path node objects into the
// Go model structs. It returns nil if the list is null or unknown.
//
// It decodes each element attribute by attribute rather than reflecting the
// whole struct: at the maximum nesting depth the object type omits if_else
// (mirroring the finite schema), but the IncidentEscalationPathNode struct
// always carries an if_else field, so ElementsAs would fail with a struct/object
// mismatch. Reading attributes explicitly lets us decode if_else only when the
// object actually carries it.
func decodeNodes(ctx context.Context, list types.List, diags *diag.Diagnostics) []IncidentEscalationPathNode {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	nodes := make([]IncidentEscalationPathNode, 0, len(list.Elements()))
	for _, elem := range list.Elements() {
		obj, ok := elem.(types.Object)
		if !ok || obj.IsNull() || obj.IsUnknown() {
			continue
		}
		nodes = append(nodes, objectToNode(ctx, obj, diags))
	}
	return nodes
}

// objectToNode decodes a single escalation path node object into the Go model
// struct. The if_else attribute is only decoded when present and non-null, so
// it stays safe at the maximum nesting depth where if_else is absent.
func objectToNode(ctx context.Context, obj types.Object, diags *diag.Diagnostics) IncidentEscalationPathNode {
	attrs := obj.Attributes()
	node := IncidentEscalationPathNode{}
	if v, ok := attrs["id"].(types.String); ok {
		node.ID = v
	}
	if v, ok := attrs["type"].(types.String); ok {
		node.Type = v
	}

	decodeObject := func(key string, target any) bool {
		o, ok := attrs[key].(types.Object)
		if !ok || o.IsNull() || o.IsUnknown() {
			return false
		}
		diags.Append(o.As(ctx, target, basetypes.ObjectAsOptions{})...)
		return true
	}

	var level IncidentEscalationPathNodeLevel
	if decodeObject("level", &level) {
		node.Level = &level
	}
	var notifyChannel IncidentEscalationPathNodeNotifyChannel
	if decodeObject("notify_channel", &notifyChannel) {
		node.NotifyChannel = &notifyChannel
	}
	var delay IncidentEscalationPathNodeDelay
	if decodeObject("delay", &delay) {
		node.Delay = &delay
	}
	var repeat IncidentEscalationPathNodeRepeat
	if decodeObject("repeat", &repeat) {
		node.Repeat = &repeat
	}
	var ifElse IncidentEscalationPathNodeIfElse
	if decodeObject("if_else", &ifElse) {
		node.IfElse = &ifElse
	}

	return node
}

// decodeTargets decodes a types.List of target objects into the Go model structs.
func decodeTargets(ctx context.Context, list types.List, diags *diag.Diagnostics) []IncidentEscalationPathTarget {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var targets []IncidentEscalationPathTarget
	diags.Append(list.ElementsAs(ctx, &targets, false)...)
	return targets
}

// rotaRequiredScheduleModes is the set of schedule_mode values that require a
// selected_rota_id. Other modes must leave selected_rota_id unset.
var rotaRequiredScheduleModes = map[string]bool{
	string(client.EscalationPathTargetV2ScheduleModeAllUsersForRota):        true,
	string(client.EscalationPathTargetV2ScheduleModeCurrentlyOnCallForRota): true,
	string(client.EscalationPathTargetV2ScheduleModeNextOnCallForRota):      true,
}

// validateEscalationPathNodes walks the path validating targets, and enforces
// the maximum if_else nesting depth. depth is the remaining schema depth at this
// level (pathSchemaDepth at the top, decremented into each if_else branch).
//
// The schema only declares if_else down to pathSchemaDepth levels, so a node
// nested deeper has no if_else attribute to decode into and reaches us with
// type "if_else" but no if_else block. We reject that at plan time with a clear
// message rather than letting it through to fail at apply with an opaque API
// error about a missing if_else payload.
func validateEscalationPathNodes(ctx context.Context, nodeList types.List, depth int, diags *diag.Diagnostics) {
	nodes := decodeNodes(ctx, nodeList, diags)
	for _, node := range nodes {
		if depth <= 0 && node.Type.ValueString() == string(client.EscalationPathNodeV2TypeIfElse) {
			diags.Append(diag.NewErrorDiagnostic(
				"Escalation path nested too deeply",
				fmt.Sprintf("if_else nodes can be nested at most %d levels deep. Reduce the nesting in your escalation path.", pathSchemaDepth),
			))
			continue
		}
		if node.Level != nil {
			for _, target := range decodeTargets(ctx, node.Level.Targets, diags) {
				validateEscalationPathTarget(target, diags)
			}
		}
		if node.NotifyChannel != nil {
			for _, target := range decodeTargets(ctx, node.NotifyChannel.Targets, diags) {
				validateEscalationPathTarget(target, diags)
			}
		}
		if node.IfElse != nil {
			validateEscalationPathNodes(ctx, node.IfElse.ThenPath, depth-1, diags)
			validateEscalationPathNodes(ctx, node.IfElse.ElsePath, depth-1, diags)
		}
	}
}

func validateEscalationPathTarget(target IncidentEscalationPathTarget, diags *diag.Diagnostics) {
	if target.ScheduleMode.IsUnknown() || target.SelectedRotaID.IsUnknown() {
		return
	}

	mode := target.ScheduleMode.ValueString()
	rotaID := target.SelectedRotaID.ValueString()

	if rotaRequiredScheduleModes[mode] {
		if rotaID == "" {
			diags.Append(diag.NewErrorDiagnostic(
				"Missing selected_rota_id",
				fmt.Sprintf("Escalation path target with schedule_mode %q requires selected_rota_id to be set.", mode),
			))
		}
		return
	}

	if rotaID != "" {
		diags.Append(diag.NewErrorDiagnostic(
			"Unexpected selected_rota_id",
			fmt.Sprintf("Escalation path target with schedule_mode %q must not set selected_rota_id; it is only valid for all_users_for_rota, currently_on_call_for_rota, and next_on_call_for_rota.", mode),
		))
	}
}

func (r *IncidentEscalationPathResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var workingHours *[]client.WeekdayIntervalConfigV2
	if !data.WorkingHours.IsNull() && !data.WorkingHours.IsUnknown() {
		var whModels []models.IncidentWeekdayIntervalConfig
		resp.Diagnostics.Append(data.WorkingHours.ElementsAs(ctx, &whModels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(whModels) > 0 {
			workingHours = &[]client.WeekdayIntervalConfigV2{}
			for _, wh := range whModels {
				*workingHours = append(*workingHours, wh.ToClientV2(ctx, &resp.Diagnostics))
			}
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

	var repeatConfig *client.EscalationPathRepeatConfigV2
	if !data.RepeatConfig.IsNull() && !data.RepeatConfig.IsUnknown() {
		var rc IncidentEscalationPathRepeatConfig
		resp.Diagnostics.Append(data.RepeatConfig.As(ctx, &rc, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
		repeatConfig = &client.EscalationPathRepeatConfigV2{
			RepeatAfterSeconds:    int32(rc.RepeatAfterSeconds.ValueInt64()),
			DelayRepeatOnActivity: rc.DelayRepeatOnActivity.ValueBool(),
		}
	}

	pathPayload := r.toPathPayload(ctx, data.Path, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationsV2CreatePathWithResponse(ctx, client.EscalationsV2CreatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         pathPayload,
		WorkingHours: workingHours,
		TeamIds:      teamIDs,
		RepeatConfig: repeatConfig,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create escalation path, got error: %s", err))
		return
	}

	if result.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create escalation path: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON201.EscalationPath.Id, &resp.Diagnostics, client.EscalationPath, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("created an escalation path resource with id=%s", result.JSON201.EscalationPath.Id))
	data = r.buildModel(ctx, result.JSON201.EscalationPath, &resp.Diagnostics)
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
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Escalation path with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path, got error: %s", err))
		return
	}

	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read escalation path: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	data = r.buildModel(ctx, result.JSON200.EscalationPath, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentEscalationPathResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var workingHours *[]client.WeekdayIntervalConfigV2
	if !data.WorkingHours.IsNull() && !data.WorkingHours.IsUnknown() {
		var whModels []models.IncidentWeekdayIntervalConfig
		resp.Diagnostics.Append(data.WorkingHours.ElementsAs(ctx, &whModels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(whModels) > 0 {
			workingHours = &[]client.WeekdayIntervalConfigV2{}
			for _, wh := range whModels {
				*workingHours = append(*workingHours, wh.ToClientV2(ctx, &resp.Diagnostics))
			}
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

	var repeatConfig *client.EscalationPathRepeatConfigV2
	if !data.RepeatConfig.IsNull() && !data.RepeatConfig.IsUnknown() {
		var rc IncidentEscalationPathRepeatConfig
		resp.Diagnostics.Append(data.RepeatConfig.As(ctx, &rc, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
		repeatConfig = &client.EscalationPathRepeatConfigV2{
			RepeatAfterSeconds:    int32(rc.RepeatAfterSeconds.ValueInt64()),
			DelayRepeatOnActivity: rc.DelayRepeatOnActivity.ValueBool(),
		}
	}

	pathPayload := r.toPathPayload(ctx, data.Path, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationsV2UpdatePathWithResponse(ctx, data.ID.ValueString(), client.EscalationsV2UpdatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         pathPayload,
		WorkingHours: workingHours,
		TeamIds:      teamIDs,
		RepeatConfig: repeatConfig,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update escalation path, got error: %s", err))
		return
	}

	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to update escalation path: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON200.EscalationPath.Id, &resp.Diagnostics, client.EscalationPath, r.terraformVersion)

	data = r.buildModel(ctx, result.JSON200.EscalationPath, &resp.Diagnostics)
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
	claimResource(ctx, r.client, req.ID, &resp.Diagnostics, client.EscalationPath, r.terraformVersion)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentEscalationPathResource) buildModel(ctx context.Context, ep client.EscalationPathV2, diags *diag.Diagnostics) *IncidentEscalationPathResourceModel {
	workingHoursType := types.ObjectType{AttrTypes: models.WeekdayIntervalConfigAttrTypes()}
	workingHours := types.ListNull(workingHoursType)
	if ep.WorkingHours != nil {
		whModels := lo.Map(*ep.WorkingHours, func(wh client.WeekdayIntervalConfigV2, _ int) models.IncidentWeekdayIntervalConfig {
			return models.IncidentWeekdayIntervalConfig{}.FromClientV2(ctx, wh, diags)
		})
		list, d := types.ListValueFrom(ctx, workingHoursType, whModels)
		diags.Append(d...)
		workingHours = list
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

	repeatConfigAttrTypes := map[string]attr.Type{
		"repeat_after_seconds":     types.Int64Type,
		"delay_repeat_on_activity": types.BoolType,
	}
	var repeatConfigObj types.Object
	if ep.RepeatConfig != nil {
		repeatConfigObj = types.ObjectValueMust(repeatConfigAttrTypes, map[string]attr.Value{
			"repeat_after_seconds":     types.Int64Value(int64(ep.RepeatConfig.RepeatAfterSeconds)),
			"delay_repeat_on_activity": types.BoolValue(ep.RepeatConfig.DelayRepeatOnActivity),
		})
	} else {
		repeatConfigObj = types.ObjectNull(repeatConfigAttrTypes)
	}

	return &IncidentEscalationPathResourceModel{
		ID:           types.StringValue(ep.Id),
		Name:         types.StringValue(ep.Name),
		Path:         r.toPathModel(ctx, ep.Path, pathSchemaDepth, diags),
		WorkingHours: workingHours,
		RepeatConfig: repeatConfigObj,
		TeamIDs:      teamIDsSet,
	}
}

// targetsFromAPI builds a types.List of escalation path target objects from API
// targets.
func targetsFromAPI(ctx context.Context, targets []client.EscalationPathTargetV2, diags *diag.Diagnostics) types.List {
	targetModels := lo.Map(targets, func(target client.EscalationPathTargetV2, _ int) IncidentEscalationPathTarget {
		scheduleMode := types.StringNull()
		if target.ScheduleMode != nil {
			scheduleMode = types.StringValue(string(*target.ScheduleMode))
		}

		selectedRotaID := types.StringNull()
		if target.SelectedRotaId != nil && *target.SelectedRotaId != "" {
			selectedRotaID = types.StringValue(*target.SelectedRotaId)
		}

		return IncidentEscalationPathTarget{
			ID:             types.StringValue(target.Id),
			Type:           types.StringValue(string(target.Type)),
			Urgency:        types.StringValue(string(target.Urgency)),
			ScheduleMode:   scheduleMode,
			SelectedRotaID: selectedRotaID,
		}
	})

	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: targetAttrTypes()}, targetModels)
	diags.Append(d...)
	return list
}

func (r *IncidentEscalationPathResource) toPathModel(ctx context.Context, nodes []client.EscalationPathNodeV2, depth int, diags *diag.Diagnostics) types.List {
	elemType := types.ObjectType{AttrTypes: nodeAttrTypes(depth)}

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
				ThenPath: r.toPathModel(ctx, node.IfElse.ThenPath, depth-1, diags),
				ElsePath: r.toPathModel(ctx, node.IfElse.ElsePath, depth-1, diags),
			}
		}
		if node.Level != nil {
			elem.Level = &IncidentEscalationPathNodeLevel{
				Targets: targetsFromAPI(ctx, node.Level.Targets, diags),
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
				Targets: targetsFromAPI(ctx, node.NotifyChannel.Targets, diags),
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
		if node.Delay != nil {
			elem.Delay = &IncidentEscalationPathNodeDelay{}
			if value := node.Delay.DelaySeconds; value != nil {
				elem.Delay.DelaySeconds = types.Int64Value(*value)
			}
			if value := node.Delay.DelayIntervalCondition; value != nil {
				elem.Delay.DelayIntervalCondition = types.StringValue(string(*value))
			}
			if value := node.Delay.DelayWeekdayIntervalConfigId; value != nil && *value != "" {
				elem.Delay.DelayWeekdayIntervalConfigID = types.StringValue(*value)
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

	// Build the object values explicitly rather than reflecting the whole
	// struct via ListValueFrom. The IncidentEscalationPathNode struct always
	// carries an if_else field, but nodeAttrTypes omits if_else at depth 0
	// (matching the finite schema), so whole-struct reflection would fail at
	// the maximum nesting depth with a struct/object mismatch.
	objs := make([]attr.Value, 0, len(out))
	for _, node := range out {
		objs = append(objs, nodeToObject(ctx, node, depth, diags))
	}

	list, d := types.ListValue(elemType, objs)
	diags.Append(d...)
	return list
}

// nodeToObject converts a single escalation path node to a types.Object using
// the attribute types for the given recursion depth. It only sets the if_else
// attribute when depth > 0, mirroring nodeAttrTypes/getPathSchema, so it stays
// safe at the maximum nesting depth where if_else is not part of the schema.
func nodeToObject(ctx context.Context, node IncidentEscalationPathNode, depth int, diags *diag.Diagnostics) types.Object {
	attrTypes := nodeAttrTypes(depth)
	values := map[string]attr.Value{
		"id":   node.ID,
		"type": node.Type,
	}

	setObject := func(key string, isNil bool, from any) {
		objType, ok := attrTypes[key].(types.ObjectType)
		if !ok {
			return
		}
		if isNil {
			values[key] = types.ObjectNull(objType.AttrTypes)
			return
		}
		obj, d := types.ObjectValueFrom(ctx, objType.AttrTypes, from)
		diags.Append(d...)
		values[key] = obj
	}

	setObject("level", node.Level == nil, node.Level)
	setObject("notify_channel", node.NotifyChannel == nil, node.NotifyChannel)
	setObject("delay", node.Delay == nil, node.Delay)
	setObject("repeat", node.Repeat == nil, node.Repeat)
	if depth > 0 {
		setObject("if_else", node.IfElse == nil, node.IfElse)
	}

	obj, d := types.ObjectValue(attrTypes, values)
	diags.Append(d...)
	return obj
}

// targetsToPayload converts a types.List of target objects to client payloads.
func targetsToPayload(ctx context.Context, list types.List, diags *diag.Diagnostics) []client.EscalationPathTargetV2 {
	targets := decodeTargets(ctx, list, diags)
	return lo.Map(targets, func(target IncidentEscalationPathTarget, _ int) client.EscalationPathTargetV2 {
		targetPayload := client.EscalationPathTargetV2{
			Id:      target.ID.ValueString(),
			Type:    client.EscalationPathTargetV2Type(target.Type.ValueString()),
			Urgency: client.EscalationPathTargetV2Urgency(target.Urgency.ValueString()),
		}

		if target.ScheduleMode.ValueString() != "" {
			targetPayload.ScheduleMode = lo.ToPtr(client.EscalationPathTargetV2ScheduleMode(target.ScheduleMode.ValueString()))
		}

		if target.SelectedRotaID.ValueString() != "" {
			targetPayload.SelectedRotaId = lo.ToPtr(target.SelectedRotaID.ValueString())
		}

		return targetPayload
	})
}

func (r *IncidentEscalationPathResource) toPathPayload(ctx context.Context, pathList types.List, diags *diag.Diagnostics) []client.EscalationPathNodePayloadV2 {
	path := decodeNodes(ctx, pathList, diags)
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
				ThenPath:   r.toPathPayload(ctx, node.IfElse.ThenPath, diags),
				ElsePath:   r.toPathPayload(ctx, node.IfElse.ElsePath, diags),
			}
		}
		if !reflect.ValueOf(node.Level).IsZero() {
			var intervalCondition *client.EscalationPathNodeLevelV2TimeToAckIntervalCondition
			if value := node.Level.TimeToAckIntervalCondition.ValueStringPointer(); value != nil {
				intervalCondition = lo.ToPtr(client.EscalationPathNodeLevelV2TimeToAckIntervalCondition(*value))
			}

			elem.Level = &client.EscalationPathNodeLevelV2{
				Targets:                    targetsToPayload(ctx, node.Level.Targets, diags),
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
				Targets:                    targetsToPayload(ctx, node.NotifyChannel.Targets, diags),
				TimeToAckIntervalCondition: intervalCondition,
				TimeToAckSeconds: node.NotifyChannel.
					TimeToAckSeconds.ValueInt64Pointer(),
				TimeToAckWeekdayIntervalConfigId: node.NotifyChannel.
					TimeToAckWeekdayIntervalConfigID.ValueStringPointer(),
			}
		}
		if !reflect.ValueOf(node.Delay).IsZero() {
			var intervalCondition *client.EscalationPathNodeDelayV2DelayIntervalCondition
			if value := node.Delay.DelayIntervalCondition.ValueStringPointer(); value != nil {
				intervalCondition = lo.ToPtr(client.EscalationPathNodeDelayV2DelayIntervalCondition(*value))
			}

			elem.Delay = &client.EscalationPathNodeDelayV2{
				DelayIntervalCondition:       intervalCondition,
				DelaySeconds:                 node.Delay.DelaySeconds.ValueInt64Pointer(),
				DelayWeekdayIntervalConfigId: node.Delay.DelayWeekdayIntervalConfigID.ValueStringPointer(),
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
