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
	_ resource.Resource                = &IncidentCatalogTypeAttributesResource{}
	_ resource.ResourceWithImportState = &IncidentCatalogTypeAttributesResource{}
)

type IncidentCatalogTypeAttributesResource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogTypeAttributesResourceModel struct {
	CatalogTypeID types.String `tfsdk:"catalog_type_id"`
	Attribute     types.List   `tfsdk:"attribute"`
}

func NewIncidentCatalogTypeAttributesResource() resource.Resource {
	return &IncidentCatalogTypeAttributesResource{}
}

func (r *IncidentCatalogTypeAttributesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_type_attributes"
}

func (r *IncidentCatalogTypeAttributesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Catalog V2"),
		Attributes: map[string]schema.Attribute{
			"catalog_type_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CatalogTypeV2ResponseBody", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"attribute": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: `The name of this attribute.`,
							Required:    true,
						},
						"type": schema.StringAttribute{
							Description: `The type of this attribute.`,
							Required:    true,
						},
						"array": schema.BoolAttribute{
							Description: `Whether this attribute is an array or scalar.`,
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (r *IncidentCatalogTypeAttributesResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentCatalogTypeAttributesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	attributes := []client.CatalogTypeAttributeV2{}
	for _, elem := range data.Attribute.Elements() {
		//
	}
	result, err := r.client.CatalogV2UpdateTypeSchemaWithResponse(ctx, data.CatalogTypeID.ValueString(), client.UpdateTypeSchemaRequestBody{
		Attributes: attributes,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update catalog type schema, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a catalog type resource with id=%s", result.JSON201.CatalogType.Id))
	data = r.buildModel(result.JSON201.CatalogType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeAttributesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV2ShowTypeWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog type, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CatalogType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeAttributesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var semanticType string
	if data.SemanticType.IsUnknown() {
		semanticType = "custom"
	} else {
		semanticType = data.SemanticType.ValueString()
	}
	result, err := r.client.CatalogV2UpdateTypeWithResponse(ctx, data.ID.ValueString(), client.CatalogV2UpdateTypeJSONRequestBody{
		Name:         data.Name.ValueString(),
		Description:  data.Description.ValueString(),
		SemanticType: semanticType,
	})
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

func (r *IncidentCatalogTypeAttributesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CatalogV2DestroyTypeWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete catalog type, got error: %s", err))
		return
	}
}

func (r *IncidentCatalogTypeAttributesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentCatalogTypeAttributesResource) buildModel(catalogType client.CatalogTypeV2) *IncidentCatalogTypeAttributesResourceModel {
	return &IncidentCatalogTypeAttributesResourceModel{
		ID:           types.StringValue(catalogType.Id),
		Name:         types.StringValue(catalogType.Name),
		Description:  types.StringValue(catalogType.Description),
		SemanticType: types.StringValue(catalogType.SemanticType),
	}
}
