package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentCatalogTypeResource{}
	_ resource.ResourceWithImportState = &IncidentCatalogTypeResource{}
)

type IncidentCatalogTypeResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

type IncidentCatalogTypeResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	TypeName      types.String `tfsdk:"type_name"`
	Description   types.String `tfsdk:"description"`
	SourceRepoURL types.String `tfsdk:"source_repo_url"`
	Categories    types.List   `tfsdk:"categories"`
}

func NewIncidentCatalogTypeResource() resource.Resource {
	return &IncidentCatalogTypeResource{}
}

func (r *IncidentCatalogTypeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_type"
}

func (r IncidentCatalogTypeResource) CategoryDescription() string {
	// Make a category description where we list all the possible values of categories
	categories := []string{}
	for _, category := range apischema.Property("CatalogTypeV3", "categories").Value.Items.Value.Enum {
		categoryAsString, _ := category.(string)
		categories = append(categories, "`"+categoryAsString+"`")
	}

	return fmt.Sprintf("The categories that this type belongs to, to be shown in the web dashboard. Possible values are: %s.", strings.Join(categories, ", "))
}

func (r *IncidentCatalogTypeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Catalog V3"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CatalogTypeV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogTypeV3", "name"),
				Required:            true,
			},
			"type_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true, // If not provided, we'll use the generated ID
				MarkdownDescription: apischema.Docstring("CatalogTypeV3", "type_name"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogTypeV3", "description"),
				Required:            true,
			},
			"categories": schema.ListAttribute{
				MarkdownDescription: r.CategoryDescription(),
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"source_repo_url": schema.StringAttribute{
				MarkdownDescription: "The url of the external repository where this type is managed. When set, users will not be able to edit the catalog type (or its entries) via the UI, and will instead be provided a link to this URL.",
				Required:            true,
			},
		},
	}
}

func (r *IncidentCatalogTypeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentCatalogTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestBody := client.CatalogCreateTypePayloadV3{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	}
	if typeName := data.TypeName.ValueString(); typeName != "" {
		requestBody.TypeName = &typeName
	}
	if sourceRepoURL := data.SourceRepoURL.ValueString(); sourceRepoURL != "" {
		requestBody.SourceRepoUrl = &sourceRepoURL
	}

	categories := []client.CatalogCreateTypePayloadV3Categories{}
	if !data.Categories.IsNull() {
		for _, elem := range data.Categories.Elements() {
			category, ok := elem.(types.String)
			if !ok {
				continue
			}
			categories = append(categories,
				client.CatalogCreateTypePayloadV3Categories(category.ValueString()))
		}
	}
	requestBody.Categories = lo.ToPtr(categories)

	result, err := r.client.CatalogV3CreateTypeWithResponse(ctx, requestBody)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create catalog type, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a catalog type resource with id=%s", result.JSON201.CatalogType.Id))
	data = r.buildModel(result.JSON201.CatalogType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV3ShowTypeWithResponse(ctx, data.ID.ValueString())
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog type, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CatalogType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCatalogTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestBody := client.CatalogV3UpdateTypeJSONRequestBody{
		Name: data.Name.ValueString(),
		// TypeName cannot be changed once set
		Description: data.Description.ValueString(),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	}

	if sourceRepoURL := data.SourceRepoURL.ValueString(); sourceRepoURL != "" {
		requestBody.SourceRepoUrl = &sourceRepoURL
	}

	categories := []client.CatalogUpdateTypePayloadV3Categories{}
	if !data.Categories.IsNull() {
		for _, elem := range data.Categories.Elements() {
			category, ok := elem.(types.String)
			if !ok {
				continue
			}
			categories = append(categories,
				client.CatalogUpdateTypePayloadV3Categories(category.ValueString()))
		}
	}
	requestBody.Categories = lo.ToPtr(categories)

	result, err := r.client.CatalogV3UpdateTypeWithResponse(ctx, data.ID.ValueString(), requestBody)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update catalog type, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CatalogType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CatalogV3DestroyTypeWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete catalog type, got error: %s", err))
		return
	}
}

func (r *IncidentCatalogTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentCatalogTypeResource) buildModel(catalogType client.CatalogTypeV3) *IncidentCatalogTypeResourceModel {
	model := &IncidentCatalogTypeResourceModel{
		ID:          types.StringValue(catalogType.Id),
		Name:        types.StringValue(catalogType.Name),
		TypeName:    types.StringValue(catalogType.TypeName),
		Description: types.StringValue(catalogType.Description),
	}
	if catalogType.SourceRepoUrl != nil {
		model.SourceRepoURL = types.StringValue(*catalogType.SourceRepoUrl)
	}

	categories := []attr.Value{}
	for _, category := range catalogType.Categories {
		categories = append(categories, types.StringValue(string(category)))
	}
	model.Categories = types.ListValueMust(types.StringType, categories)

	return model
}
