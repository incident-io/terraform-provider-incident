package provider

import (
	"context"
	"errors"
	"fmt"

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
	_ resource.Resource                = &IncidentAccessGroupResource{}
	_ resource.ResourceWithConfigure   = &IncidentAccessGroupResource{}
	_ resource.ResourceWithImportState = &IncidentAccessGroupResource{}
)

type IncidentAccessGroupResource struct {
	resourceConfigurer
}

func NewIncidentAccessGroupResource() resource.Resource {
	return &IncidentAccessGroupResource{}
}

func (r *IncidentAccessGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_group"
}

func (r *IncidentAccessGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Access Groups V2"),
			"This resource manages custom access groups. Who belongs to a group is not managed here: "+
				"add people in the dashboard, or map the group to a group in your identity provider "+
				"with SCIM. Give a group privileges with `incident_access_grant`.\n\n"+
				"Every organization also has the base groups `owner`, `admin` and `standard`, which "+
				"cannot be renamed or deleted. Look those up with the `incident_access_group` data "+
				"source instead of managing them here."),
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardAttribute(),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "name"),
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "description"),
			},
			"slug": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "slug") +
					" Changing it replaces the group.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "type"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *IncidentAccessGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AccessGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AccessGroupsV2CreateWithResponse(ctx, data.ToCreatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create access group '%s', got error: %s", data.Name.ValueString(), err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create access group: unexpected response from API (status %s)", result.Status()))
		return
	}
	group := result.JSON201.AccessGroup

	if shouldClaim(data.UnlockInDashboard) {
		claimResource(ctx, r.client, group.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGroup, r.terraformVersion)
	}

	tflog.Trace(ctx, fmt.Sprintf("created an access group with id=%s", group.Id))

	state := models.AccessGroupModel{}.FromAPI(group, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAccessGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AccessGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, notFound, err := r.show(ctx, data.ID.ValueString())
	if notFound {
		tflog.Warn(ctx, fmt.Sprintf("Access group with ID %s not found: removing from state.", data.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access group, got error: %s", err))
		return
	}

	state := models.AccessGroupModel{}.FromAPI(*group, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAccessGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AccessGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AccessGroupsV2UpdateWithResponse(ctx, data.ID.ValueString(), data.ToUpdatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update access group, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update access group: unexpected response from API (status %s)", result.Status()))
		return
	}
	group := result.JSON200.AccessGroup

	if shouldClaim(data.UnlockInDashboard) {
		claimResource(ctx, r.client, group.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGroup, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, group.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGroup)
	}

	state := models.AccessGroupModel{}.FromAPI(group, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAccessGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AccessGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.AccessGroupsV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete access group, got error: %s", err))
		return
	}
}

func (r *IncidentAccessGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	group, notFound, err := r.show(ctx, req.ID)
	if notFound {
		resp.Diagnostics.AddError("Access Group Not Found", fmt.Sprintf("No access group with ID %q exists.", req.ID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access group, got error: %s", err))
		return
	}

	claimResourceOnImport(ctx, r.client, group.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAccessGroup, r.terraformVersion,
		r.markImportedAsManaged)

	state := models.AccessGroupModel{}.FromAPI(*group, types.BoolNull())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// show fetches an access group, and reports separately whether it no longer exists.
func (r *IncidentAccessGroupResource) show(ctx context.Context, id string) (*client.AccessGroupV2, bool, error) {
	result, err := r.client.AccessGroupsV2ShowWithResponse(ctx, id)
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

	return &result.JSON200.AccessGroup, false, nil
}
