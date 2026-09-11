package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// apiKeyDefaultGracePeriodMinutes is how long a rotated key's previous token keeps working
// when the configuration doesn't say. It matches the API's own documented example, and
// gives whatever is holding the old token a window to pick up the new one before it stops
// being accepted.
const apiKeyDefaultGracePeriodMinutes = 30

// apiKeyValidateTimeout keeps an unresponsive API from stalling a plan. A var so tests
// needn't wait it out.
var apiKeyValidateTimeout = 10 * time.Second

var (
	_ resource.Resource                   = &IncidentAPIKeyResource{}
	_ resource.ResourceWithConfigure      = &IncidentAPIKeyResource{}
	_ resource.ResourceWithImportState    = &IncidentAPIKeyResource{}
	_ resource.ResourceWithValidateConfig = &IncidentAPIKeyResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentAPIKeyResource{}
)

type IncidentAPIKeyResource struct {
	resourceConfigurer
}

func NewIncidentAPIKeyResource() resource.Resource {
	return &IncidentAPIKeyResource{}
}

func (r *IncidentAPIKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *IncidentAPIKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("API Keys V1"),
			"Manages an API key: the credential an integration authenticates to incident.io with, and the "+
				"roles that decide what it may do.\n\n"+
				"## The token\n\n"+
				"incident.io returns a key's token when it issues one - when the key is created, and each "+
				"time it is rotated - and at no other time. A read cannot recover it, so Terraform keeps it "+
				"in state, in the `token` attribute. It is marked sensitive, so Terraform will not print it, "+
				"but **anything that can read your state can read the token**: treat the state file as the "+
				"credential it now contains, and pass `token` on to wherever it is needed through a "+
				"sensitive output or variable rather than copying it around.\n\n"+
				"A key adopted with `terraform import` has no token, because the one it was created with is "+
				"gone and cannot be re-read. Rotate it to get a token Terraform can hand on.\n\n"+
				"## Rotating\n\n"+
				"`token_version` is your own counter, and changing it - conventionally by incrementing it - "+
				"is what asks incident.io to rotate the key. The plan says so, `token` becomes known after "+
				"apply, and the previous token keeps working for `rotation_grace_period_minutes` so that "+
				"whatever holds it has a window to pick up the new one.\n\n"+
				"Editing a key's roles or name never rotates it: the token outlives its permissions, and a "+
				"key whose scopes you have just narrowed carries on working. Rotate deliberately if a "+
				"permissions change means the old token should stop being accepted.\n\n"+
				"## Roles\n\n"+
				"`role_names` grants a role across the whole account. `team_role_names` grants roles only "+
				"for the teams in `team_ids`, so those two go together: set both, or neither.\n\n"+
				"incident.io will not let a key grant more than its own bearer holds, so the key running "+
				"Terraform has to already have every role it assigns. It also refuses to assign "+
				"`api_keys_manage` at all, which means a Terraform-managed key cannot itself manage keys.\n\n"+
				"Neither rule is decided by this provider - which roles your own key holds is not something "+
				"a configuration can know - so the plan asks incident.io whether the key it describes would "+
				"be accepted. A role you cannot grant is reported when you plan, rather than part way "+
				"through an apply. If that check cannot be reached the plan warns and carries on, so the "+
				"rejection may still arrive at apply time.",
		),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("APIKeyV1", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("APIKeyV1", "name"),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"comments": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("APIKeyV1", "comments"),
				Validators: []validator.String{
					// Empty comments are stored as none at all and read back absent, which would
					// not match a config saying "". Removing the attribute is how you clear them.
					stringvalidator.LengthAtLeast(1),
				},
			},
			"role_names": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				// Defaulting to the empty set is what lets a team-scoped key leave this out
				// entirely: the API requires the field and reads an empty array as "no
				// account-level roles", which is exactly what an absent attribute means here.
				Default: setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.Docstring("APIKeysCreatePayloadV1", "role_names"),
					EnumValuesDescription("APIKeyRoleV1", "name")),
			},
			"team_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: apischema.Docstring("APIKeysCreatePayloadV1", "team_ids"),
			},
			"team_role_names": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.Docstring("APIKeysCreatePayloadV1", "team_role_names"),
					EnumValuesDescription("APIKeyTeamRoleV1", "name")),
			},
			"token": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				MarkdownDescription: "The bearer token to authenticate as this key, which incident.io returns only " +
					"when it issues one: on create, and on each rotation. Terraform stores it, because nothing can " +
					"read it back afterwards, so anything with access to your state can read it. Null for a key " +
					"adopted with `terraform import`, whose token was issued before Terraform knew about it - " +
					"rotate the key to get one.",
				PlanModifiers: []planmodifier.String{
					// Not UseStateForUnknown: that would promise the old token on the very plan
					// that rotates it, and the apply would then contradict itself.
					apiKeyRotationAware{},
				},
			},
			"token_version": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "The version of the token this configuration holds. Terraform stores this " +
					"number, so changing it - conventionally by incrementing it - is what asks incident.io to " +
					"rotate the key. It is your own counter, and incident.io never sees it. Leave it unset to " +
					"manage a key's name and roles without ever rotating it.",
			},
			"rotation_grace_period_minutes": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(apiKeyDefaultGracePeriodMinutes),
				MarkdownDescription: fmt.Sprintf(
					"How long the previous token keeps working after a rotation, in minutes, giving whatever "+
						"holds it a window to pick up the new one. Defaults to %d. Set it to 0 to retire the old "+
						"token immediately, at the cost of breaking anything still using it. incident.io "+
						"documents an hour as the longest a rotated token stays valid, so it may reject a longer "+
						"period. Only read when a change to `token_version` rotates the key.",
					apiKeyDefaultGracePeriodMinutes,
				),
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:   true,
				CustomType: timetypes.RFC3339Type{},
				// The API schema describes this one, unlike the secret timestamps.
				MarkdownDescription: apischema.Docstring("APIKeyV1", "created_at"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"token_last_issued_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("APIKeyV1", "token_last_issued_at"),
				PlanModifiers: []planmodifier.String{
					// Moves only when a token is issued, so it holds still unless this plan
					// rotates the key.
					apiKeyRotationAware{},
				},
			},
			"last_used_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("APIKeyV1", "last_used_at"),
				// Deliberately no plan modifier. This moves whenever something authenticates
				// with the key, and Update writes back whatever the API reports, so pinning the
				// stored value would promise a timestamp the apply then contradicts - which is
				// what "Provider produced inconsistent result after apply" is.
				//
				// Leaving it bare does not make every plan an update. The framework only marks
				// a null-in-config Computed attribute unknown when the proposed new state
				// already differs from prior state (MarkComputedNilsAsUnknown, gated in
				// server_planresourcechange.go), so a no-op plan keeps the stored value and
				// stays empty, and a real update gets an unknown for the apply to fill in.
				// `incident_secret`'s updated_at is the same shape, for the same reason.
			},
		},
	}
}

