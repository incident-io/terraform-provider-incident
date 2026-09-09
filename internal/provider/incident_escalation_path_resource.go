package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ resource.Resource                   = &escalationPathResource{}
	_ resource.ResourceWithImportState    = &escalationPathResource{}
	_ resource.ResourceWithMoveState      = &escalationPathResource{}
	_ resource.ResourceWithValidateConfig = &escalationPathResource{}
	_ resource.ResourceWithModifyPlan     = &escalationPathResource{}
)

func NewEscalationPathResource() resource.Resource {
	return &escalationPathResource{}
}

type escalationPathResource struct {
	resourceConfigurer
}

type escalationPathModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Start        types.String `tfsdk:"start"`
	Sequences    types.Map    `tfsdk:"sequences"`
	WorkingHours types.List   `tfsdk:"working_hours"`
	RepeatConfig types.Object `tfsdk:"repeat_config"`
	TeamIDs      types.Set    `tfsdk:"team_ids"`
}

type escalationPathSequence struct {
	Nodes types.List `tfsdk:"nodes"`
}

// escalationPathNode is one step of a sequence. Exactly one of the blocks is set, which
// validateSequenceNodes enforces, and which one it is takes the place of the type
// discriminator the API carries.
type escalationPathNode struct {
	ID             types.String                              `tfsdk:"id"`
	Level          *IncidentEscalationPathNodeLevel          `tfsdk:"level"`
	NotifyChannel  *IncidentEscalationPathNodeNotifyChannel  `tfsdk:"notify_channel"`
	Delay          *IncidentEscalationPathNodeDelay          `tfsdk:"delay"`
	EscalationPath *IncidentEscalationPathNodeEscalationPath `tfsdk:"escalation_path"`
	Branch         *escalationPathBranch                     `tfsdk:"branch"`
	Loop           *escalationPathLoop                       `tfsdk:"loop"`
}

// escalationPathBranch sends the escalation down one of two sequences. It replaces
// incident_escalation_path's if_else, whose then_path and else_path hold their nodes
// inline: here they're the keys of sequences declared alongside this one.
type escalationPathBranch struct {
	If   *escalationPathBranchIf `tfsdk:"if"`
	Then types.String            `tfsdk:"then"`
	Else types.String            `tfsdk:"else"`
}

// escalationPathLoop repeats from an earlier node. It replaces
// incident_escalation_path's repeat.
type escalationPathLoop struct {
	BackTo types.String `tfsdk:"back_to"`
	Times  types.Int64  `tfsdk:"times"`
}

// escalationPathNodeAttrTypes returns the attribute types for a node object. Unlike
// incident_escalation_path's, this doesn't recurse: a branch names its sequences rather
// than holding them, so the type is the same at every depth.
func escalationPathNodeAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":              types.StringType,
		"level":           types.ObjectType{AttrTypes: levelAttrTypes()},
		"notify_channel":  types.ObjectType{AttrTypes: notifyChannelAttrTypes()},
		"delay":           types.ObjectType{AttrTypes: delayAttrTypes()},
		"escalation_path": types.ObjectType{AttrTypes: escalationPathAttrTypes()},
		"branch": types.ObjectType{AttrTypes: map[string]attr.Type{
			"if":   types.ObjectType{AttrTypes: escalationPathBranchIfAttrTypes()},
			"then": types.StringType,
			"else": types.StringType,
		}},
		"loop": types.ObjectType{AttrTypes: map[string]attr.Type{
			"back_to": types.StringType,
			"times":   types.Int64Type,
		}},
	}
}

func (r *escalationPathResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path"
}

