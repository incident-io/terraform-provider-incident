package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// secretNameMaxLengthBytes is the API's cap on a secret's name. The limit is on bytes
// rather than characters, so a name of multi-byte characters reaches it sooner.
const secretNameMaxLengthBytes = 1024

// secretTrimmedName matches a name incident.io will store as written. It trims a name
// before storing it, so a padded name reads back as a different string.
var secretTrimmedName = regexp.MustCompile(`(?s)^\S(.*\S)?$`)

var (
	_ resource.Resource                   = &IncidentSecretResource{}
	_ resource.ResourceWithConfigure      = &IncidentSecretResource{}
	_ resource.ResourceWithImportState    = &IncidentSecretResource{}
	_ resource.ResourceWithValidateConfig = &IncidentSecretResource{}
)

type IncidentSecretResource struct {
	resourceConfigurer
}

func NewIncidentSecretResource() resource.Resource {
	return &IncidentSecretResource{}
}

func (r *IncidentSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *IncidentSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Secrets V2"),
			`A secret's value is write-only: incident.io stores it encrypted and never returns it, so
Terraform cannot read it back to compare against your configuration. Set it with `+"`value_wo`"+`, a
[write-only attribute](https://developer.hashicorp.com/terraform/language/resources/ephemeral/write-only),
which needs Terraform 1.11 or OpenTofu 1.11 and above. The value is never written to state or to a
plan file.

Because the value is invisible to Terraform, changing `+"`value_wo`"+` on its own does nothing. Rotating a
secret means changing `+"`value_wo_version`"+` as well, conventionally by incrementing it: that is the
change Terraform can see, and it is what asks incident.io to rotate. A rotation replaces the value
and advances `+"`version`"+`; the previous value is retired and cannot be recovered.

Both value attributes are optional after the secret exists, so Terraform can own a secret's name,
description and owning teams while something else rotates it. Leave them unset and the value is
never touched.`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("SecretV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: apischema.Docstring("SecretV2", "name") +
					fmt.Sprintf(". At most %d bytes, counted in bytes rather than characters, so a name "+
						"using multi-byte characters reaches the limit sooner.", secretNameMaxLengthBytes),
				Validators: []validator.String{
					// The API trims the name before storing it, so a name that is padded or
					// entirely whitespace would read back as something else and fail the
					// post-apply consistency check. Say so at plan time instead.
					stringvalidator.RegexMatches(
						secretTrimmedName,
						"must not start or end with whitespace, which incident.io strips from a secret's name",
					),
					// The API rejects a longer name, so say so at plan time. This validator
					// counts bytes, as the API's limit does, so the two agree on a name
					// holding characters that aren't one byte each.
					stringvalidator.LengthAtMost(secretNameMaxLengthBytes),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("SecretV2", "description"),
				Validators: []validator.String{
					// An empty description is stored as no description and reads back absent,
					// which would not match a config saying "". Removing the attribute is how
					// you clear one.
					stringvalidator.LengthAtLeast(1),
				},
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: apischema.Docstring("SecretV2", "owning_team_ids"),
			},
			"value_wo": schema.StringAttribute{
				Optional:  true,
				WriteOnly: true,
				Sensitive: true,
				MarkdownDescription: "The secret's plaintext value, as a " +
					"[write-only attribute](https://developer.hashicorp.com/terraform/language/resources/ephemeral/write-only): " +
					"it is sent to incident.io and never written to state or to a plan file. Required when creating " +
					"a secret. Changing it alone has no effect, as Terraform cannot see that it changed: change " +
					"`value_wo_version` to rotate the secret.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"value_wo_version": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "The version of `value_wo` this configuration holds. Terraform stores this " +
					"number, so changing it - conventionally by incrementing it - is what asks incident.io to " +
					"rotate the secret to the current `value_wo`. It is your own counter, unrelated to the " +
					"`version` incident.io reports.",
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("SecretV2", "version"),
			},
			"last_four_chars": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("SecretV2", "last_four_chars"),
			},
			"created_at": schema.StringAttribute{
				Computed:   true,
				CustomType: timetypes.RFC3339Type{},
				// The API schema has no description for either timestamp, so these are the
				// provider's own words rather than apischema.Docstring.
				MarkdownDescription: "When this secret was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: "When this secret was last changed, which includes being rotated as well as having its metadata edited.",
			},
		},
	}
}

func (r *IncidentSecretResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data *models.SecretModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that comes from another resource is unknown until apply, and asking whether
	// it was set would be answering a question we can't see the answer to yet.
	if data.ValueWO.IsUnknown() || data.ValueWOVersion.IsUnknown() {
		return
	}

	if !data.ValueWOVersion.IsNull() && data.ValueWO.IsNull() {
		resp.Diagnostics.AddError(
			"Missing value_wo",
			"value_wo_version is set but value_wo is not, so there is no value to rotate to. Set value_wo, "+
				"or remove value_wo_version to leave the secret's value alone.",
		)
	}
}

