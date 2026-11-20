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
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ resource.Resource                   = &IncidentGroupingPreferenceResource{}
	_ resource.ResourceWithConfigure      = &IncidentGroupingPreferenceResource{}
	_ resource.ResourceWithImportState    = &IncidentGroupingPreferenceResource{}
	_ resource.ResourceWithValidateConfig = &IncidentGroupingPreferenceResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentGroupingPreferenceResource{}
	_ resource.ResourceWithMoveState      = &IncidentGroupingPreferenceResource{}
)

type IncidentGroupingPreferenceResource struct {
	resourceConfigurer
}

func NewIncidentGroupingPreferenceResource() resource.Resource {
	return &IncidentGroupingPreferenceResource{}
}

func (r *IncidentGroupingPreferenceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_grouping_preference"
}

func (r *IncidentGroupingPreferenceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `How alerts are grouped: the organisation default, or one team's preference. Alert rules group
an alert by the first branch whose conditions hold, else by the default. Alert routes only use a
team's default.

The organisation has one default (` + "`scope = \"organisation\"`" + `), and a plan that creates it fails once it
exists: import it by ID instead. Destroying it turns its grouping off and hands it back to the
dashboard. Each team has at most one preference (` + "`scope = \"team\"`" + `), created and deleted like any other
resource. A preference can't change scope or move to another team, so a plan that changes ` + "`scope`" + ` or
` + "`team_id`" + ` fails.

A team's default can be written whatever your organisation routes alerts with. Its branches, and
anything about the organisation default, can only be changed once your organisation has moved to
alert rules. Until then the API refuses the change and this resource fails at apply with that message.

This replaces ` + "`incident_team_grouping_preference`" + `. To move one over without recreating it, on
Terraform 1.8 or later, add ` + "`scope = \"team\"`" + ` and a ` + "`moved`" + ` block:

` + "```terraform" + `
moved {
  from = incident_team_grouping_preference.payments
  to   = incident_grouping_preference.payments
}
` + "```" + `

On earlier versions, run ` + "`terraform state rm`" + ` on the old address and import the preference into
this resource by its ID. The old resource left the team's branches and expressions as the dashboard
wrote them, but this one manages them: declare any the team has, or the next apply removes them.`,
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardAttribute(),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("TeamGroupingPreferenceV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"scope": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Whether this is the organisation default or a team's preference. " +
					"Possible values are: `organisation`, `team`. It can't change.",
				Validators: []validator.String{
					stringvalidator.OneOf(models.GroupingScopeOrganisation, models.GroupingScopeTeam),
				},
			},
			"team_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: apischema.Docstring("TeamGroupingPreferenceV3", "team_id") +
					". Required when `scope` is `team`, and not allowed otherwise. It can't change.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("TeamGroupingPreferenceV3", "version"),
			},
			"default": models.GroupingDefaultAttribute("The grouping settings for alerts no branch matches."),
		},
		Blocks: models.GroupingPreferenceBlocks(),
	}
}

func (r *IncidentGroupingPreferenceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.GroupingPreferenceResourceModel

	// A value another resource computes can't decode into the model, so skip until it is known.
	if req.Config.Get(ctx, &data).HasError() {
		return
	}

	models.ValidateGroupingPreference(data, &resp.Diagnostics)
}

