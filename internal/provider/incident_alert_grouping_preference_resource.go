package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ resource.Resource                   = &IncidentAlertGroupingPreferenceResource{}
	_ resource.ResourceWithConfigure      = &IncidentAlertGroupingPreferenceResource{}
	_ resource.ResourceWithImportState    = &IncidentAlertGroupingPreferenceResource{}
	_ resource.ResourceWithValidateConfig = &IncidentAlertGroupingPreferenceResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentAlertGroupingPreferenceResource{}
	_ resource.ResourceWithMoveState      = &IncidentAlertGroupingPreferenceResource{}
)

type IncidentAlertGroupingPreferenceResource struct {
	resourceConfigurer
}

func NewIncidentAlertGroupingPreferenceResource() resource.Resource {
	return &IncidentAlertGroupingPreferenceResource{}
}

func (r *IncidentAlertGroupingPreferenceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_grouping_preference"
}

func (r *IncidentAlertGroupingPreferenceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `How alerts are grouped: the organisation default, or one team's preference. Alert rules group
an alert by the first branch whose conditions hold, else by the default. Alert routes only use a
team's default.

The organisation has one default (` + "`kind = \"default\"`" + `), and a plan that creates it fails once it
exists: import it by ID instead. It can't be deleted, so destroying it turns its grouping off and
hands it back to the dashboard, where its earlier settings can be restored from its history. To stop
managing it and keep its grouping, use a ` + "`removed`" + ` block with ` + "`destroy = false`" + `. Each team has at most one preference (` + "`kind = \"team\"`" + `). Create one
for a team that has none, or import it if it exists. Destroying a team's preference deletes it. A
preference can't change kind or move to another team, so a plan that changes ` + "`kind`" + ` or
` + "`team_id`" + ` fails.

A team's default can be written whatever your organisation routes alerts with. Its branches, and
anything about the organisation default, can only be changed once your organisation has moved to
alert rules. Until then the API refuses the change and this resource fails at apply with that message.

This replaces ` + "`incident_team_grouping_preference`" + `. To move one over without recreating it, on
Terraform 1.8 or later, add ` + "`kind = \"team\"`" + ` and a ` + "`moved`" + ` block:

` + "```terraform" + `
moved {
  from = incident_team_grouping_preference.payments
  to   = incident_alert_grouping_preference.payments
}
` + "```" + `

On earlier versions, run ` + "`terraform state rm`" + ` on the old address and import the preference into
this resource by its ID. The old resource left the team's branches and expressions as the dashboard
wrote them, but this one manages them: declare any the team has, or the next apply removes them.`,
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardAttribute(),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertGroupingPreferenceV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"kind": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.EnumValuesDescription("AlertGroupingPreferenceV3", "kind") + " It can't change.",
				Validators: []validator.String{
					stringvalidator.OneOf(apischema.EnumValues("AlertGroupingPreferenceV3", "kind")...),
				},
			},
			"team_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: apischema.Docstring("AlertGroupingPreferenceV3", "team_id") +
					". Required when `kind` is `team`, and not allowed otherwise. It can't change.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertGroupingPreferenceV3", "version"),
			},
			"default": models.GroupingDefaultAttribute("The grouping settings for alerts no branch matches."),
		},
		Blocks: models.GroupingPreferenceBlocks(),
	}
}

func (r *IncidentAlertGroupingPreferenceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.AlertGroupingPreferenceResourceModel

	// A value another resource computes can't decode into the model, so skip until it is known.
	if req.Config.Get(ctx, &data).HasError() {
		return
	}

	models.ValidateGroupingPreference(data, &resp.Diagnostics)
}

