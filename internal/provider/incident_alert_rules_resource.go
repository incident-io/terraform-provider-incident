package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ resource.Resource                   = &IncidentAlertRulesResource{}
	_ resource.ResourceWithConfigure      = &IncidentAlertRulesResource{}
	_ resource.ResourceWithImportState    = &IncidentAlertRulesResource{}
	_ resource.ResourceWithValidateConfig = &IncidentAlertRulesResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentAlertRulesResource{}
)

type IncidentAlertRulesResource struct {
	resourceConfigurer
}

func NewIncidentAlertRulesResource() resource.Resource {
	return &IncidentAlertRulesResource{}
}

func (r *IncidentAlertRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_rules"
}

func (r *IncidentAlertRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := models.AlertRuleSetAttributes()
	attributes["unlock_in_dashboard"] = unlockInDashboardAttribute()

	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Rules V3"),
			`A set of rules is one resource: its blocks, branches, actions and the expressions they reference
are written together on every apply.

An organisation has one set of `+"`required`"+` rules and one of `+"`default`"+` rules, and a plan that creates
either fails once it exists. To manage one, import it by ID. Destroying it afterwards empties the set and hands
it back to the dashboard. Each team has at most one set of `+"`team`"+` rules, which is created and deleted like
any other resource. A set's kind cannot change, so a plan that changes `+"`kind`"+` fails, and changing
`+"`team_id`"+` replaces the set.

Writing alert rules needs your organisation to have moved to alert rules. Until then the API refuses to
create, update or delete a set, and this resource fails at apply with that message.`),
		Attributes: attributes,
		Blocks:     models.AlertRuleSetBlocks(),
	}
}

func (r *IncidentAlertRulesResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.AlertRuleSetResourceModel

	// A value another resource computes can't decode into the model, so skip until it is known.
	if req.Config.Get(ctx, &data).HasError() {
		return
	}

	models.ValidateAlertRuleSet(data, &resp.Diagnostics)
}

// ModifyPlan refuses at plan what the API would refuse at apply: a new kind, or a second
// required, default or team set.
func (r *IncidentAlertRulesResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var kind types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("kind"), &kind)...)
	if resp.Diagnostics.HasError() || kind.IsUnknown() {
		return
	}

	if !req.State.Raw.IsNull() {
		var priorKind types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("kind"), &priorKind)...)
		if !resp.Diagnostics.HasError() && !kind.Equal(priorKind) {
			resp.Diagnostics.AddAttributeError(path.Root("kind"), "Kind cannot change",
				"The kind of a set of alert rules can't change. Remove this resource and add a new one.")
		}
		return
	}

	var teamID types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("team_id"), &teamID)...)
	if resp.Diagnostics.HasError() || teamID.IsUnknown() || r.client == nil {
		return
	}
	// A replace, such as a new team_id, plans its create half here too, so skipping this on an
	// error would let the destroy remove the old set before the create fails.
	id, err := r.findExistingID(ctx, kind.ValueString(), teamID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to check for existing alert rules, got error: %s", err))
		return
	}
	// ModifyPlan isn't told the resource's address, but Terraform prints it above this message.
	if id != "" {
		what := fmt.Sprintf("This organisation already has %s rules", kind.ValueString())
		if kind.ValueString() == string(client.AlertRuleSetV3KindTeam) {
			what = "This team already has rules"
		}
		resp.Diagnostics.AddAttributeError(path.Root("kind"), "Alert rules already exist",
			fmt.Sprintf("%[1]s, with ID %[2]s, so they cannot be created. "+
				"To manage them, import them into this resource with an import block:"+
				"\n\n  import {\n    to = <this resource's address, shown above>\n    id = %[2]q\n  }"+
				"\n\nBefore Terraform 1.5, run terraform import with this resource's address and %[2]s instead.",
				what, id))
	}
}

func (r *IncidentAlertRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.AlertRuleSetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, err := plan.ToCreatePayload()
	if err != nil {
		resp.Diagnostics.AddError("Invalid alert rules", err.Error())
		return
	}

	result, err := r.client.AlertRulesV3CreateWithResponse(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert rules, got error: %s", err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to create alert rules: unexpected response from API (status %s)", result.Status()))
		return
	}

	set := result.JSON201.AlertRuleSet
	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, set.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertRule, r.terraformVersion)
	}
	tflog.Trace(ctx, fmt.Sprintf("created a set of alert rules with id=%s", set.Id))

	state := models.AlertRuleSetResourceModel{}.FromAPI(set, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAlertRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.AlertRuleSetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	set, found := r.show(ctx, state.ID.ValueString(), resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		tflog.Warn(ctx, fmt.Sprintf("Alert rules with ID %s not found: removing from state.", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	newState := models.AlertRuleSetResourceModel{}.FromAPI(*set, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *IncidentAlertRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.AlertRuleSetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API takes the version this update writes, one more than the current one.
	current, found := r.show(ctx, plan.ID.ValueString(), resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		tflog.Warn(ctx, fmt.Sprintf("Alert rules with ID %s not found: removing from state.", plan.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	payload, err := plan.ToUpdatePayload(current.Version + 1)
	if err != nil {
		resp.Diagnostics.AddError("Invalid alert rules", err.Error())
		return
	}

	result, err := r.client.AlertRulesV3UpdateWithResponse(ctx, plan.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert rules, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to update alert rules: unexpected response from API (status %s)", result.Status()))
		return
	}

	set := result.JSON200.AlertRuleSet
	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, set.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertRule, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, set.Id, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertRule)
	}

	state := models.AlertRuleSetResourceModel{}.FromAPI(set, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete removes team rules. The API empties required and default rules and unclaims them instead.
func (r *IncidentAlertRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.AlertRuleSetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.AlertRulesV3DeleteWithResponse(ctx, state.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert rules, got error: %s", err))
	}
}

func (r *IncidentAlertRulesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertRule, r.terraformVersion,
		r.markImportedAsManaged)
	if resp.Diagnostics.HasError() {
		return
	}

	set, found := r.show(ctx, req.ID, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError("Alert Rules Not Found", fmt.Sprintf("No set of alert rules with ID %q exists.", req.ID))
		return
	}

	state := models.AlertRuleSetResourceModel{}.FromAPI(*set, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// show reports a 404 as not found, so each caller decides what a missing set means.
func (r *IncidentAlertRulesResource) show(ctx context.Context, id string, addError func(summary, detail string)) (*client.AlertRuleSetV3, bool) {
	result, err := r.client.AlertRulesV3ShowWithResponse(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil, false
		}
		addError("Client Error", fmt.Sprintf("Unable to read alert rules, got error: %s", err))
		return nil, false
	}
	if result.JSON200 == nil {
		addError("Client Error",
			fmt.Sprintf("Unable to read alert rules: unexpected response from API (status %s)", result.Status()))
		return nil, false
	}

	return &result.JSON200.AlertRuleSet, true
}

// findExistingID finds the existing rules of this kind, or this team's, which the API won't
// create again. It returns an empty ID when there are none.
func (r *IncidentAlertRulesResource) findExistingID(ctx context.Context, kind string, teamID types.String) (string, error) {
	result, err := r.client.AlertRulesV3ListWithResponse(ctx, &client.AlertRulesV3ListParams{
		Kind:   lo.ToPtr(client.AlertRulesV3ListParamsKind(kind)),
		TeamId: teamID.ValueStringPointer(),
	})
	if err != nil {
		return "", err
	}
	if result.JSON200 == nil {
		return "", fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if len(result.JSON200.AlertRuleSets) == 0 {
		return "", nil
	}

	return result.JSON200.AlertRuleSets[0].Id, nil
}