// ModifyPlan keeps each branch's ID with its branch, and refuses at plan what the API would
// refuse at apply: a new scope or team, or a preference that already exists.
func (r *IncidentGroupingPreferenceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	// Read the scope and team on their own, so an unknown elsewhere in the plan can't skip
	// these checks.
	var scope, teamID types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("scope"), &scope)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("team_id"), &teamID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !req.State.Raw.IsNull() {
		var priorScope, priorTeamID types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("scope"), &priorScope)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("team_id"), &priorTeamID)...)
		if resp.Diagnostics.HasError() {
			return
		}

		switch {
		case !scope.IsUnknown() && !scope.Equal(priorScope):
			resp.Diagnostics.AddAttributeError(path.Root("scope"), "Scope cannot change",
				"A grouping preference can't change between the organisation default and a team's preference. "+
					"Remove this resource and add a new one.")
		case !teamID.IsUnknown() && !teamID.Equal(priorTeamID):
			resp.Diagnostics.AddAttributeError(path.Root("team_id"), "Team cannot change",
				"A team's grouping preference can't move to another team. Remove this resource and add a new one for that team.")
		}

		r.carryBranchIDs(ctx, req, resp)
		return
	}

	if r.client == nil || scope.IsUnknown() || teamID.IsUnknown() {
		return
	}
	// ModifyPlan isn't told the resource's address, but Terraform prints it above this message.
	if id, ok := r.findExistingID(ctx, scope.ValueString(), teamID); ok {
		what := "a default grouping preference"
		if scope.ValueString() == models.GroupingScopeTeam {
			what = "a grouping preference for this team"
		}
		resp.Diagnostics.AddAttributeError(path.Root("scope"), "Grouping preference already exists",
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
func (r *IncidentGroupingPreferenceResource) carryBranchIDs(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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

func (r *IncidentGroupingPreferenceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.GroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state models.GroupingPreferenceResourceModel
	if plan.Scope.ValueString() == models.GroupingScopeTeam {
		payload, err := plan.ToTeamCreatePayload(plan.TeamID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid grouping preference", err.Error())
			return
		}
		result, err := r.client.TeamGroupingPreferencesV3CreateWithResponse(ctx, payload)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create grouping preference, got error: %s", err))
			return
		}
		if result.JSON201 == nil {
			resp.Diagnostics.AddError("Client Error",
				fmt.Sprintf("Unable to create grouping preference: unexpected response from API (status %s)", result.Status()))
			return
		}
		state = models.GroupingPreferenceResourceModel{}.FromTeamAPI(result.JSON201.TeamGroupingPreference, &plan)
	} else {
		payload, err := plan.ToDefaultCreatePayload()
		if err != nil {
			resp.Diagnostics.AddError("Invalid grouping preference", err.Error())
			return
		}
		result, err := r.client.DefaultGroupingPreferencesV3CreateWithResponse(ctx, payload)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create grouping preference, got error: %s", err))
			return
		}
		if result.JSON201 == nil {
			resp.Diagnostics.AddError("Client Error",
				fmt.Sprintf("Unable to create grouping preference: unexpected response from API (status %s)", result.Status()))
			return
		}
		state = models.GroupingPreferenceResourceModel{}.FromDefaultAPI(result.JSON201.DefaultGroupingPreference, &plan)
	}

	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, state.ID.ValueString(), &resp.Diagnostics,
			groupingPreferenceManagedType(state.Scope.ValueString()), r.terraformVersion)
	}
	tflog.Trace(ctx, fmt.Sprintf("created a grouping preference with id=%s", state.ID.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentGroupingPreferenceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.GroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	newState, found := r.show(ctx, state.Scope.ValueString(), state.ID.ValueString(), &state, resp.Diagnostics.AddError)
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

func (r *IncidentGroupingPreferenceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.GroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// State names the object this updates, whatever the plan says.
	var prior models.GroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope, id := prior.Scope.ValueString(), prior.ID.ValueString()

	// The API takes the version this update writes, one more than the current one, so a
	// concurrent edit is refused rather than overwritten.
	current, found := r.show(ctx, scope, id, nil, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		tflog.Warn(ctx, fmt.Sprintf("Grouping preference with ID %s not found: removing from state.", id))
		resp.State.RemoveResource(ctx)
		return
	}
	version := current.Version.ValueInt64() + 1

	var state models.GroupingPreferenceResourceModel
	if scope == models.GroupingScopeTeam {
		payload, err := plan.ToTeamUpdatePayload(version)
		if err != nil {
			resp.Diagnostics.AddError("Invalid grouping preference", err.Error())
			return
		}
		result, err := r.client.TeamGroupingPreferencesV3UpdateWithResponse(ctx, id, payload)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update grouping preference, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error",
				fmt.Sprintf("Unable to update grouping preference: unexpected response from API (status %s)", result.Status()))
			return
		}
		state = models.GroupingPreferenceResourceModel{}.FromTeamAPI(result.JSON200.TeamGroupingPreference, &plan)
	} else {
		payload, err := plan.ToDefaultUpdatePayload(version)
		if err != nil {
			resp.Diagnostics.AddError("Invalid grouping preference", err.Error())
			return
		}
		result, err := r.client.DefaultGroupingPreferencesV3UpdateWithResponse(ctx, id, payload)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update grouping preference, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error",
				fmt.Sprintf("Unable to update grouping preference: unexpected response from API (status %s)", result.Status()))
			return
		}
		state = models.GroupingPreferenceResourceModel{}.FromDefaultAPI(result.JSON200.DefaultGroupingPreference, &plan)
	}

	managedType := groupingPreferenceManagedType(scope)
	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, id, &resp.Diagnostics, managedType, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, id, &resp.Diagnostics, managedType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete removes a team's preference. The API turns the organisation default's grouping off
// and hands it back to the dashboard instead, as it always exists once saved.
func (r *IncidentGroupingPreferenceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.GroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var err error
	if state.Scope.ValueString() == models.GroupingScopeTeam {
		_, err = r.client.TeamGroupingPreferencesV3DeleteWithResponse(ctx, state.ID.ValueString())
	} else {
		_, err = r.client.DefaultGroupingPreferencesV3DeleteWithResponse(ctx, state.ID.ValueString())
	}
	// A 404 is a preference already gone. The organisation default's endpoints also answer 404
	// while the organisation can't see alert rules, so then state is dropped without the
	// default being handed back.
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete grouping preference, got error: %s", err))
	}
}