func (r *IncidentSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.SecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The value lives only in the config: a write-only attribute is null in the plan.
	value, ok := r.valueFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !ok {
		resp.Diagnostics.AddError(
			"Missing value_wo",
			"A secret is created with its initial value, so value_wo must be set to create one. You can "+
				"remove it afterwards to have Terraform manage the secret's metadata without owning its value.",
		)
		return
	}

	result, err := r.client.SecretsV2CreateWithResponse(ctx, data.ToCreatePayload(value))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create secret '%s', got error: %s", data.Name.ValueString(), err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create secret: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON201.Secret.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeSecret, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("created a secret with id=%s", result.JSON201.Secret.Id))

	state := models.SecretModel{}.FromAPI(result.JSON201.Secret, data.ValueWOVersion)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.SecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SecretsV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Secret with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read secret, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read secret: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	// value_wo_version is the practitioner's own counter, so it carries over from the prior
	// state: a refresh has nothing to say about it.
	state := models.SecretModel{}.FromAPI(result.JSON200.Secret, data.ValueWOVersion)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.SecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state models.SecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.updateMetadata(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update secret, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, secret.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeSecret, r.terraformVersion)

	// The value itself is invisible to Terraform, so a change to value_wo_version is the
	// only thing that can ask for a rotation. Dropping it is not such a change: a config
	// that removes both value attributes is handing the value back rather than asking for
	// a new one, so only a version that is still set can rotate.
	if !plan.ValueWOVersion.IsNull() && !plan.ValueWOVersion.Equal(state.ValueWOVersion) {
		value, ok := r.valueFromConfig(ctx, req.Config, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if !ok {
			resp.Diagnostics.AddError(
				"Missing value_wo",
				"value_wo_version changed, which asks for the secret to be rotated, but value_wo is not set so "+
					"there is no value to rotate to.",
			)
			return
		}

		// Rotating after the metadata update means this response is the fresher of the two,
		// so it's the one that becomes state.
		secret, err = r.rotate(ctx, plan.ID.ValueString(), value)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to rotate secret, got error: %s", err))
			return
		}

		tflog.Trace(ctx, fmt.Sprintf("rotated secret with id=%s to version %d", secret.Id, secret.Version))
	}

	newState := models.SecretModel{}.FromAPI(*secret, plan.ValueWOVersion)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *IncidentSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.SecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.SecretsV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		// incident.io refuses to delete a secret that something still references, and names
		// what: that message is the useful part of the failure, so let it through.
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete secret, got error: %s", err))
		return
	}
}

func (r *IncidentSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeSecret, r.terraformVersion,
		r.markImportedAsManaged)

	result, err := r.client.SecretsV2ShowWithResponse(ctx, req.ID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError(
				"Secret Not Found",
				fmt.Sprintf("No secret with ID %q exists.", req.ID),
			)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read secret, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read secret: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	// An imported secret has no value_wo_version, because the value it holds was not set
	// from this configuration and there's no way to learn what it is. A config that sets
	// value_wo and value_wo_version therefore plans a rotation on the first apply, which
	// the plan says plainly; omit both to adopt the secret without touching its value.
	data := models.SecretModel{}.FromAPI(result.JSON200.Secret, types.Int64Null())
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// updateMetadata applies a secret's name, description and owning teams. It's always sent
// on an update: incident.io reads an omitted field as "leave unchanged", so a no-op update
// is cheaper than working out whether anything in particular moved.
func (r *IncidentSecretResource) updateMetadata(ctx context.Context, plan models.SecretModel) (*client.SecretV2, error) {
	result, err := r.client.SecretsV2UpdateWithResponse(ctx, plan.ID.ValueString(), plan.ToUpdatePayload())
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON200.Secret, nil
}

// rotate replaces the secret's value, advancing the version incident.io reports.
func (r *IncidentSecretResource) rotate(ctx context.Context, id string, value string) (*client.SecretV2, error) {
	result, err := r.client.SecretsV2RotateWithResponse(ctx, id, client.SecretsRotatePayloadV2{
		Value: value,
	})
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON200.Secret, nil
}

// valueFromConfig reads value_wo out of the config, which is the only place a write-only
// attribute holds anything: it is null in both the plan and the state. It reports false
// when the attribute isn't set, which is a different thing from a failure to read it.
func (r *IncidentSecretResource) valueFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) (string, bool) {
	var value types.String
	diags.Append(config.GetAttribute(ctx, path.Root("value_wo"), &value)...)
	if diags.HasError() {
		return "", false
	}

	if value.IsNull() || value.IsUnknown() {
		return "", false
	}

	return value.ValueString(), true
}
