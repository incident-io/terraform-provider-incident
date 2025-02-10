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
	_ resource.Resource                = &IncidentAlertAttributeResource{}
	_ resource.ResourceWithImportState = &IncidentAlertAttributeResource{}
)

type IncidentAlertAttributeResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

type IncidentAlertAttributeResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Type  types.String `tfsdk:"type"`
	Array types.Bool   `tfsdk:"array"`
}

func NewIncidentAlertAttributeResource() resource.Resource {
	return &IncidentAlertAttributeResource{}
}

func (r *IncidentAlertAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_attribute"
}

func (r *IncidentAlertAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Alert attributes V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "name"),
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "type"),
				Required:            true,
			},
			"array": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "array"),
				Required:            true,
			},
		},
	}
}

func (r *IncidentAlertAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.terraformVersion = client.TerraformVersion
}

func (r *IncidentAlertAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestBody := client.AlertAttributesCreatePayloadV2{
		Name:  data.Name.ValueString(),
		Type:  data.Type.ValueString(),
		Array: data.Array.ValueBool(),
	}

	result, err := r.client.AlertAttributesV2CreateWithResponse(ctx, requestBody)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert attribute, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an alert attribute resource with id=%s", result.JSON201.AlertAttribute.Id))
	data = r.buildModel(result.JSON201.AlertAttribute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertAttributesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert attribute, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.AlertAttribute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertAttributeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestBody := client.AlertAttributesV2UpdateJSONRequestBody{
		Name:  data.Name.ValueString(),
		Type:  data.Type.ValueString(),
		Array: data.Array.ValueBool(),
	}

	result, err := r.client.AlertAttributesV2UpdateWithResponse(ctx, data.ID.ValueString(), requestBody)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert attribute, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.AlertAttribute)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertAttributeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.AlertAttributesV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert attribute, got error: %s", err))
		return
	}
}

func (r *IncidentAlertAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentAlertAttributeResource) buildModel(alertAttribute client.AlertAttributeV2) *IncidentAlertAttributeResourceModel {
	model := &IncidentAlertAttributeResourceModel{
		ID:    types.StringValue(alertAttribute.Id),
		Name:  types.StringValue(alertAttribute.Name),
		Type:  types.StringValue(alertAttribute.Type),
		Array: types.BoolValue(alertAttribute.Array),
	}
	return model
}