// ImportState takes the preference's ID, for a team's preference and the organisation
// default alike: the two share one ID space, so the ID says which it is.
func (r *IncidentGroupingPreferenceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	state, found := r.show(ctx, models.GroupingScopeTeam, req.ID, nil, resp.Diagnostics.AddError)
	if !found && !resp.Diagnostics.HasError() {
		state, found = r.show(ctx, models.GroupingScopeOrganisation, req.ID, nil, resp.Diagnostics.AddError)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError("Grouping Preference Not Found",
			fmt.Sprintf("No team grouping preference or default grouping preference with ID %q exists.", req.ID))
		return
	}

	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		groupingPreferenceManagedType(state.Scope.ValueString()), r.terraformVersion, r.markImportedAsManaged)
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
//	  to   = incident_grouping_preference.payments
//	}
//
// The attributes both resources hold are carried across, and the old one only ever held a
// team's preference. Terraform refreshes the moved resource before it plans, which reads the
// branches and expressions the old resource never held.
func (r *IncidentGroupingPreferenceResource) MoveState(ctx context.Context) []resource.StateMover {
	source := NewIncidentTeamGroupingPreferenceResource()
	mover := migrateStateMover(resourceTypeName(ctx, source), declaredResourceSchema(ctx, source), declaredResourceSchema(ctx, r))

	carry := mover.StateMover
	mover.StateMover = func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
		carry(ctx, req, resp)
		if resp.Diagnostics.HasError() || resp.TargetState.Raw.IsNull() {
			return
		}
		resp.Diagnostics.Append(resp.TargetState.SetAttribute(ctx, path.Root("scope"), models.GroupingScopeTeam)...)
	}

	return []resource.StateMover{mover}
}

// show reads one preference from its scope's endpoint, reporting a 404 as not found so each
// caller decides what a missing preference means. prior is passed on to FromAPI.
func (r *IncidentGroupingPreferenceResource) show(
	ctx context.Context, scope, id string, prior *models.GroupingPreferenceResourceModel, addError func(summary, detail string),
) (*models.GroupingPreferenceResourceModel, bool) {
	var (
		preference *models.GroupingPreferenceResourceModel
		status     string
		err        error
	)
	if scope == models.GroupingScopeTeam {
		var result *client.TeamGroupingPreferencesV3ShowResponse
		if result, err = r.client.TeamGroupingPreferencesV3ShowWithResponse(ctx, id); err == nil {
			status = result.Status()
			if result.JSON200 != nil {
				preference = lo.ToPtr(models.GroupingPreferenceResourceModel{}.FromTeamAPI(result.JSON200.TeamGroupingPreference, prior))
			}
		}
	} else {
		var result *client.DefaultGroupingPreferencesV3ShowResponse
		if result, err = r.client.DefaultGroupingPreferencesV3ShowWithResponse(ctx, id); err == nil {
			status = result.Status()
			if result.JSON200 != nil {
				preference = lo.ToPtr(models.GroupingPreferenceResourceModel{}.FromDefaultAPI(result.JSON200.DefaultGroupingPreference, prior))
			}
		}
	}

	switch {
	case preference != nil:
		return preference, true
	case isNotFound(err):
	case err != nil:
		addError("Client Error", fmt.Sprintf("Unable to read grouping preference, got error: %s", err))
	default:
		addError("Client Error", fmt.Sprintf("Unable to read grouping preference: unexpected response from API (status %s)", status))
	}

	return nil, false
}

// findExistingID finds the preference a create would collide with: the organisation default,
// or the team's preference.
func (r *IncidentGroupingPreferenceResource) findExistingID(ctx context.Context, scope string, teamID types.String) (string, bool) {
	if scope == models.GroupingScopeTeam {
		result, err := r.client.TeamGroupingPreferencesV3ListWithResponse(ctx, &client.TeamGroupingPreferencesV3ListParams{
			TeamId: teamID.ValueStringPointer(),
		})
		if err != nil || result.JSON200 == nil || len(result.JSON200.TeamGroupingPreferences) == 0 {
			return "", false
		}
		return result.JSON200.TeamGroupingPreferences[0].Id, true
	}

	result, err := r.client.DefaultGroupingPreferencesV3ListWithResponse(ctx)
	if err != nil || result.JSON200 == nil || len(result.JSON200.DefaultGroupingPreferences) == 0 {
		return "", false
	}
	return result.JSON200.DefaultGroupingPreferences[0].Id, true
}

// groupingPreferenceManagedType is the type a claim names. The API locks the default in the
// dashboard by its own type, and refuses to claim it as a team's preference.
func groupingPreferenceManagedType(scope string) client.ManagedResourcesCreateManagedResourcePayloadV2ResourceType {
	if scope == models.GroupingScopeTeam {
		return client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeTeamGroupingPreference
	}
	return client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeDefaultGroupingPreference
}
