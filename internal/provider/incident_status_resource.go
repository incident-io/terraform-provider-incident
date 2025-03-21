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
	_ resource.Resource                = &IncidentStatusResource{}
	_ resource.ResourceWithImportState = &IncidentStatusResource{}
)

type IncidentStatusResource struct {
	client *client.ClientWithResponses
}

type IncidentStatusResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Category    types.String `tfsdk:"category"`
}

func NewIncidentStatusResource() resource.Resource {
	return &IncidentStatusResource{}
}

func (r *IncidentStatusResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status"
}

func (r *IncidentStatusResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Incident Statuses V1"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "description"),
				Required:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "category"),
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *IncidentStatusResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentStatusResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentStatusesV1CreateWithResponse(ctx, client.IncidentStatusesV1CreateJSONRequestBody{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Category:    client.IncidentStatusesCreatePayloadV1Category(data.Category.ValueString()),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create incident status, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an incident status resource with id=%s", result.JSON201.IncidentStatus.Id))
	data = r.buildModel(result.JSON201.IncidentStatus)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentStatusResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentStatusesV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident status, got error: %s", err))
		return
	}
	if result.StatusCode() == 404 {
		resp.Diagnostics.AddWarning("Not Found", fmt.Sprintf("Unable to read incident status, got status code: %d", result.StatusCode()))
		resp.State.RemoveResource(ctx)
		return
	}

	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident status, got status code: %d", result.StatusCode()))
		return
	}

	data = r.buildModel(result.JSON200.IncidentStatus)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentStatusResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentStatusesV1UpdateWithResponse(ctx, data.ID.ValueString(), client.IncidentStatusesV1UpdateJSONRequestBody{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update incident status, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.IncidentStatus)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentStatusResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.IncidentStatusesV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete incident status, got error: %s", err))
		return
	}
}

func (r *IncidentStatusResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentStatusResource) buildModel(status client.IncidentStatusV1) *IncidentStatusResourceModel {
	return &IncidentStatusResourceModel{
		ID:          types.StringValue(status.Id),
		Name:        types.StringValue(status.Name),
		Description: types.StringValue(status.Description),
		Category:    types.StringValue(string(status.Category)),
	}
}