// ModifyPlan keeps each branch's ID with its branch, and refuses at plan what the API would
// refuse at apply: a new kind or team, or a preference that already exists.
func (r *IncidentAlertGroupingPreferenceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	// Read the kind and team on their own, so an unknown elsewhere in the plan can't skip
	// these checks.
	var kind, teamID types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("kind"), &kind)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("team_id"), &teamID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !req.State.Raw.IsNull() {
		var priorKind, priorTeamID types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("kind"), &priorKind)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("team_id"), &priorTeamID)...)
		if resp.Diagnostics.HasError() {
			return
		}

		switch {
		case !kind.IsUnknown() && !kind.Equal(priorKind):
			resp.Diagnostics.AddAttributeError(path.Root("kind"), "Kind cannot change",
				"A grouping preference can't change between the organisation default and a team's preference. "+
					"Remove this resource and add a new one.")
		case !teamID.IsUnknown() && !teamID.Equal(priorTeamID):
			resp.Diagnostics.AddAttributeError(path.Root("team_id"), "Team cannot change",
				"A team's grouping preference can't move to another team. Remove this resource and add a new one for that team.")
		}

		r.carryBranchIDs(ctx, req, resp)
		return
	}

	if r.client == nil || kind.IsUnknown() || teamID.IsUnknown() {
		return
	}
	// A replace plans its create half here too, so skipping this on an error would let the
	// destroy hand the preference back before the create fails.
	id, err := r.findExistingID(ctx, kind.ValueString(), teamID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to check for an existing grouping preference, got error: %s", err))
		return
	}
	// ModifyPlan isn't told the resource's address, but Terraform prints it above this message.
	if id != "" {
		what := "a default grouping preference"
		if kind.ValueString() == models.GroupingKindTeam {
			what = "a grouping preference for this team"
		}
		resp.Diagnostics.AddAttributeError(path.Root("kind"), "Grouping preference already exists",
			fmt.Sprintf("This organisation already has %[1]s, with ID %[2]s, so it cannot be created. "+
				"To manage it, import it into this resource with an import block:"+
				"\n\n  import {\n    to = <this resource's address, shown above>\n    id = %[2]q\n  }"+
				"\n\nIf an incident_team_grouping_preference manages it, move that resource here with a moved block instead.",
				what, id))
	}
}

// carryBranchIDs puts each state branch's ID back on the planned branch it becomes. The API
// keys alert groups on a branch's ID, so it has to follow the branch wherever it moved to
// rather than stay at its index.
func (r *IncidentAlertGroupingPreferenceResource) carryBranchIDs(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var plan, state []models.GroupingBranchModel
	if req.Plan.GetAttribute(ctx, path.Root("branch"), &plan).HasError() {
		// A branch holds something still unknown in a way the model can't, such as a whole
		// list. There is nothing to carry over yet.
		return
	}
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("branch"), &state)...)
	if resp.Diagnostics.HasError() || len(plan) == 0 || len(state) == 0 {
		return
	}

	models.CarryBranchIDs(plan, state)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("branch"), plan)...)
}

func (r *IncidentAlertGroupingPreferenceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.AlertGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, err := plan.ToCreatePayload(plan.Kind.ValueString(), plan.TeamID.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError("Invalid grouping preference", err.Error())
		return
	}
	result, err := r.client.AlertGroupingPreferencesV3CreateWithResponse(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create grouping preference, got error: %s", err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to create grouping preference: unexpected response from API (status %s)", result.Status()))
		return
	}
	state := models.AlertGroupingPreferenceResourceModel{}.FromAPI(result.JSON201.AlertGroupingPreference, &plan)

	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, state.ID.ValueString(), &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertGroupingPreference, r.terraformVersion)
	}
	tflog.Trace(ctx, fmt.Sprintf("created a grouping preference with id=%s", state.ID.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAlertGroupingPreferenceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.AlertGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	newState, found := r.show(ctx, state.ID.ValueString(), &state, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		tflog.Warn(ctx, fmt.Sprintf("Grouping preference with ID %s not found: removing from state.", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *IncidentAlertGroupingPreferenceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.AlertGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// State names the object this updates, whatever the plan says.
	var prior models.AlertGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := prior.ID.ValueString()

	payload, err := plan.ToUpdatePayload()
	if err != nil {
		resp.Diagnostics.AddError("Invalid grouping preference", err.Error())
		return
	}
	result, err := r.client.AlertGroupingPreferencesV3UpdateWithResponse(ctx, id, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update grouping preference, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to update grouping preference: unexpected response from API (status %s)", result.Status()))
		return
	}
	state := models.AlertGroupingPreferenceResourceModel{}.FromAPI(result.JSON200.AlertGroupingPreference, &plan)

	managedType := client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertGroupingPreference
	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, id, &resp.Diagnostics, managedType, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, id, &resp.Diagnostics, managedType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete removes a team's preference. The organisation default always exists once saved, so
// the API turns its grouping off and hands it back to the dashboard instead.
func (r *IncidentAlertGroupingPreferenceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.AlertGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A 404 is a preference already gone.
	_, err := r.client.AlertGroupingPreferencesV3DeleteWithResponse(ctx, state.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete grouping preference, got error: %s", err))
	}
}

// ImportState takes the preference's ID, for a team's preference and the organisation
// default alike.
func (r *IncidentAlertGroupingPreferenceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	state, found := r.show(ctx, req.ID, nil, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError("Grouping Preference Not Found",
			fmt.Sprintf("No alert grouping preference with ID %q exists.", req.ID))
		return
	}

	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertGroupingPreference, r.terraformVersion, r.markImportedAsManaged)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// MoveState takes the state of an incident_team_grouping_preference, which this resource
// replaces, so moving one over is a `moved` block rather than a destroy and create:
//
//	moved {
//	  from = incident_team_grouping_preference.payments
//	  to   = incident_alert_grouping_preference.payments
//	}
//
// The attributes both resources hold are carried across, and the old one only ever held a
// team's preference. Terraform refreshes the moved resource before it plans, which reads the
// branches and expressions the old resource never held.
func (r *IncidentAlertGroupingPreferenceResource) MoveState(ctx context.Context) []resource.StateMover {
	source := NewIncidentTeamGroupingPreferenceResource()
	mover := migrateStateMover(resourceTypeName(ctx, source), declaredResourceSchema(ctx, source), declaredResourceSchema(ctx, r))

	carry := mover.StateMover
	mover.StateMover = func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
		carry(ctx, req, resp)
		if resp.Diagnostics.HasError() || resp.TargetState.Raw.IsNull() {
			return
		}
		resp.Diagnostics.Append(resp.TargetState.SetAttribute(ctx, path.Root("kind"), models.GroupingKindTeam)...)
	}

	return []resource.StateMover{mover}
}

// show reads one preference, reporting a 404 as not found so each caller decides what a
// missing preference means. prior is passed on to FromAPI.
func (r *IncidentAlertGroupingPreferenceResource) show(
	ctx context.Context, id string, prior *models.AlertGroupingPreferenceResourceModel, addError func(summary, detail string),
) (*models.AlertGroupingPreferenceResourceModel, bool) {
	result, err := r.client.AlertGroupingPreferencesV3ShowWithResponse(ctx, id)
	switch {
	case err == nil && result.JSON200 != nil:
		preference := models.AlertGroupingPreferenceResourceModel{}.FromAPI(result.JSON200.AlertGroupingPreference, prior)
		return &preference, true
	case isNotFound(err):
	case err != nil:
		addError("Client Error", fmt.Sprintf("Unable to read grouping preference, got error: %s", err))
	default:
		addError("Client Error", fmt.Sprintf("Unable to read grouping preference: unexpected response from API (status %s)", result.Status()))
	}

	return nil, false
}

// findExistingID finds the preference a create would collide with: the organisation default,
// or the team's preference. It returns an empty ID when there is none.
func (r *IncidentAlertGroupingPreferenceResource) findExistingID(ctx context.Context, kind string, teamID types.String) (string, error) {
	result, err := r.client.AlertGroupingPreferencesV3ListWithResponse(ctx, &client.AlertGroupingPreferencesV3ListParams{
		Kind:   (*client.AlertGroupingPreferencesV3ListParamsKind)(&kind),
		TeamId: teamID.ValueStringPointer(),
	})
	if err != nil {
		return "", err
	}
	if result.JSON200 == nil {
		return "", fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if len(result.JSON200.AlertGroupingPreferences) == 0 {
		return "", nil
	}

	return result.JSON200.AlertGroupingPreferences[0].Id, nil
}
