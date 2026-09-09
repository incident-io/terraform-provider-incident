package provider

import (
	"context"
	"fmt"
	"time"

	// Embeds the timezone database so validating a timezone doesn't depend on the
	// machine running Terraform having system zoneinfo — otherwise the same config
	// would validate on a laptop and fail in a slim CI image.
	_ "time/tzdata"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ resource.Resource                   = &IncidentScheduleResource{}
	_ resource.ResourceWithConfigure      = &IncidentScheduleResource{}
	_ resource.ResourceWithImportState    = &IncidentScheduleResource{}
	_ resource.ResourceWithMoveState      = &IncidentScheduleResource{}
	_ resource.ResourceWithValidateConfig = &IncidentScheduleResource{}
)

func NewIncidentScheduleResource() resource.Resource {
	return &IncidentScheduleResource{}
}

type IncidentScheduleResource struct {
	resourceConfigurer
}

// IncidentScheduleModel is the Terraform state/plan shape for the resource.
type IncidentScheduleModel struct {
	ID                   types.String                    `tfsdk:"id"`
	Name                 types.String                    `tfsdk:"name"`
	Timezone             types.String                    `tfsdk:"timezone"`
	TeamIDs              types.Set                       `tfsdk:"team_ids"`
	HolidaysPublicConfig *IncidentScheduleHolidaysConfig `tfsdk:"holidays_public_config"`
}

type IncidentScheduleHolidaysConfig struct {
	CountryCodes []types.String `tfsdk:"country_codes"`
}

func (r *IncidentScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (r *IncidentScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manage an on-call schedule.

A schedule holds the rotations that decide who is on call. This resource manages
only the schedule itself — its name, timezone, owning teams and public holidays.
Rotations are managed separately, as ` + "`incident_schedule_rotation`" + `, so
adding or editing one never means rewriting the whole schedule.

A schedule created here starts with no rotations, and nobody is on call until one
is added.

## What changed in v7

Before v7, a schedule and every one of its rotations were declared in a single
resource. A change to one rotation rewrote the whole schedule, which is why
editing a rotation could disturb who was on call elsewhere on it, or drop
overrides attached to a shift.

Splitting the two apart confines a change to the rotation you edited, and brings
controls the old shape had no way to express: ` + "`rollout`" + ` decides when a
changed line-up takes over rather than swapping it under whoever is on call right
now, ` + "`scheduling_mode`" + ` decides how people are allocated across shifts of
differing length, and ` + "`working_intervals`" + ` restricts a rotation to given hours.

Moving a configuration across is not a rename, because one old resource becomes
one schedule plus one ` + "`incident_schedule_rotation`" + ` per rotation. Import the
schedule and each of its rotations by ID rather than recreating them, so nobody's
on-call history is disturbed - the [v7 migration
guide](https://registry.terraform.io/providers/incident-io/incident/latest/docs/guides/migrating-to-v7)
has the field-by-field mapping and the import IDs.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleV3", "name"),
			},
			"timezone": schema.StringAttribute{
				Required: true,
				MarkdownDescription: apischema.Docstring("ScheduleV3", "timezone") +
					". Changing this replaces the schedule: a timezone is what its rotations are " +
					"anchored to, and we don't support moving an existing schedule to another one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"team_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("ScheduleV3", "team_ids"),
			},
			"holidays_public_config": schema.SingleNestedAttribute{
				Optional: true,
				MarkdownDescription: "Public holidays to show on this schedule. Omit the block " +
					"entirely to show none.",
				Attributes: map[string]schema.Attribute{
					"country_codes": schema.ListAttribute{
						Required:            true,
						ElementType:         types.StringType,
						MarkdownDescription: apischema.Docstring("ScheduleHolidaysPublicConfigV2", "country_codes"),
					},
				},
			},
		},
	}
}

// ValidateConfig checks what it can without calling the API, so `terraform
// validate` and plan catch mistakes that would otherwise apply cleanly and behave
// oddly afterwards.
//
// It reads attributes rather than decoding IncidentScheduleModel, because the model
// holds country_codes as []types.String, which can't represent a list that isn't
// known yet — decoding a config whose codes come from another resource's output
// would fail here instead of validating.
func (r *IncidentScheduleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	r.validateTimezone(ctx, req, resp)
	r.validateHolidays(ctx, req, resp)
}

