package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
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
	_ resource.Resource                = &IncidentAccessGrantResource{}
	_ resource.ResourceWithConfigure   = &IncidentAccessGrantResource{}
	_ resource.ResourceWithImportState = &IncidentAccessGrantResource{}
)

type IncidentAccessGrantResource struct {
	resourceConfigurer
}

func NewIncidentAccessGrantResource() resource.Resource {
	return &IncidentAccessGrantResource{}
}

func (r *IncidentAccessGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_grant"
}

func (r *IncidentAccessGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Access Grants V2"),
			"An access group holds one grant on the organization, so manage at most one "+
				"`incident_access_grant` for each group. Creating a grant for a group sets that group's "+
				"privileges, and adopts the grant the group already has.\n\n"+
				"You can manage the grants of the `admin` and `standard` base groups, but not of "+
				"`owner`. A base group always keeps some privileges, so destroying the grant of a base "+
				"group leaves its privileges as they are and only stops Terraform managing them.\n\n"+
				"Use the `incident_access_privileges` data source to see which privilege names you can grant."),
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardAttribute(),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AccessGrantV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"principal": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Who this grant gives privileges to. Changing it replaces the grant.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: EnumValuesDescription("AccessGrantPrincipalV2", "type"),
						Validators: []validator.String{
							stringvalidator.OneOf(enumValues("AccessGrantPrincipalV2", "type")...),
						},
					},
					"id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AccessGrantPrincipalV2", "id"),
					},
				},
			},
			"resource": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "What this grant gives privileges on. Changing it replaces the grant.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: EnumValuesDescription("AccessGrantResourceV2", "type"),
						Validators: []validator.String{
							stringvalidator.OneOf(enumValues("AccessGrantResourceV2", "type")...),
						},
					},
				},
			},
			"privileges": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				MarkdownDescription: apischema.Docstring("AccessGrantV2", "privileges") +
					" A grant must give at least one privilege.",
				Validators: []validator.Set{
					// A grant with no privileges doesn't exist as far as the API is concerned:
					// it returns 404 for one, so an empty set could never read back.
					setvalidator.SizeAtLeast(1),
				},
			},
		},
	}
}

func (r *IncidentAccessGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AccessGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AccessGrantsV2CreateWithResponse(ctx, data.ToCreatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create access grant, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create access grant: unexpected response from API (status %s)", result.Status()))
		return
	}
	grant := result.JSON200.AccessGrant

	if shouldClaim(data.UnlockInDashboard) {
		claimResource(ctx, r.client, grant.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGrant, r.terraformVersion)
	}

	tflog.Trace(ctx, fmt.Sprintf("created an access grant with id=%s", grant.Id))

	state := models.AccessGrantModel{}.FromAPI(grant, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAccessGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AccessGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API answers 404 for a grant whose privileges were all removed, as well as for one
	// whose group was deleted, so either way the next apply creates it again.
	grant, notFound, err := r.show(ctx, data.ID.ValueString())
	if notFound {
		tflog.Warn(ctx, fmt.Sprintf("Access grant with ID %s not found: removing from state.", data.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access grant, got error: %s", err))
		return
	}

	state := models.AccessGrantModel{}.FromAPI(*grant, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAccessGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AccessGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Editing the privileges keeps the grant's ID.
	result, err := r.client.AccessGrantsV2UpdateWithResponse(ctx, data.ID.ValueString(), data.ToUpdatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update access grant, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update access grant: unexpected response from API (status %s)", result.Status()))
		return
	}
	grant := result.JSON200.AccessGrant

	if shouldClaim(data.UnlockInDashboard) {
		claimResource(ctx, r.client, grant.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGrant, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, grant.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGrant)
	}

	state := models.AccessGrantModel{}.FromAPI(grant, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAccessGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AccessGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A base group must keep its privileges, so the API refuses to destroy its grant. Hand
	// the grant back to the dashboard instead, which is what removing it from Terraform
	// means for a grant that cannot stop existing.
	if data.Principal != nil && data.Principal.Type.ValueString() == string(client.AccessGrantPrincipalV2TypeAccessGroup) {
		isBase, err := r.principalIsBaseGroup(ctx, data.Principal.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read the access group this grant belongs to, got error: %s", err))
			return
		}
		if isBase {
			unclaimResource(ctx, r.client, data.ID.ValueString(), &resp.Diagnostics,
				client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGrant)
			resp.Diagnostics.AddWarning(
				"Base group privileges left in place",
				fmt.Sprintf("Access grant %s belongs to a base access group, whose privileges cannot be removed. "+
					"It is no longer managed by Terraform, and keeps its current privileges.", data.ID.ValueString()),
			)
			return
		}
	}

	_, err := r.client.AccessGrantsV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete access grant, got error: %s", err))
		return
	}
}

func (r *IncidentAccessGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	grant, notFound, err := r.show(ctx, req.ID)
	if notFound {
		resp.Diagnostics.AddError("Access Grant Not Found", fmt.Sprintf("No access grant with ID %q exists.", req.ID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access grant, got error: %s", err))
		return
	}

	claimResourceOnImport(ctx, r.client, grant.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGrant, r.terraformVersion,
		r.markImportedAsManaged)

	state := models.AccessGrantModel{}.FromAPI(*grant, types.BoolNull())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// show fetches an access grant, and reports separately whether it no longer exists.
func (r *IncidentAccessGrantResource) show(ctx context.Context, id string) (*client.AccessGrantV2, bool, error) {
	result, err := r.client.AccessGrantsV2ShowWithResponse(ctx, id)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return nil, true, nil
		}
		return nil, false, err
	}
	if result.JSON200 == nil {
		return nil, false, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON200.AccessGrant, false, nil
}

// principalIsBaseGroup reports whether the access group a grant belongs to is a base group.
// A group that no longer exists is not one: its grant went with it, and destroying it
// finds nothing to do.
func (r *IncidentAccessGrantResource) principalIsBaseGroup(ctx context.Context, groupID string) (bool, error) {
	result, err := r.client.AccessGroupsV2ShowWithResponse(ctx, groupID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	if result.JSON200 == nil {
		return false, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return result.JSON200.AccessGroup.Type == client.AccessGroupV2TypeBase, nil
}
