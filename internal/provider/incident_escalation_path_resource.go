package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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
	_ resource.ResourceWithModifyPlan     = &IncidentEscalationPathResource{}
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
	ID             types.String                              `tfsdk:"id"`
	Type           types.String                              `tfsdk:"type"`
	Delay          *IncidentEscalationPathNodeDelay          `tfsdk:"delay"`
	EscalationPath *IncidentEscalationPathNodeEscalationPath `tfsdk:"escalation_path"`
	IfElse         *IncidentEscalationPathNodeIfElse         `tfsdk:"if_else"`
	Level          *IncidentEscalationPathNodeLevel          `tfsdk:"level"`
	Repeat         *IncidentEscalationPathNodeRepeat         `tfsdk:"repeat"`
	NotifyChannel  *IncidentEscalationPathNodeNotifyChannel  `tfsdk:"notify_channel"`
}

type IncidentEscalationPathNodeIfElse struct {
	Conditions models.IncidentEngineConditions `tfsdk:"conditions"`
	ElsePath   types.List                      `tfsdk:"else_path"`
	ThenPath   types.List                      `tfsdk:"then_path"`
}

type IncidentEscalationPathNodeLevel struct {
	Targets                          types.List                          `tfsdk:"targets"`
	RoundRobinConfig                 *IncidentEscalationRoundRobinConfig `tfsdk:"round_robin_config"`
	RetryConfig                      *IncidentEscalationRetryConfig      `tfsdk:"retry_config"`
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

type IncidentEscalationPathNodeEscalationPath struct {
	EscalationPathID types.String `tfsdk:"escalation_path_id"`
}

type IncidentEscalationPathNodeRepeat struct {
	RepeatTimes types.Int64  `tfsdk:"repeat_times"`
	ToNode      types.String `tfsdk:"to_node"`
}

type IncidentEscalationRoundRobinConfig struct {
	Enabled            types.Bool  `tfsdk:"enabled"`
	RotateAfterSeconds types.Int64 `tfsdk:"rotate_after_seconds"`
}

type IncidentEscalationRetryConfig struct {
	Attempts        types.Int64 `tfsdk:"attempts"`
	IntervalSeconds types.Int64 `tfsdk:"interval_seconds"`
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
		"id":    types.StringType,
		"type":  types.StringType,
		"level": types.ObjectType{AttrTypes: levelAttrTypes()},
		"repeat": types.ObjectType{AttrTypes: map[string]attr.Type{
			"repeat_times": types.Int64Type,
			"to_node":      types.StringType,
		}},
		"notify_channel":  types.ObjectType{AttrTypes: notifyChannelAttrTypes()},
		"delay":           types.ObjectType{AttrTypes: delayAttrTypes()},
		"escalation_path": types.ObjectType{AttrTypes: escalationPathAttrTypes()},
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
					// Not UseStateForUnknown: nodes are a nested collection, so a newly
					// added node has no state value and that modifier would plan null.
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("EscalationPathNodeV2", "type"),
				Required:            true,
			},
			"level": escalationPathLevelAttribute("all"),
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
			"notify_channel":  escalationPathNotifyChannelAttribute(),
			"delay":           escalationPathDelayAttribute(),
			"escalation_path": escalationPathEscalationPathAttribute(),
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

// escalationPathValidateTimeout bounds the plan-time validate call. Long enough that a
// slow-but-working API still answers, short enough that an unhealthy one costs a plan
// seconds rather than minutes. A var so tests don't have to wait it out.
var escalationPathValidateTimeout = 10 * time.Second

// escalationPathLeafIsSafeUnknown reports whether an unknown value at this path is one the
// validate payload doesn't need settled: a path node's own "id" (Computed+Optional; we
// generate one in toPathPayload when it's empty) and a target's "schedule_mode"
// (Optional+Computed with no static default; the API fills it in regardless). A target's
// own "id" is Required, user-supplied data that may reference another resource this same
// apply creates, so that one is NOT safe - an unknown there means we skip the check rather
// than validate around the gap.
func escalationPathLeafIsSafeUnknown(steps *tftypes.AttributePath) bool {
	all := steps.Steps()
	if len(all) == 0 {
		return false
	}

	name, ok := all[len(all)-1].(tftypes.AttributeName)
	if !ok {
		return false
	}

	switch string(name) {
	case "schedule_mode":
		return true
	case "id":
		if len(all) == 1 {
			// The resource's own id: Computed only, always unknown on create, and not part
			// of the validate payload at all.
			return true
		}
		if len(all) < 3 {
			return false
		}
		if _, ok := all[len(all)-2].(tftypes.ElementKeyInt); !ok {
			return false
		}
		parent, ok := all[len(all)-3].(tftypes.AttributeName)
		return ok && (string(parent) == "path" || string(parent) == "then_path" || string(parent) == "else_path")
	default:
		return false
	}
}

// ModifyPlan asks the API whether the planned escalation path would be accepted, so a
// config the API would reject - such as a target missing a required selected_rota_id -
// surfaces in the plan instead of part way through an apply.
//
// It can't live in ValidateConfig, which `terraform validate` also calls: that runs the
// provider without configuring it, so there's no client to ask with.
func (r *IncidentEscalationPathResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A destroy plans no escalation path, and an unconfigured provider has no client.
	if r.client == nil || req.Plan.Raw.IsNull() {
		return
	}

	// A path the plan doesn't change isn't going to be applied, so there's nothing to warn
	// about - and checking every escalation path on every plan is a request each.
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	// A target or condition pointing at something this same apply creates is unknown until
	// it exists. Validating around the gaps would report errors the apply won't hit, so
	// leave those configs alone.
	if !escalationPathValidateSettled(req.Plan.Raw) {
		return
	}

	var data *IncidentEscalationPathResourceModel
	if req.Plan.Get(ctx, &data).HasError() || data == nil {
		return
	}

	// A decode failure here means a value the model's concrete types can't represent, not
	// that the config is bad - give up on validating rather than failing the plan over a
	// config the API may well accept.
	var localDiags diag.Diagnostics
	workingHours, teamIDs, repeatConfig, pathPayload := r.toEscalationPathPayload(ctx, data, &localDiags)
	if localDiags.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, escalationPathValidateTimeout)
	defer cancel()

	result, err := r.client.EscalationsV2ValidatePathWithResponse(ctx, client.EscalationsV2ValidatePathJSONRequestBody{
		Path:         pathPayload,
		WorkingHours: workingHours,
		TeamIds:      teamIDs,
		RepeatConfig: repeatConfig,
	})
	if err == nil {
		addEscalationPathValidateWarnings(result, &resp.Diagnostics)
		return
	}

	// 422 is the API rejecting this config, which is the whole point.
	var httpErr client.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity {
		resp.Diagnostics.AddError("Invalid escalation path", httpErr.Error())
		return
	}

	// Anything else means the check didn't run, not that the config is bad: the endpoint
	// isn't deployed yet, the API is down, the request timed out. Warn, because failing here
	// would break plans over a config that may be perfectly good.
	resp.Diagnostics.AddWarning(
		"Could not validate the escalation path",
		fmt.Sprintf("The escalation path was not checked, and may still be rejected when you apply: %s", err),
	)
}