func (r *IncidentAPIKeyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data *models.APIKeyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// Team scoping is a pair: the API rejects teams with no roles to grant for them, and
	// roles with no teams to grant them for. A set built from another resource is unknown
	// until apply, so there's nothing to count yet.
	if !data.TeamIDs.IsUnknown() && !data.TeamRoleNames.IsUnknown() {
		var (
			teams = len(data.TeamIDs.Elements())
			roles = len(data.TeamRoleNames.Elements())
		)

		switch {
		case teams > 0 && roles == 0:
			resp.Diagnostics.AddAttributeError(
				path.Root("team_role_names"),
				"Missing team_role_names",
				"team_ids scopes team_role_names to particular teams, so a key with teams needs at least one "+
					"team role to grant for them. Set team_role_names, or remove team_ids and grant roles "+
					"across the account with role_names instead.",
			)
		case roles > 0 && teams == 0:
			resp.Diagnostics.AddAttributeError(
				path.Root("team_ids"),
				"Missing team_ids",
				"team_role_names grants roles for particular teams, so it needs team_ids saying which. Set "+
					"team_ids, or move the roles to role_names to grant them across the whole account.",
			)
		}
	}

	// A grace period only ever applies to a rotation, and only token_version can ask for
	// one, so setting the period alone is config that will never be read.
	if !data.RotationGracePeriodMinutes.IsNull() && !data.RotationGracePeriodMinutes.IsUnknown() &&
		data.TokenVersion.IsNull() {
		resp.Diagnostics.AddAttributeWarning(
			path.Root("rotation_grace_period_minutes"),
			"rotation_grace_period_minutes has no effect",
			"This key has no token_version, so nothing can ask for it to be rotated and the grace period is "+
				"never read. Set token_version if you want to rotate the key.",
		)
	}
}

// apiKeyValidatedAttributes are the attributes the validate payload is built from. Gating
// on the whole plan instead would skip every create, which plans id and token unknown.
var apiKeyValidatedAttributes = []string{
	"name",
	"comments",
	"role_names",
	"team_ids",
	"team_role_names",
}

