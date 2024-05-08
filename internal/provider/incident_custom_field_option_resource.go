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
	"github.com/samber/lo"
)

var (
	_ resource.Resource                = &IncidentCustomFieldOptionResource{}
	_ resource.ResourceWithImportState = &IncidentCustomFieldOptionResource{}
)

type IncidentCustomFieldOptionResource struct {
	client *client.ClientWithResponses
}

type IncidentCustomFieldOptionResourceModel struct {
	ID            types.String `tfsdk:"id"`
	CustomFieldID types.String `tfsdk:"custom_field_id"`
	SortKey       types.Int64  `tfsdk:"sort_key"`
	Value         types.String `tfsdk:"value"`
}

func NewIncidentCustomFieldOptionResource() resource.Resource {
	return &IncidentCustomFieldOptionResource{}
}

func (r *IncidentCustomFieldOptionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_field_option"
}

func (r *IncidentCustomFieldOptionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Custom Field Options V1"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CustomFieldOptionV1ResponseBody", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"custom_field_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldOptionsV1CreateRequestBody", "custom_field_id"),
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"sort_key": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("CustomFieldOptionsV1CreateRequestBody", "sort_key"),
				Optional:            true,
				Computed:            true,
			},
			"value": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldOptionsV1CreateRequestBody", "value"),
				Required:            true,
			},
		},
	}
}

func (r *IncidentCustomFieldOptionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentCustomFieldOptionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCustomFieldOptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var sortKey *int64
	if !data.SortKey.IsNull() {
		sortKey = lo.ToPtr(data.SortKey.ValueInt64())
	}
	result, err := r.client.CustomFieldOptionsV1CreateWithResponse(ctx, client.CustomFieldOptionsV1CreateJSONRequestBody{
		CustomFieldId: data.CustomFieldID.ValueString(),
		SortKey:       sortKey,
		Value:         data.Value.ValueString(),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create custom field option, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a custom field option resource with id=%s", result.JSON201.CustomFieldOption.Id))
	data = r.buildModel(result.JSON201.CustomFieldOption)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCustomFieldOptionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCustomFieldOptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CustomFieldOptionsV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read custom field option, got error: %s", err))
		return
	}

	if result.StatusCode() == 404 {
		resp.Diagnostics.AddWarning("Not Found", fmt.Sprintf("Unable to read custom field option, got status code: %d", result.StatusCode()))
		resp.State.RemoveResource(ctx)
		return
	}

	data = r.buildModel(result.JSON200.CustomFieldOption)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCustomFieldOptionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCustomFieldOptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CustomFieldOptionsV1UpdateWithResponse(ctx, data.ID.ValueString(), client.CustomFieldOptionsV1UpdateJSONRequestBody{
		SortKey: data.SortKey.ValueInt64(),
		Value:   data.Value.ValueString(),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update custom field, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CustomFieldOption)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCustomFieldOptionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCustomFieldOptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CustomFieldOptionsV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete custom field option, got error: %s", err))
		return
	}
}

func (r *IncidentCustomFieldOptionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentCustomFieldOptionResource) buildModel(option client.CustomFieldOptionV1) *IncidentCustomFieldOptionResourceModel {
	return &IncidentCustomFieldOptionResourceModel{
		ID:            types.StringValue(option.Id),
		CustomFieldID: types.StringValue(option.CustomFieldId),
		SortKey:       types.Int64Value(option.SortKey),
		Value:         types.StringValue(option.Value),
	}
}