// validateTimezone rejects a timezone the runtime doesn't know. The API validates
// this too, but only at apply — by which point Terraform has already decided to
// replace the schedule, since timezone is force-new.
func (r *IncidentScheduleResource) validateTimezone(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	timezonePath := path.Root("timezone")

	var timezone types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, timezonePath, &timezone)...)
	if resp.Diagnostics.HasError() || timezone.IsNull() || timezone.IsUnknown() {
		return
	}

	value := timezone.ValueString()

	// LoadLocation resolves "" to UTC and "Local" to the machine's zone, neither of
	// which is a schedule timezone — the API rejects both, so reject them here too.
	_, err := time.LoadLocation(value)
	if err != nil || value == "" || value == "Local" {
		resp.Diagnostics.AddAttributeError(
			timezonePath,
			"Invalid timezone",
			fmt.Sprintf("%q isn't an IANA timezone name. Use a name like Europe/London or America/Los_Angeles.", value),
		)
	}
}

// validateHolidays checks the country codes are well-formed. This is the only
// place a mistake surfaces: the API matches codes against its holiday database and
// silently ignores anything it doesn't recognise, so a typo applies successfully
// and then quietly shows no holidays.
//
// It deliberately checks the shape rather than membership of a country list. Which
// countries we hold holidays for is the server's business, and a copy of that list
// here would drift and start rejecting codes the API is happy with.
func (r *IncidentScheduleResource) validateHolidays(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	holidaysPath := path.Root("holidays_public_config")

	var holidays types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, holidaysPath, &holidays)...)
	if resp.Diagnostics.HasError() || holidays.IsNull() || holidays.IsUnknown() {
		return
	}

	codesPath := holidaysPath.AtName("country_codes")

	var countryCodes types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, codesPath, &countryCodes)...)
	if resp.Diagnostics.HasError() || countryCodes.IsNull() || countryCodes.IsUnknown() {
		return
	}

	if len(countryCodes.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(
			codesPath,
			"Empty country_codes",
			"Set at least one country code, or remove the holidays_public_config block to show no holidays.",
		)
		return
	}

	var codes []types.String
	resp.Diagnostics.Append(countryCodes.ElementsAs(ctx, &codes, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	seen := map[string]bool{}
	for i, code := range codes {
		elementPath := codesPath.AtListIndex(i)
		if code.IsUnknown() {
			continue
		}
		if code.IsNull() {
			resp.Diagnostics.AddAttributeError(elementPath, "Empty country code",
				"Country codes must be ISO 3166-1 alpha-2, like GB or FR.")
			continue
		}

		value := code.ValueString()
		if !isISO3166Alpha2(value) {
			resp.Diagnostics.AddAttributeError(elementPath, "Invalid country code",
				fmt.Sprintf("%q isn't an ISO 3166-1 alpha-2 country code. Use two uppercase letters, like GB or FR.", value))
			continue
		}
		if seen[value] {
			resp.Diagnostics.AddAttributeError(elementPath, "Duplicate country code",
				fmt.Sprintf("%q is listed more than once.", value))
			continue
		}
		seen[value] = true
	}
}

// isISO3166Alpha2 reports whether the value is two uppercase ASCII letters, the
// shape of an ISO 3166-1 alpha-2 country code.
func isISO3166Alpha2(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func (r *IncidentScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data IncidentScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamIDs := r.toTeamIDsPayload(ctx, data.TeamIDs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SchedulesV3CreateWithResponse(ctx, client.SchedulesV3CreateJSONRequestBody{
		Schedule: client.ScheduleCreatePayloadV3{
			Name:                 data.Name.ValueString(),
			Timezone:             data.Timezone.ValueString(),
			TeamIds:              teamIDs,
			HolidaysPublicConfig: toHolidaysPayload(data.HolidaysPublicConfig),
			Annotations:          r.annotations(),
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create schedule", err.Error())
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Unable to create schedule", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentScheduleFromAPI(result.JSON201.Schedule, data.TeamIDs))...)
}

func (r *IncidentScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data IncidentScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SchedulesV3ShowWithResponse(ctx, data.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read schedule", err.Error())
		return
	}
	// The client turns any non-2xx into an error, so a missing body here is an
	// unexpected success response rather than a deleted schedule. Dropping it from
	// state would make the next apply create a second schedule, so fail instead.
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to read schedule", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentScheduleFromAPI(result.JSON200.Schedule, data.TeamIDs))...)
}

func (r *IncidentScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state IncidentScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Always send team_ids, as an empty list when the attribute is unset: omitting
	// it tells the API to leave ownership alone, which would strand a schedule on
	// its old teams after they're removed from the config.
	teamIDs := r.toTeamIDsPayload(ctx, plan.TeamIDs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if teamIDs == nil {
		teamIDs = &[]string{}
	}

	result, err := r.client.SchedulesV3UpdateWithResponse(ctx, state.ID.ValueString(), client.SchedulesV3UpdateJSONRequestBody{
		Schedule: client.ScheduleUpdatePayloadV3{
			Name:                 plan.Name.ValueString(),
			TeamIds:              teamIDs,
			HolidaysPublicConfig: toHolidaysPayload(plan.HolidaysPublicConfig),
			Annotations:          r.annotations(),
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update schedule", err.Error())
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to update schedule", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentScheduleFromAPI(result.JSON200.Schedule, plan.TeamIDs))...)
}

func (r *IncidentScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data IncidentScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.SchedulesV3DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete schedule", err.Error())
	}
}

func (r *IncidentScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Create and Update carry the Terraform annotation in their payload, but an
	// import writes nothing, so claim the schedule here instead.
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeSchedule, r.terraformVersion, r.markImportedAsManaged)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// MoveState takes the state of a `incident_schedule_beta`, which is the name this
// resource went by before v7. The schema is the same one, so a `moved` block is
// all it takes to bring a schedule across:
//
//	moved {
//	  from = incident_schedule_beta.primary
//	  to   = incident_schedule.primary
//	}
func (r *IncidentScheduleResource) MoveState(ctx context.Context) []resource.StateMover {
	return []resource.StateMover{
		renameStateMover("incident_schedule_beta", declaredResourceSchema(ctx, r)),
	}
}

func (r *IncidentScheduleResource) annotations() *map[string]string {
	return &map[string]string{
		"incident.io/terraform/version": r.terraformVersion,
	}
}

// toTeamIDsPayload decodes the team_ids set into the API payload, or returns nil
// when unset.
func (r *IncidentScheduleResource) toTeamIDsPayload(ctx context.Context, set types.Set, diags *diag.Diagnostics) *[]string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	ids := []string{}
	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	if diags.HasError() {
		return nil
	}
	return &ids
}

func toHolidaysPayload(config *IncidentScheduleHolidaysConfig) *client.ScheduleHolidaysPublicConfigPayloadV2 {
	if config == nil {
		return nil
	}

	codes := make([]string, len(config.CountryCodes))
	for i, code := range config.CountryCodes {
		codes[i] = code.ValueString()
	}
	return &client.ScheduleHolidaysPublicConfigPayloadV2{CountryCodes: codes}
}

// incidentScheduleFromAPI projects an API schedule into Terraform state. configTeamIDs
// is whatever the plan or prior state held, which is needed to tell "owned by
// nobody" apart from "attribute not set" — see teamIDsToState.
func incidentScheduleFromAPI(schedule client.ScheduleV3, configTeamIDs types.Set) *IncidentScheduleModel {
	var holidays *IncidentScheduleHolidaysConfig
	if schedule.HolidaysPublicConfig != nil {
		codes := make([]types.String, len(schedule.HolidaysPublicConfig.CountryCodes))
		for i, code := range schedule.HolidaysPublicConfig.CountryCodes {
			codes[i] = types.StringValue(code)
		}
		holidays = &IncidentScheduleHolidaysConfig{CountryCodes: codes}
	}

	return &IncidentScheduleModel{
		ID:                   types.StringValue(schedule.Id),
		Name:                 types.StringValue(schedule.Name),
		Timezone:             types.StringValue(schedule.Timezone),
		TeamIDs:              teamIDsToState(schedule.TeamIds, configTeamIDs),
		HolidaysPublicConfig: holidays,
	}
}

// teamIDsToState decides how to store the team_ids the API returned.
//
// The API always sends team_ids, using an empty list for a schedule nobody owns,
// while HCL can either omit the attribute or set it to []. Storing [] against an
// omitted attribute would show a diff on every plan, so when the API says "no
// teams" we keep whichever form the config used.
func teamIDsToState(ids []string, configTeamIDs types.Set) types.Set {
	if len(ids) == 0 && configTeamIDs.IsNull() {
		return types.SetNull(types.StringType)
	}

	elements := make([]attr.Value, len(ids))
	for i, id := range ids {
		elements[i] = types.StringValue(id)
	}
	return types.SetValueMust(types.StringType, elements)
}