// ModifyPlan asks the API whether the planned key would be accepted, so a role the calling
// key cannot grant surfaces here rather than part way through an apply.
//
// This is where the scope rules live: incident.io resolves the role names, checks them
// against the scopes the calling key holds, and applies the account's key limit. None of
// that is knowable from the configuration alone, which is why the provider asks rather than
// deciding for itself - and why the rules can change server-side without a provider release.
//
// It can't live in ValidateConfig, which `terraform validate` also calls: that runs the
// provider without configuring it, so there is no client to ask with.
func (r *IncidentAPIKeyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A destroy plans no key, and an unconfigured provider has no client.
	if r.client == nil || req.Plan.Raw.IsNull() {
		return
	}

	// The framework runs this for every resource in the plan, changed or not. A rejection on
	// a key planning no change isn't something an apply could fix.
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	// A team ID pointing at a catalog entry this same apply creates is unknown until it
	// exists, and validating around the gaps reports errors the apply won't hit.
	if !apiKeyValidateSettled(req.Plan.Raw) {
		return
	}

	var data models.APIKeyModel
	if req.Plan.Get(ctx, &data).HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, apiKeyValidateTimeout)
	defer cancel()

	// The endpoint answers 204 with no body, so a success has nothing to read. The client's
	// middleware turns any status over 299 into an HTTPError, so a rejection arrives here as
	// an error rather than as a response to inspect.
	_, err := r.client.APIKeysV1ValidateWithResponse(ctx, data.ToValidatePayload())
	if err == nil {
		return
	}

	// 422 is the API rejecting this key, which is the whole point.
	var httpErr client.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity {
		resp.Diagnostics.AddError("Invalid API key", httpErr.Error())
		return
	}

	// Anything else means the check didn't run, not that the config is bad, so failing here
	// would break plans that would have applied fine.
	resp.Diagnostics.AddWarning(
		"Could not validate the API key",
		fmt.Sprintf("The API key was not checked, and may still be rejected when you apply: %s", err),
	)
}

// apiKeyValidateSettled reports whether every value the check would send is known. The
// role and team sets are Optional and Computed, so their static defaults land before this
// runs, and anything unknown here is waiting on another resource.
func apiKeyValidateSettled(plan tftypes.Value) bool {
	attributes := map[string]tftypes.Value{}
	if err := plan.As(&attributes); err != nil {
		return false
	}

	for _, name := range apiKeyValidatedAttributes {
		value, ok := attributes[name]
		if !ok || !value.IsFullyKnown() {
			return false
		}
	}

	return true
}