// addEscalationPathValidateWarnings reports what the API found suspect without rejecting,
// such as an if_else branch with no nodes in it. The API's path is a dotted/indexed
// location into the payload (e.g. "path.0.if_else.then_path") rather than a schema
// attribute path, so these surface as plan-level warnings rather than attribute ones.
func addEscalationPathValidateWarnings(result *client.EscalationsV2ValidatePathResponse, diags *diag.Diagnostics) {
	if result == nil || result.JSON200 == nil {
		return
	}

	for _, warning := range result.JSON200.Warnings {
		diags.AddWarning(warning.Summary, fmt.Sprintf("%s (at %s)", warning.Detail, warning.Path))
	}
}

// escalationPathValidateSettled reports whether every value the validate payload could
// carry is known, other than the computed leaves the API fills in regardless.
//
// It walks the whole plan value in one pass rather than per-attribute: escalationPathLeafIsSafeUnknown
// disambiguates a node's own "id" from a target's "id" by looking at the grandparent
// attribute name ("path"/"then_path"/"else_path" vs "targets"), which only works when
// every step's AttributePath is rooted at the plan object - walking a sub-value directly
// would silently drop that leading step and misclassify a brand new node's id as unsafe.
func escalationPathValidateSettled(plan tftypes.Value) bool {
	settled := true
	err := tftypes.Walk(plan, func(steps *tftypes.AttributePath, v tftypes.Value) (bool, error) {
		if v.IsKnown() {
			return true, nil
		}
		if escalationPathLeafIsSafeUnknown(steps) {
			return false, nil
		}
		settled = false
		return false, nil
	})
	if err != nil {
		return false
	}

	return settled
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
	var escalationPath IncidentEscalationPathNodeEscalationPath
	if decodeObject("escalation_path", &escalationPath) {
		node.EscalationPath = &escalationPath
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
		validateEscalationPathNodeEscalationPath(node, diags)
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

// validateEscalationPathNodeEscalationPath checks that a node's type and its
// escalation_path block agree.
//
// Nothing else does: the type attribute has no enum validator, and a node's blocks are all
// optional, so a reassignment node missing its block reaches the API as "type:
// escalation_path" with no config and comes back as an opaque payload error. Catching it
// here names the problem while the author is still looking at the plan.
func validateEscalationPathNodeEscalationPath(node IncidentEscalationPathNode, diags *diag.Diagnostics) {
	nodeType := node.Type
	if nodeType.IsUnknown() || nodeType.IsNull() {
		return
	}

	isReassignment := nodeType.ValueString() == string(client.EscalationPathNodeV2TypeEscalationPath)
	switch {
	case isReassignment && node.EscalationPath == nil:
		diags.Append(diag.NewErrorDiagnostic(
			"Missing escalation_path block",
			"A node with type \"escalation_path\" must set an escalation_path block naming the path to reassign to.",
		))
	case !isReassignment && node.EscalationPath != nil:
		diags.Append(diag.NewErrorDiagnostic(
			"Unexpected escalation_path block",
			fmt.Sprintf("A node with type %q must not set an escalation_path block; it is only valid on a node with type \"escalation_path\".", nodeType.ValueString()),
		))
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

// toEscalationPathPayload builds the working hours, team IDs, repeat config, and node
// path shared by the create, update, and validate requests.
func (r *IncidentEscalationPathResource) toEscalationPathPayload(ctx context.Context, data *IncidentEscalationPathResourceModel, diags *diag.Diagnostics) (
	workingHours *[]client.WeekdayIntervalConfigV2,
	teamIDs *[]string,
	repeatConfig *client.EscalationPathRepeatConfigV2,
	pathPayload []client.EscalationPathNodePayloadV2,
) {
	workingHours = escalationPathWorkingHoursToPayload(ctx, data.WorkingHours, diags)
	teamIDs = escalationPathTeamIDsToPayload(ctx, data.TeamIDs, diags)
	repeatConfig = escalationPathRepeatConfigToPayload(ctx, data.RepeatConfig, diags)
	if diags.HasError() {
		return
	}

	pathPayload = r.toPathPayload(ctx, data.Path, diags)

	return
}

func (r *IncidentEscalationPathResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	workingHours, teamIDs, repeatConfig, pathPayload := r.toEscalationPathPayload(ctx, data, &resp.Diagnostics)
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

	claimResource(ctx, r.client, result.JSON201.EscalationPath.Id, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath, r.terraformVersion)

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

	workingHours, teamIDs, repeatConfig, pathPayload := r.toEscalationPathPayload(ctx, data, &resp.Diagnostics)
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

	claimResource(ctx, r.client, result.JSON200.EscalationPath.Id, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath, r.terraformVersion)

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
	claimResource(ctx, r.client, req.ID, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath, r.terraformVersion)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentEscalationPathResource) buildModel(ctx context.Context, ep client.EscalationPathV2, diags *diag.Diagnostics) *IncidentEscalationPathResourceModel {
	return &IncidentEscalationPathResourceModel{
		ID:           types.StringValue(ep.Id),
		Name:         types.StringValue(ep.Name),
		Path:         r.toPathModel(ctx, ep.Path, pathSchemaDepth, diags),
		WorkingHours: escalationPathWorkingHoursFromAPI(ctx, ep.WorkingHours, diags),
		RepeatConfig: escalationPathRepeatConfigFromAPI(ep.RepeatConfig),
		TeamIDs:      escalationPathTeamIDsFromAPI(ep.TeamIds),
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
		elem.Level = levelFromAPI(ctx, node.Level, diags)
		elem.NotifyChannel = notifyChannelFromAPI(ctx, node.NotifyChannel, diags)
		elem.Delay = delayFromAPI(node.Delay)
		elem.EscalationPath = escalationPathFromAPI(node.EscalationPath)
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
	setObject("escalation_path", node.EscalationPath == nil, node.EscalationPath)
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
		elem.Level = levelToPayload(ctx, node.Level, diags)
		elem.NotifyChannel = notifyChannelToPayload(ctx, node.NotifyChannel, diags)
		elem.Delay = delayToPayload(node.Delay)
		elem.EscalationPath = escalationPathToPayload(node.EscalationPath)
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