func (r *escalationPathResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Create and manage escalation paths, written as a flat map of named node sequences.

Each sequence runs its nodes in order and either ends with a ` + "`branch`" + `, which names the sequences to continue down, or falls off the end of the escalation path.

We'd generally recommend building escalation paths in our [web dashboard](https://app.incident.io/~/on-call/escalation-paths) and using the 'Export' flow to generate your Terraform.

## What changed in v7

Before v7, a branch held the nodes that followed it, as ` + "`if_else.then_path`" + ` and
` + "`if_else.else_path`" + `. Terraform schemas can't recurse indefinitely, so that shape stopped at
five levels of branching, and a path that branched more than that couldn't be written at all.

Sequences have no nesting, so there is no limit. A ` + "`branch`" + ` says which sequence to
continue down rather than holding its nodes, ` + "`loop`" + ` replaces ` + "`repeat`" + ` and names the node
to go back to, and ` + "`branch.if`" + ` replaces the raw engine ` + "`conditions`" + ` with one attribute
per thing an escalation can be tested on.

Moving a configuration across is a rewrite rather than a rename, though the escalation path
itself is untouched: import it by ID rather than recreating it, and everything escalating
through it carries on. The [v7 migration
guide](https://registry.terraform.io/providers/incident-io/incident/latest/docs/guides/migrating-to-v7)
has the mapping.`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "id"),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "name"),
				Required:            true,
			},

			"start": schema.StringAttribute{
				MarkdownDescription: "The key of the sequence this escalation path begins with.",
				Required:            true,
			},

			"sequences": schema.MapNestedAttribute{
				MarkdownDescription: "Named sequences of nodes, keyed by a name you choose. Each sequence either ends with a `branch` node or runs off the end of the escalation path. Branches reference other sequences by key.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"nodes": schema.ListNestedAttribute{
							MarkdownDescription: "The nodes in this sequence, in the order they run.",
							Required:            true,
							NestedObject:        escalationPathNodeSchema(),
						},
					},
				},
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

// escalationPathNodeSchema returns the schema for a single node. The level,
// notify_channel, delay and escalation_path blocks are shared with incident_escalation_path;
// branch and loop are this resource's own, and reference other nodes by name rather than
// nesting.
func escalationPathNodeSchema() schema.NestedAttributeObject {
	return schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An id for this node, unique within the escalation path, so a `loop` can name it. " +
					"Leave it unset unless something loops back here: we derive one from the node's position, which keeps it stable across applies.",
				Optional: true,
			},

			"level":           escalationPathLevelAttribute("first"),
			"notify_channel":  escalationPathNotifyChannelAttribute(),
			"delay":           escalationPathDelayAttribute(),
			"escalation_path": escalationPathEscalationPathAttribute(),

			"branch": schema.SingleNestedAttribute{
				MarkdownDescription: "Send the escalation down one of two sequences, depending on what `if` tests. A branch must be the last node in its sequence.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"if": escalationPathBranchIfAttribute(),
					"then": schema.StringAttribute{
						MarkdownDescription: "The key of the sequence to continue down when the condition is met.",
						Required:            true,
					},
					"else": schema.StringAttribute{
						MarkdownDescription: "The key of the sequence to continue down when the condition is not met. Leave unset to end the escalation path instead.",
						Optional:            true,
					},
				},
			},

			"loop": schema.SingleNestedAttribute{
				MarkdownDescription: "Go back to an earlier node and run from there again.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"back_to": schema.StringAttribute{
						MarkdownDescription: "The `id` of the node to repeat from.",
						Required:            true,
					},
					"times": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2", "repeat_times"),
						Required:            true,
					},
				},
			},
		},
	}
}

func (r *escalationPathResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data *escalationPathModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	validateSequences(ctx, data, &resp.Diagnostics)
	validateSequenceConditions(ctx, data, &resp.Diagnostics)
	validateEscalationPathTargets(ctx, data, &resp.Diagnostics)
}

// validateEscalationPathTargets applies the same target checks as
// incident_escalation_path, which the flat layout doesn't change.
func validateEscalationPathTargets(ctx context.Context, data *escalationPathModel, diags *diag.Diagnostics) {
	for _, nodes := range decodeSequences(ctx, data.Sequences, diags) {
		for _, node := range nodes {
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
		}
	}
}

// ModifyPlan asks the API whether it would accept this path, so a config that can't apply
// fails the plan instead.
func (r *escalationPathResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A destroy plans no path, and an unconfigured provider has no client.
	if r.client == nil || req.Plan.Raw.IsNull() {
		return
	}

	// The framework runs this for every resource in the plan, changed or not. A rejection
	// on a path planning no change isn't something an apply could fix.
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	// A target pointing at a schedule this same apply creates is unknown until it exists,
	// and checking around the gaps reports errors the apply won't hit.
	if !escalationPathPlanSettled(req.Plan.Raw) {
		return
	}

	// Create and Update read the same plan and report a failure to decode it, so there's
	// nothing to add by reporting it here too.
	var data escalationPathModel
	if req.Plan.Get(ctx, &data).HasError() {
		return
	}

	payload := r.toPayload(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, escalationPathValidateTimeout)
	defer cancel()

	result, err := r.client.EscalationsV2ValidatePathWithResponse(ctx, client.EscalationsV2ValidatePathJSONRequestBody{
		Path:         payload.Path,
		TeamIds:      payload.TeamIds,
		WorkingHours: payload.WorkingHours,
		RepeatConfig: payload.RepeatConfig,
	})
	if err == nil {
		addEscalationPathValidateWarnings(result, &resp.Diagnostics)
		return
	}

	// 422 is the API rejecting this path, which is the whole point.
	var httpErr client.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity {
		resp.Diagnostics.AddError("Invalid escalation path", httpErr.Error())
		return
	}

	// Anything else means the check didn't run, not that the config is bad, so failing
	// here would break plans that would have applied fine.
	resp.Diagnostics.AddWarning(
		"Could not validate the escalation path",
		fmt.Sprintf("The escalation path was not checked, and may still be rejected when you apply: %s", err),
	)
}

// escalationPathPlanSettled reports whether every value the check would send is
// known. Gating on the whole plan instead would skip every create, which plans the path's
// own id unknown, and every target that leaves schedule_mode to the API.
func escalationPathPlanSettled(plan tftypes.Value) bool {
	settled := true
	//nolint:errcheck // the callback never returns an error, so neither does the walk.
	_ = tftypes.Walk(plan, func(attrPath *tftypes.AttributePath, value tftypes.Value) (bool, error) {
		if value.IsKnown() {
			return true, nil
		}
		if !escalationPathComputedPath(attrPath) {
			settled = false
		}
		// An unknown has no children worth walking either way.
		return false, nil
	})
	return settled
}

// escalationPathComputedPath reports whether an unknown at this path is the API
// filling a value in, rather than another resource we're waiting on.
func escalationPathComputedPath(attrPath *tftypes.AttributePath) bool {
	steps := attrPath.Steps()
	if len(steps) == 0 {
		return false
	}

	name, ok := steps[len(steps)-1].(tftypes.AttributeName)
	if !ok {
		return false
	}

	// The escalation path's own id, unknown on every create. Nested ids don't qualify: a
	// target's is how it points at a schedule another resource creates, which is exactly
	// the case we skip for.
	if len(steps) == 1 {
		return name == "id"
	}

	// A target's schedule_mode, which the API derives from the target when it isn't set.
	return name == "schedule_mode"
}

func (r *escalationPathResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *escalationPathModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.toPayload(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationsV2CreatePathWithResponse(ctx, client.EscalationsV2CreatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         payload.Path,
		WorkingHours: payload.WorkingHours,
		TeamIds:      payload.TeamIds,
		RepeatConfig: payload.RepeatConfig,
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
	model := r.buildModel(ctx, result.JSON201.EscalationPath, data, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *escalationPathResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *escalationPathModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationsV2ShowPathWithResponse(ctx, data.ID.ValueString())
	if isNotFound(err) {
		tflog.Warn(ctx, fmt.Sprintf("Escalation path with ID %s not found: removing from state.", data.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
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

	model := r.buildModel(ctx, result.JSON200.EscalationPath, data, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *escalationPathResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *escalationPathModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.toPayload(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationsV2UpdatePathWithResponse(ctx, data.ID.ValueString(), client.EscalationsV2UpdatePathJSONRequestBody{
		Name:         data.Name.ValueString(),
		Path:         payload.Path,
		WorkingHours: payload.WorkingHours,
		TeamIds:      payload.TeamIds,
		RepeatConfig: payload.RepeatConfig,
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

	model := r.buildModel(ctx, result.JSON200.EscalationPath, data, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *escalationPathResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *escalationPathModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.EscalationsV2DestroyPathWithResponse(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete escalation path, got error: %s", err))
		return
	}
}

func (r *escalationPathResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath, r.terraformVersion, r.markImportedAsManaged)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// MoveState takes the state of an `incident_escalation_path_beta`, which is the
// name this resource went by before v7. The schema is the same one, so a `moved`
// block is all it takes:
//
//	moved {
//	  from = incident_escalation_path_beta.urgent_support
//	  to   = incident_escalation_path.urgent_support
//	}
func (r *escalationPathResource) MoveState(ctx context.Context) []resource.StateMover {
	return []resource.StateMover{
		renameStateMover("incident_escalation_path_beta", declaredResourceSchema(ctx, r)),
	}
}

// escalationPathPayload is the part of a create, update or validate body that this
// resource builds the same way for all three.
type escalationPathPayload struct {
	Path         []client.EscalationPathNodePayloadV2
	WorkingHours *[]client.WeekdayIntervalConfigV2
	RepeatConfig *client.EscalationPathRepeatConfigV2
	TeamIds      *[]string
}

func (r *escalationPathResource) toPayload(ctx context.Context, data *escalationPathModel, diags *diag.Diagnostics) escalationPathPayload {
	sequences := decodeSequences(ctx, data.Sequences, diags)
	if diags.HasError() {
		return escalationPathPayload{}
	}

	return escalationPathPayload{
		Path:         unflattenSequences(ctx, data.Start.ValueString(), sequences, diags),
		WorkingHours: escalationPathWorkingHoursToPayload(ctx, data.WorkingHours, diags),
		RepeatConfig: escalationPathRepeatConfigToPayload(ctx, data.RepeatConfig, diags),
		TeamIds:      escalationPathTeamIDsToPayload(ctx, data.TeamIDs, diags),
	}
}

// buildModel converts what the API returned into state. The API stores no sequence names,
// so prior is where they come from: the plan on a create or update, and the state on a
// read, which on an import holds nothing but the id and so names nothing.
func (r *escalationPathResource) buildModel(ctx context.Context, ep client.EscalationPathV2, prior *escalationPathModel, diags *diag.Diagnostics) *escalationPathModel {
	start, sequences := flattenSequences(ctx, ep.Path, escalationPathPriorNamesFrom(ctx, prior), diags)

	return &escalationPathModel{
		ID:           types.StringValue(ep.Id),
		Name:         types.StringValue(ep.Name),
		Start:        types.StringValue(start),
		Sequences:    escalationPathSequencesToMap(ctx, sequences, diags),
		WorkingHours: escalationPathWorkingHoursFromAPI(ctx, ep.WorkingHours, diags),
		RepeatConfig: escalationPathRepeatConfigFromAPI(ep.RepeatConfig),
		TeamIDs:      escalationPathTeamIDsFromAPI(ep.TeamIds),
	}
}

// escalationPathValidateTimeout caps how long a plan waits on the API's own
// opinion of the path, so an unreachable API slows a plan down rather than
// hanging it.
var escalationPathValidateTimeout = 10 * time.Second

func addEscalationPathValidateWarnings(result *client.EscalationsV2ValidatePathResponse, diags *diag.Diagnostics) {
	if result == nil || result.JSON200 == nil {
		return
	}

	for _, warning := range result.JSON200.Warnings {
		diags.AddWarning(warning.Summary, fmt.Sprintf("%s (at %s)", warning.Detail, warning.Path))
	}
}