func (r *IncidentAPIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.APIKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.APIKeysV1CreateWithResponse(ctx, data.ToCreatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create API key '%s', got error: %s", data.Name.ValueString(), err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create API key: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON201.ApiKey.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeApiKey, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("created an API key with id=%s", result.JSON201.ApiKey.Id))

	// This response carries the only copy of the token there will ever be.
	state := models.APIKeyModel{}.FromAPI(result.JSON201.ApiKey, models.APIKeyLocals{
		Token:                      types.StringValue(result.JSON201.Token),
		TokenVersion:               data.TokenVersion,
		RotationGracePeriodMinutes: data.RotationGracePeriodMinutes,
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAPIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.APIKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.APIKeysV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("API key with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API key, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read API key: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	// A read says nothing about the token or the practitioner's own counters, so all three
	// carry over from the prior state. A key rotated outside Terraform is the one case this
	// gets wrong, and there is no way to put it right: the new token was returned to
	// whoever rotated it, once.
	state := models.APIKeyModel{}.FromAPI(result.JSON200.ApiKey, models.APIKeyLocals{
		Token:                      data.Token,
		TokenVersion:               data.TokenVersion,
		RotationGracePeriodMinutes: data.RotationGracePeriodMinutes,
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAPIKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.APIKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state models.APIKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.APIKeysV1UpdateWithResponse(ctx, plan.ID.ValueString(), plan.ToUpdatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update API key, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to update API key: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON200.ApiKey.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeApiKey, r.terraformVersion)

	var (
		key   = result.JSON200.ApiKey
		token = state.Token
	)

	// Whether this plan rotates has to be decided exactly as the plan modifier decided it,
	// or the token we return won't be the one the plan promised.
	if apiKeyRotationPlanned(plan.TokenVersion, state.TokenVersion) {
		rotated, newToken, err := r.rotate(ctx, plan.ID.ValueString(), plan.RotationGracePeriodMinutes)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to rotate API key, got error: %s", err))
			return
		}

		tflog.Trace(ctx, fmt.Sprintf("rotated API key with id=%s", rotated.Id))

		// Rotating after the update means this response is the fresher of the two, so it's
		// the one that becomes state.
		key, token = *rotated, types.StringValue(newToken)
	}

	newState := models.APIKeyModel{}.FromAPI(key, models.APIKeyLocals{
		Token:                      token,
		TokenVersion:               plan.TokenVersion,
		RotationGracePeriodMinutes: plan.RotationGracePeriodMinutes,
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *IncidentAPIKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.APIKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.APIKeysV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete API key, got error: %s", err))
		return
	}
	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete API key: %s", result.Body),
		)
	}
}

func (r *IncidentAPIKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeApiKey, r.terraformVersion,
		r.markImportedAsManaged)

	result, err := r.client.APIKeysV1ShowWithResponse(ctx, req.ID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError(
				"API Key Not Found",
				fmt.Sprintf("No API key with ID %q exists.", req.ID),
			)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API key, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read API key: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	// An imported key has no token: the one it was issued with went to whoever created it
	// and cannot be read back. It has no token_version either, so a configuration that sets
	// one plans a rotation on the first apply - which is also the only way to obtain a token
	// for a key Terraform has adopted. The grace period takes its default, so a config that
	// leaves it out matches the imported state.
	data := models.APIKeyModel{}.FromAPI(result.JSON200.ApiKey, models.APIKeyLocals{
		Token:                      types.StringNull(),
		TokenVersion:               types.Int64Null(),
		RotationGracePeriodMinutes: types.Int64Value(apiKeyDefaultGracePeriodMinutes),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// rotate issues a new token for the key, retiring the old one after the grace period.
func (r *IncidentAPIKeyResource) rotate(ctx context.Context, id string, gracePeriod types.Int64) (*client.APIKeyV1, string, error) {
	// The attribute is Computed with a static default, so it is only null or unknown if
	// something has gone unexpectedly wrong; fall back rather than rotating with a zero
	// grace period nobody asked for.
	minutes := int64(apiKeyDefaultGracePeriodMinutes)
	if !gracePeriod.IsNull() && !gracePeriod.IsUnknown() {
		minutes = gracePeriod.ValueInt64()
	}

	result, err := r.client.APIKeysV1RotateWithResponse(ctx, id, client.APIKeysRotatePayloadV1{
		GracePeriodMinutes: minutes,
	})
	if err != nil {
		return nil, "", err
	}
	// The rotate endpoint answers 201, not 200: it is issuing a new token rather than
	// editing the key.
	if result.JSON201 == nil {
		return nil, "", fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON201.ApiKey, result.JSON201.Token, nil
}

// apiKeyRotationPlanned reports whether a change between these two token versions asks for
// the key to be rotated.
//
// Dropping the version is not such a change: a configuration that removes token_version is
// handing the token back rather than asking for a new one, so only a version that is still
// set can rotate. An unknown version might turn out to be either, and is treated as a
// rotation so that the token is not promised to hold still.
func apiKeyRotationPlanned(planned, state types.Int64) bool {
	if planned.IsUnknown() {
		return true
	}
	if planned.IsNull() {
		return false
	}

	return !planned.Equal(state)
}

// apiKeyRotationAware is a plan modifier for the attributes that change if and only if a
// token is issued. It holds the value from state, like UseStateForUnknown, except on a plan
// that rotates the key - where the whole point is that the value changes, and the plan has
// to say it isn't known yet.
type apiKeyRotationAware struct{}

func (m apiKeyRotationAware) Description(ctx context.Context) string {
	return m.MarkdownDescription(ctx)
}

func (m apiKeyRotationAware) MarkdownDescription(_ context.Context) string {
	return "Keeps the value held in state unless this plan rotates the key, in which case it is known after apply."
}

func (m apiKeyRotationAware) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Nothing to carry forward on a create, and a destroy has no plan worth modifying.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	// Only fill in a value the framework doesn't already know: anything else is not ours to
	// overwrite.
	if !req.PlanValue.IsUnknown() {
		return
	}

	planned, state, ok := apiKeyTokenVersions(ctx, req.Plan, req.State, &resp.Diagnostics)
	if !ok {
		return
	}

	// A rotation is coming, so leave the value unknown for the apply to fill in.
	if apiKeyRotationPlanned(planned, state) {
		return
	}

	resp.PlanValue = req.StateValue
}

// apiKeyTokenVersions reads token_version out of the plan and the state. It reports false
// when either can't be read, which leaves the caller to make no promises rather than base
// one on a value it doesn't have.
func apiKeyTokenVersions(ctx context.Context, plan tfsdk.Plan, state tfsdk.State, diags *diag.Diagnostics) (planned types.Int64, stored types.Int64, ok bool) {
	version := path.Root("token_version")

	if diags.Append(plan.GetAttribute(ctx, version, &planned)...); diags.HasError() {
		return planned, stored, false
	}
	if diags.Append(state.GetAttribute(ctx, version, &stored)...); diags.HasError() {
		return planned, stored, false
	}

	return planned, stored, true
}
