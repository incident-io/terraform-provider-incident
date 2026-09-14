package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ resource.Resource                   = &IncidentTeamGroupingPreferenceResource{}
	_ resource.ResourceWithConfigure      = &IncidentTeamGroupingPreferenceResource{}
	_ resource.ResourceWithImportState    = &IncidentTeamGroupingPreferenceResource{}
	_ resource.ResourceWithValidateConfig = &IncidentTeamGroupingPreferenceResource{}
)

type IncidentTeamGroupingPreferenceResource struct {
	resourceConfigurer
}

func NewIncidentTeamGroupingPreferenceResource() resource.Resource {
	return &IncidentTeamGroupingPreferenceResource{}
}

func (r *IncidentTeamGroupingPreferenceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_grouping_preference"
}

func (r *IncidentTeamGroupingPreferenceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Team Grouping Preferences V3"),
			`A team has at most one preference, and it cannot move to another team: changing `+"`team_id`"+` destroys the
preference and creates one for the new team. The settings use the field names of an alert route's
`+"`grouping_config`"+`, so a team's preference can be written by copying the route block it replaces.

Writing a preference needs team grouping preferences enabled for your organisation. Until then the
API refuses to create, update or delete one, and this resource fails at apply with that message.`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("TeamGroupingPreferenceV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("TeamGroupingPreferenceV3", "team_id") + " A preference cannot move to another team, so changing this replaces it.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("TeamGroupingPreferenceV3", "version"),
			},
			"default": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "The grouping settings applied to this team's alerts on every alert route they match.",
				Attributes: map[string]schema.Attribute{
					"settings": schema.SingleNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("TeamGroupingBranchV3", "settings"),
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								Required:            true,
								MarkdownDescription: apischema.Docstring("TeamGroupingSettingsV3", "enabled"),
							},
							"grouping_keys": schema.SetNestedAttribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("TeamGroupingSettingsV3", "grouping_keys"),
								NestedObject: schema.NestedAttributeObject{
									Attributes: map[string]schema.Attribute{
										"reference": schema.StringAttribute{
											Required:            true,
											MarkdownDescription: apischema.Docstring("GroupingKeyV3", "reference"),
										},
									},
								},
							},
							"window_seconds": schema.Int64Attribute{
								Optional:            true,
								MarkdownDescription: apischema.Docstring("TeamGroupingSettingsV3", "window_seconds"),
							},
							"window_type": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: EnumValuesDescription("TeamGroupingSettingsV3", "window_type"),
							},
						},
					},
				},
			},
		},
	}
}

// ValidateConfig applies the rules the API enforces on the settings at plan time: an
// enabled preference needs its window, a disabled one carries no window or keys.
func (r *IncidentTeamGroupingPreferenceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	settings := path.Root("default").AtName("settings")

	var enabled types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, settings.AtName("enabled"), &enabled)...)
	if resp.Diagnostics.HasError() || enabled.IsNull() || enabled.IsUnknown() {
		return
	}

	var windowSeconds types.Int64
	var windowType types.String
	var groupingKeys types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, settings.AtName("window_seconds"), &windowSeconds)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, settings.AtName("window_type"), &windowType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, settings.AtName("grouping_keys"), &groupingKeys)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if enabled.ValueBool() {
		if windowSeconds.IsNull() {
			resp.Diagnostics.AddAttributeError(settings.AtName("window_seconds"), "Missing required attribute",
				"`window_seconds` is required when `default.settings.enabled` is true.")
		}
		if windowType.IsNull() {
			resp.Diagnostics.AddAttributeError(settings.AtName("window_type"), "Missing required attribute",
				"`window_type` is required when `default.settings.enabled` is true.")
		}
		return
	}

	// An unknown value may still resolve to null at apply, so only a known value is a conflict.
	if !windowSeconds.IsNull() && !windowSeconds.IsUnknown() {
		resp.Diagnostics.AddAttributeError(settings.AtName("window_seconds"), "Invalid attribute combination",
			"`window_seconds` must not be set when `default.settings.enabled` is false.")
	}
	if !windowType.IsNull() && !windowType.IsUnknown() {
		resp.Diagnostics.AddAttributeError(settings.AtName("window_type"), "Invalid attribute combination",
			"`window_type` must not be set when `default.settings.enabled` is false.")
	}
	if !groupingKeys.IsNull() && !groupingKeys.IsUnknown() {
		resp.Diagnostics.AddAttributeError(settings.AtName("grouping_keys"), "Invalid attribute combination",
			"`grouping_keys` must not be set when `default.settings.enabled` is false.")
	}
}

func (r *IncidentTeamGroupingPreferenceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.TeamGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.TeamGroupingPreferencesV3CreateWithResponse(ctx, plan.ToCreatePayload())
	if err != nil {
		if isAPINotYetAvailable(err) {
			resp.Diagnostics.AddError(teamGroupingPreferenceUnavailableError())
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create team grouping preference, got error: %s", err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to create team grouping preference: unexpected response from API (status %s)", result.Status()))
		return
	}

	preference := result.JSON201.TeamGroupingPreference
	claimResource(ctx, r.client, preference.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeTeamGroupingPreference, r.terraformVersion)
	tflog.Trace(ctx, fmt.Sprintf("created a team grouping preference with id=%s", preference.Id))

	state := models.TeamGroupingPreferenceResourceModel{}.FromAPI(preference, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentTeamGroupingPreferenceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.TeamGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	preference, found := r.show(ctx, state.ID.ValueString(), resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		tflog.Warn(ctx, fmt.Sprintf("Team grouping preference with ID %s not found: removing from state.", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	newState := models.TeamGroupingPreferenceResourceModel{}.FromAPI(*preference, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *IncidentTeamGroupingPreferenceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.TeamGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API wants the version this update creates, one more than the current one, so a
	// concurrent edit is rejected rather than silently overwritten.
	current, found := r.show(ctx, plan.ID.ValueString(), resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		tflog.Warn(ctx, fmt.Sprintf("Team grouping preference with ID %s not found: removing from state.", plan.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	result, err := r.client.TeamGroupingPreferencesV3UpdateWithResponse(ctx, plan.ID.ValueString(), plan.ToUpdatePayload(current.Version+1))
	if err != nil {
		if isAPINotYetAvailable(err) {
			resp.Diagnostics.AddError(teamGroupingPreferenceUnavailableError())
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update team grouping preference, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to update team grouping preference: unexpected response from API (status %s)", result.Status()))
		return
	}

	preference := result.JSON200.TeamGroupingPreference
	claimResource(ctx, r.client, preference.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeTeamGroupingPreference, r.terraformVersion)

	state := models.TeamGroupingPreferenceResourceModel{}.FromAPI(preference, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentTeamGroupingPreferenceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.TeamGroupingPreferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.TeamGroupingPreferencesV3DeleteWithResponse(ctx, state.ID.ValueString())
	if err != nil && !isNotFound(err) {
		if isAPINotYetAvailable(err) {
			resp.Diagnostics.AddError(teamGroupingPreferenceUnavailableError())
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete team grouping preference, got error: %s", err))
	}
}

func (r *IncidentTeamGroupingPreferenceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeTeamGroupingPreference, r.terraformVersion,
		r.markImportedAsManaged)
	if resp.Diagnostics.HasError() {
		return
	}

	preference, found := r.show(ctx, req.ID, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError("Team Grouping Preference Not Found",
			fmt.Sprintf("No team grouping preference with ID %q exists.", req.ID))
		return
	}

	state := models.TeamGroupingPreferenceResourceModel{}.FromAPI(*preference, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// show reads one preference, reporting a 404 as not found rather than an error so each
// caller can decide what a missing preference means for it.
func (r *IncidentTeamGroupingPreferenceResource) show(ctx context.Context, id string, addError func(summary, detail string)) (*client.TeamGroupingPreferenceV3, bool) {
	result, err := r.client.TeamGroupingPreferencesV3ShowWithResponse(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil, false
		}
		addError("Client Error", fmt.Sprintf("Unable to read team grouping preference, got error: %s", err))
		return nil, false
	}
	if result.JSON200 == nil {
		addError("Client Error",
			fmt.Sprintf("Unable to read team grouping preference: unexpected response from API (status %s)", result.Status()))
		return nil, false
	}

	return &result.JSON200.TeamGroupingPreference, true
}

func teamGroupingPreferenceUnavailableError() (summary, detail string) {
	return "Team grouping preferences not available",
		"Team grouping preferences are not enabled for your organisation yet, so incident.io refused this " +
			"change. Contact incident.io to have them switched on before managing a preference in Terraform."
}
