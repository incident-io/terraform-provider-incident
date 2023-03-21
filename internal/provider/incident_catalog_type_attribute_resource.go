package provider

import (
	"context"
	"fmt"

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
	_ resource.Resource = &IncidentCatalogTypeAttributeResource{}
)

type IncidentCatalogTypeAttributeResource struct {
	client *client.ClientWithResponses
}

type CatalogTypeAttribute struct {
	Name  types.String `tfsdk:"name"`
	Type  types.String `tfsdk:"type"`
	Array types.Bool   `tfsdk:"array"`
}

type IncidentCatalogTypeAttributesResourceModel struct {
	ID            types.String `tfsdk:"id"`
	CatalogTypeID types.String `tfsdk:"catalog_type_id"`
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Array         types.Bool   `tfsdk:"array"`
}

func (m IncidentCatalogTypeAttributesResourceModel) buildAttribute() client.CatalogTypeAttributePayloadV2 {
	var array bool
	if m.Array.IsUnknown() {
		array = false
	} else {
		array = m.Array.ValueBool()
	}

	var id *string
	if m.ID.IsUnknown() {
		id = nil
	} else {
		id = lo.ToPtr(m.ID.ValueString())
	}
	return client.CatalogTypeAttributePayloadV2{
		Id:    id,
		Name:  m.Name.ValueString(),
		Type:  m.Type.ValueString(),
		Array: array,
	}
}

func NewIncidentCatalogTypeAttributesResource() resource.Resource {
	return &IncidentCatalogTypeAttributeResource{}
}

func (r *IncidentCatalogTypeAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_type_attribute"
}

func (r *IncidentCatalogTypeAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Catalog V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"catalog_type_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("CatalogTypeV2ResponseBody", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
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
				Computed:    true,
			},
		},
	}
}

func (r *IncidentCatalogTypeAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentCatalogTypeAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	typeResult, err := r.client.CatalogV2ShowTypeWithResponse(ctx, data.CatalogTypeID.ValueString())
	if err == nil && typeResult.StatusCode() >= 400 {
		err = fmt.Errorf(string(typeResult.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get catalog type, got error: %s", err))
		return
	}

	attributes := []client.CatalogTypeAttributePayloadV2{}
	for _, attribute := range typeResult.JSON200.CatalogType.Schema.Attributes {
		attributes = append(attributes, client.CatalogTypeAttributePayloadV2{
			Id:    &attribute.Id,
			Name:  attribute.Name,
			Type:  attribute.Type,
			Array: attribute.Array,
		})
	}

	// Add our new attribute.
	attributes = append(attributes, data.buildAttribute())

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

	var attributeID string
	for _, attribute := range result.JSON200.CatalogType.Schema.Attributes {
		if attribute.Name == data.buildAttribute().Name {
			attributeID = attribute.Id
		}
	}
	if attributeID == "" {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find attribute in catalog type schema"))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Updated catalog type schema for id=%s", result.JSON200.CatalogType.Id))
	data = r.buildModel(result.JSON200.CatalogType, attributeID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV2ShowTypeWithResponse(ctx, data.CatalogTypeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog type, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CatalogType, data.ID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeAttributeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	typeResult, err := r.client.CatalogV2ShowTypeWithResponse(ctx, data.CatalogTypeID.ValueString())
	if err == nil && typeResult.StatusCode() >= 400 {
		err = fmt.Errorf(string(typeResult.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get catalog type, got error: %s", err))
		return
	}

	var (
		attributes    = []client.CatalogTypeAttributePayloadV2{}
		alreadyExists bool
	)
	for _, attribute := range typeResult.JSON200.CatalogType.Schema.Attributes {
		if attribute.Id == data.ID.ValueString() {
			alreadyExists = true
			attributes = append(attributes, data.buildAttribute())
		} else {
			attributes = append(attributes, client.CatalogTypeAttributePayloadV2{
				Id:    &attribute.Id,
				Name:  attribute.Name,
				Type:  attribute.Type,
				Array: attribute.Array,
			})
		}
	}
	if !alreadyExists {
		// We weren't here, so add us to the end.
		attributes = append(attributes, data.buildAttribute())
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

	var attributeID string
	if alreadyExists {
		attributeID = data.ID.ValueString()
	} else {
		for _, attribute := range result.JSON200.CatalogType.Schema.Attributes {
			if attribute.Name == data.buildAttribute().Name {
				attributeID = attribute.Id
			}
		}
		if attributeID == "" {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find attribute in catalog type schema"))
			return
		}
	}

	tflog.Trace(ctx, fmt.Sprintf("Updated catalog type schema for catalog type with id=%s", result.JSON200.CatalogType.Id))
	data = r.buildModel(result.JSON200.CatalogType, attributeID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogTypeAttributeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	typeResult, err := r.client.CatalogV2ShowTypeWithResponse(ctx, data.CatalogTypeID.ValueString())
	if err == nil && typeResult.StatusCode() >= 400 {
		err = fmt.Errorf(string(typeResult.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get catalog type, got error: %s", err))
		return
	}

	attributes := []client.CatalogTypeAttributePayloadV2{}
	for _, attribute := range typeResult.JSON200.CatalogType.Schema.Attributes {
		if attribute.Id == data.ID.ValueString() {
			continue
		}

		attributes = append(attributes, client.CatalogTypeAttributePayloadV2{
			Id:    &attribute.Id,
			Name:  attribute.Name,
			Type:  attribute.Type,
			Array: attribute.Array,
		})
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
}

func (r *IncidentCatalogTypeAttributeResource) buildModel(catalogType client.CatalogTypeV2, attributeID string) *IncidentCatalogTypeAttributesResourceModel {
	result := &IncidentCatalogTypeAttributesResourceModel{
		ID:            types.StringValue(attributeID),
		CatalogTypeID: types.StringValue(catalogType.Id),
	}

	for _, attribute := range catalogType.Schema.Attributes {
		if attribute.Id == attributeID {
			result.Name = types.StringValue(attribute.Name)
			result.Type = types.StringValue(attribute.Type)
			result.Array = types.BoolValue(attribute.Array)
		}
	}

	return result
}
