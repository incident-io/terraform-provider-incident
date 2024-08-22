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

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentRoleResource{}
	_ resource.ResourceWithImportState = &IncidentRoleResource{}
)

type IncidentRoleResource struct {
	client *client.ClientWithResponses
}

type IncidentRoleResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Instructions types.String `tfsdk:"instructions"`
	Shortform    types.String `tfsdk:"shortform"`
}

func NewIncidentRoleResource() resource.Resource {
	return &IncidentRoleResource{}
}

func (r *IncidentRoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_role"
}

func (r *IncidentRoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Incident Roles V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "description"),
				Required:            true,
			},
			"instructions": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "instructions"),
				Required:            true,
			},
			"shortform": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "shortform"),
				Required:            true,
			},
		},
	}
}

func (r *IncidentRoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client.Client
}

func (r *IncidentRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentRolesV2CreateWithResponse(ctx, client.IncidentRolesV2CreateJSONRequestBody{
		Name:         data.Name.ValueString(),
		Description:  data.Description.ValueString(),
		Instructions: data.Instructions.ValueString(),
		Shortform:    data.Shortform.ValueString(),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create incident role, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an incident role resource with id=%s", result.JSON201.IncidentRole.Id))
	data = r.buildModel(result.JSON201.IncidentRole)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentRolesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident role, got error: %s", err))
		return
	}

	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident role, got status code: %d", result.StatusCode()))
		return
	}

	data = r.buildModel(result.JSON200.IncidentRole)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentRolesV2UpdateWithResponse(ctx, data.ID.ValueString(), client.IncidentRolesV2UpdateJSONRequestBody{
		Name:         data.Name.ValueString(),
		Description:  data.Description.ValueString(),
		Instructions: data.Instructions.ValueString(),
		Shortform:    data.Shortform.ValueString(),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update incident role, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.IncidentRole)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentRolesV2DeleteWithResponse(ctx, data.ID.ValueString())
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete incident role, got error: %s", err))
		return
	}
}

func (r *IncidentRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentRoleResource) buildModel(role client.IncidentRoleV2) *IncidentRoleResourceModel {
	return &IncidentRoleResourceModel{
		ID:           types.StringValue(role.Id),
		Name:         types.StringValue(role.Name),
		Description:  types.StringValue(role.Description),
		Instructions: types.StringValue(role.Instructions),
		Shortform:    types.StringValue(role.Shortform),
	}
}
