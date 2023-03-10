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
	_ resource.Resource                = &IncidentCustomFieldResource{}
	_ resource.ResourceWithImportState = &IncidentCustomFieldResource{}
)

type IncidentCustomFieldResource struct {
	client *client.ClientWithResponses
}

type IncidentCustomFieldResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Description            types.String `tfsdk:"description"`
	FieldType              types.String `tfsdk:"field_type"`
	Required               types.String `tfsdk:"required"`
	ShowBeforeClosure      types.Bool   `tfsdk:"show_before_closure"`
	ShowBeforeCreation     types.Bool   `tfsdk:"show_before_creation"`
	ShowBeforeUpdate       types.Bool   `tfsdk:"show_before_update"`
	ShowInAnnouncementPost types.Bool   `tfsdk:"show_in_announcement_post"`
}

func NewIncidentCustomFieldResource() resource.Resource {
	return &IncidentCustomFieldResource{}
}

func (r *IncidentCustomFieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_field"
}

func (r *IncidentCustomFieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Custom Fields V1"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CustomFieldV1ResponseBody", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "description"),
				Required:            true,
			},
			"field_type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "field_type"),
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"required": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "required"),
				Required:            true,
			},
			"show_before_closure": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "show_before_closure"),
				Required:            true,
			},
			"show_before_creation": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "show_before_creation"),
				Required:            true,
			},
			"show_before_update": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "show_before_update"),
				Required:            true,
			},
			"show_in_announcement_post": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldsV1CreateRequestBody", "show_in_announcement_post"),
				Required:            true,
			},
		},
	}
}

func (r *IncidentCustomFieldResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *IncidentCustomFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCustomFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CustomFieldsV1CreateWithResponse(ctx, client.CustomFieldsV1CreateJSONRequestBody{
		Name:                   data.Name.ValueString(),
		Description:            data.Description.ValueString(),
		FieldType:              client.CreateRequestBody2FieldType(data.FieldType.ValueString()),
		Required:               client.CreateRequestBody2Required(data.Required.ValueString()),
		ShowBeforeClosure:      data.ShowBeforeClosure.ValueBool(),
		ShowBeforeCreation:     data.ShowBeforeCreation.ValueBool(),
		ShowBeforeUpdate:       data.ShowBeforeUpdate.ValueBool(),
		ShowInAnnouncementPost: lo.ToPtr(data.ShowInAnnouncementPost.ValueBool()),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create custom field, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a custom field resource with id=%s", result.JSON201.CustomField.Id))
	data = r.buildModel(result.JSON201.CustomField)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCustomFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCustomFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CustomFieldsV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read custom field, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CustomField)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCustomFieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCustomFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CustomFieldsV1UpdateWithResponse(ctx, data.ID.ValueString(), client.CustomFieldsV1UpdateJSONRequestBody{
		Name:                   data.Name.ValueString(),
		Description:            data.Description.ValueString(),
		Required:               client.UpdateRequestBody2Required(data.Required.ValueString()),
		ShowBeforeClosure:      data.ShowBeforeClosure.ValueBool(),
		ShowBeforeCreation:     data.ShowBeforeCreation.ValueBool(),
		ShowBeforeUpdate:       data.ShowBeforeUpdate.ValueBool(),
		ShowInAnnouncementPost: lo.ToPtr(data.ShowInAnnouncementPost.ValueBool()),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update custom field, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CustomField)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCustomFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCustomFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CustomFieldsV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete custom field, got error: %s", err))
		return
	}
}

func (r *IncidentCustomFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentCustomFieldResource) buildModel(cf client.CustomFieldV1) *IncidentCustomFieldResourceModel {
	var showInAnnouncementPost bool
	if cf.ShowInAnnouncementPost != nil {
		showInAnnouncementPost = *cf.ShowInAnnouncementPost
	}

	return &IncidentCustomFieldResourceModel{
		ID:                     types.StringValue(cf.Id),
		Name:                   types.StringValue(cf.Name),
		Description:            types.StringValue(cf.Description),
		FieldType:              types.StringValue(string(cf.FieldType)),
		Required:               types.StringValue(string(cf.Required)),
		ShowBeforeClosure:      types.BoolValue(cf.ShowBeforeClosure),
		ShowBeforeCreation:     types.BoolValue(cf.ShowBeforeCreation),
		ShowBeforeUpdate:       types.BoolValue(cf.ShowBeforeUpdate),
		ShowInAnnouncementPost: types.BoolValue(showInAnnouncementPost),
	}
}
