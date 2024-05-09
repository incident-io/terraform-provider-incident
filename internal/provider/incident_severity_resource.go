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
	_ resource.Resource                = &IncidentSeverityResource{}
	_ resource.ResourceWithImportState = &IncidentSeverityResource{}
)

type IncidentSeverityResource struct {
	client *client.ClientWithResponses
}

type IncidentSeverityResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Rank        types.Int64  `tfsdk:"rank"`
}

func NewIncidentSeverityResource() resource.Resource {
	return &IncidentSeverityResource{}
}

func (r *IncidentSeverityResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_severity"
}

func (r *IncidentSeverityResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Severities V1"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("SeverityV1ResponseBody", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SeveritiesV1CreateRequestBody", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SeveritiesV1CreateRequestBody", "description"),
				Required:            true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("SeveritiesV1CreateRequestBody", "rank"),
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *IncidentSeverityResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentSeverityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentSeverityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var rank *int64
	if !data.Rank.IsUnknown() {
		rank = lo.ToPtr(data.Rank.ValueInt64())
	}
	result, err := r.client.SeveritiesV1CreateWithResponse(ctx, client.SeveritiesV1CreateJSONRequestBody{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Rank:        rank,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create incident severity, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an incident severity resource with id=%s", result.JSON201.Severity.Id))
	data = r.buildModel(result.JSON201.Severity)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentSeverityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentSeverityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SeveritiesV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident severity, got error: %s", err))
		return
	}

	if result.StatusCode() == 404 {
		resp.Diagnostics.AddWarning("Not Found", fmt.Sprintf("Unable to read incident severity, got status code: %d", result.StatusCode()))
		resp.State.RemoveResource(ctx)
		return
	}

	data = r.buildModel(result.JSON200.Severity)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentSeverityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentSeverityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rank *int64
	if !data.Rank.IsNull() {
		rank = lo.ToPtr(data.Rank.ValueInt64())
	}
	result, err := r.client.SeveritiesV1UpdateWithResponse(ctx, data.ID.ValueString(), client.SeveritiesV1UpdateJSONRequestBody{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Rank:        rank,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update incident severity, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.Severity)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentSeverityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentSeverityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.SeveritiesV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete incident severity, got error: %s", err))
		return
	}
}

func (r *IncidentSeverityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentSeverityResource) buildModel(severity client.SeverityV2) *IncidentSeverityResourceModel {
	return &IncidentSeverityResourceModel{
		ID:          types.StringValue(severity.Id),
		Name:        types.StringValue(severity.Name),
		Description: types.StringValue(severity.Description),
		Rank:        types.Int64Value(severity.Rank),
	}
}
