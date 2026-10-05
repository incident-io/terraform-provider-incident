package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = &IncidentStatusPageComponentResource{}
	_ resource.ResourceWithConfigure   = &IncidentStatusPageComponentResource{}
	_ resource.ResourceWithImportState = &IncidentStatusPageComponentResource{}
)

type IncidentStatusPageComponentResource struct {
	resourceConfigurer
}

func NewIncidentStatusPageComponentResource() resource.Resource {
	return &IncidentStatusPageComponentResource{}
}

func (r *IncidentStatusPageComponentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_component"
}

func (r *IncidentStatusPageComponentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// The tag docstring points at structure endpoints the provider doesn't offer, so
		// this is the provider's own description.
		MarkdownDescription: "Manages a status page component: something a status page reports the status of, " +
			"such as a service or a region.\n\n" +
			"A component belongs to your organisation rather than to a page, and can appear on any number of " +
			"status pages. Each page's structure in the incident.io dashboard decides which pages show it, and " +
			"where. This resource creates and maintains the component; placing it on a page happens in the " +
			"dashboard. Look a page up with the `incident_status_page` data source.\n\n" +
			"The API key needs the \"Configure status pages\" permission. Deleting a component archives it. " +
			"incident.io refuses to archive a component while a page's structure still places it, or while an " +
			"incident, maintenance window or subscription still refers to it. Remove it from those first.",
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardAttribute(),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("StatusPageComponentV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("StatusPageComponentV2", "name"),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("StatusPageComponentV2", "description"),
				Validators: []validator.String{
					// A blank description reads back absent, which wouldn't match a config
					// saying "". Removing the attribute is how you clear one.
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

func (r *IncidentStatusPageComponentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.StatusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.StatusPageComponentsV2CreateWithResponse(ctx, data.ToCreatePayload())
	if err == nil && result.JSON201 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create status page component '%s', got error: %s", data.Name.ValueString(), err))
		return
	}

	if shouldClaim(data.UnlockInDashboard) {
		claimResource(ctx, r.client, result.JSON201.StatusPageComponent.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageComponent, r.terraformVersion)
	}

	tflog.Trace(ctx, fmt.Sprintf("created a status page component with id=%s", result.JSON201.StatusPageComponent.Id))

	state := models.StatusPageComponentModel{}.FromAPI(result.JSON201.StatusPageComponent, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentStatusPageComponentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.StatusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	component, err := r.show(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Status page component with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read status page component, got error: %s", err))
		return
	}

	state := models.StatusPageComponentModel{}.FromAPI(*component, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentStatusPageComponentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.StatusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.StatusPageComponentsV2UpdateWithResponse(ctx, data.ID.ValueString(), data.ToUpdatePayload())
	if err == nil && result.JSON200 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update status page component, got error: %s", err))
		return
	}

	if shouldClaim(data.UnlockInDashboard) {
		claimResource(ctx, r.client, result.JSON200.StatusPageComponent.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageComponent, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, result.JSON200.StatusPageComponent.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageComponent)
	}

	state := models.StatusPageComponentModel{}.FromAPI(result.JSON200.StatusPageComponent, data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentStatusPageComponentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.StatusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.StatusPageComponentsV2DestroyWithResponse(ctx, data.ID.ValueString()); err != nil {
		// incident.io refuses to archive a component something still references, and
		// names what, so let the message through.
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete status page component, got error: %s", err))
		return
	}
}

func (r *IncidentStatusPageComponentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageComponent, r.terraformVersion,
		r.markImportedAsManaged)

	component, err := r.show(ctx, req.ID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError("Status Page Component Not Found", fmt.Sprintf("No status page component with ID %q exists.", req.ID))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read status page component, got error: %s", err))
		return
	}

	state := models.StatusPageComponentModel{}.FromAPI(*component, types.BoolNull())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentStatusPageComponentResource) show(ctx context.Context, id string) (*client.StatusPageComponentV2, error) {
	result, err := r.client.StatusPageComponentsV2ShowWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON200.StatusPageComponent, nil
}
