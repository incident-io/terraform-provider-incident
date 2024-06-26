package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

var (
	_ resource.Resource = &IncidentCatalogTypeAttributeResource{}
)

type IncidentCatalogTypeAttributeResource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogTypeAttributesResourceModel struct {
	ID                types.String `tfsdk:"id"`
	CatalogTypeID     types.String `tfsdk:"catalog_type_id"`
	Name              types.String `tfsdk:"name"`
	Type              types.String `tfsdk:"type"`
	Array             types.Bool   `tfsdk:"array"`
	BacklinkAttribute types.String `tfsdk:"backlink_attribute"`
	Path              types.List   `tfsdk:"path"`
}

func (m IncidentCatalogTypeAttributesResourceModel) buildAttribute(ctx context.Context) client.CatalogTypeAttributePayloadV2 {
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

	var (
		mode              *client.CatalogTypeAttributePayloadV2Mode
		backlinkAttribute *string
		path              *[]client.CatalogTypeAttributePathItemPayloadV2
	)
	if !m.BacklinkAttribute.IsNull() {
		backlinkAttribute = lo.ToPtr(m.BacklinkAttribute.ValueString())
		mode = lo.ToPtr(client.CatalogTypeAttributePayloadV2ModeBacklink)
	}
	if !m.Path.IsUnknown() && !m.Path.IsNull() {
		mode = lo.ToPtr(client.CatalogTypeAttributePayloadV2ModePath)

		// Do a little dance to get the path into the right format.
		pathAsStrings := []string{}
		if diags := m.Path.ElementsAs(ctx, &pathAsStrings, false); diags.HasError() {
			panic(spew.Sdump(diags.Errors()))
		}

		path = &[]client.CatalogTypeAttributePathItemPayloadV2{}
		for _, item := range pathAsStrings {
			*path = append(*path, client.CatalogTypeAttributePathItemPayloadV2{
				AttributeId: item,
			})
		}
	}

	return client.CatalogTypeAttributePayloadV2{
		Id:                id,
		Name:              m.Name.ValueString(),
		Type:              m.Type.ValueString(),
		Array:             array,
		Mode:              mode,
		BacklinkAttribute: backlinkAttribute,
		Path:              path,
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
			"backlink_attribute": schema.StringAttribute{
				Description: `If this is a backlink, the id of the attribute that it's linked from`,
				Optional:    true,
			},
			"path": schema.ListAttribute{
				Description: `If this is a path attribute, the path that we should use to pull the data`,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				Optional: true,
			},
		},
	}
}

func (r *IncidentCatalogTypeAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentCatalogTypeAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogTypeAttributesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result *client.CatalogV2UpdateTypeSchemaResponse
	err := r.lockFor(ctx, data.CatalogTypeID.ValueString(), func(ctx context.Context, catalogType client.CatalogTypeV2) error {
		attributes := []client.CatalogTypeAttributePayloadV2{}
		for _, attribute := range catalogType.Schema.Attributes {
			attributes = append(attributes, r.attributeToPayload(attribute))
		}

		// Add our new attribute.
		attributes = append(attributes, data.buildAttribute(ctx))

		var err error
		result, err = r.client.CatalogV2UpdateTypeSchemaWithResponse(ctx, catalogType.Id, client.UpdateTypeSchemaRequestBody{
			Version:    catalogType.Schema.Version,
			Attributes: attributes,
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf(string(result.Body))
		}
		if err != nil {
			return errors.Wrap(err, "Unable to update catalog type schema, got error")
		}

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

	var attributeID string
	for _, attribute := range result.JSON200.CatalogType.Schema.Attributes {
		if attribute.Name == data.buildAttribute(ctx).Name {
			attributeID = attribute.Id
		}
	}
	if attributeID == "" {
		resp.Diagnostics.AddError("Client Error", "Unable to find attribute in catalog type schema")
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
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
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

	var (
		alreadyExists bool
	)

	var result *client.CatalogV2UpdateTypeSchemaResponse
	err := r.lockFor(ctx, data.CatalogTypeID.ValueString(), func(ctx context.Context, catalogType client.CatalogTypeV2) error {
		var (
			attributes = []client.CatalogTypeAttributePayloadV2{}
		)
		tflog.Trace(ctx, fmt.Sprintf("Looking for attribute with id=%s", data.ID.ValueString()))
		for _, attribute := range catalogType.Schema.Attributes {
			if attribute.Id == data.ID.ValueString() {
				alreadyExists = true
				attributes = append(attributes, data.buildAttribute(ctx))
			} else {
				attributes = append(attributes, r.attributeToPayload(attribute))
			}
		}

		if !alreadyExists {
			// We weren't here, so add us to the end.
			attributes = append(attributes, data.buildAttribute(ctx))
		}

		tflog.Trace(ctx, fmt.Sprintf("Updating catalog type with attributes: %v", attributes))
		var err error
		result, err = r.client.CatalogV2UpdateTypeSchemaWithResponse(ctx, data.CatalogTypeID.ValueString(), client.UpdateTypeSchemaRequestBody{
			Version:    catalogType.Schema.Version,
			Attributes: attributes,
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf(string(result.Body))
		}
		if err != nil {
			return errors.Wrap(err, "Unable to update catalog type schema, got error")
		}

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

	var attributeID string
	if alreadyExists {
		attributeID = data.ID.ValueString()
	} else {
		for _, attribute := range result.JSON200.CatalogType.Schema.Attributes {
			if attribute.Name == data.buildAttribute(ctx).Name {
				attributeID = attribute.Id
			}
		}
		if attributeID == "" {
			resp.Diagnostics.AddError("Client Error", "Unable to find attribute in catalog type schema")
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

	err := r.lockFor(ctx, data.CatalogTypeID.ValueString(), func(ctx context.Context, catalogType client.CatalogTypeV2) error {
		attributes := []client.CatalogTypeAttributePayloadV2{}
		for _, attribute := range catalogType.Schema.Attributes {
			if attribute.Id == data.ID.ValueString() {
				continue
			}

			attributes = append(attributes, r.attributeToPayload(attribute))
		}

		result, err := r.client.CatalogV2UpdateTypeSchemaWithResponse(ctx, data.CatalogTypeID.ValueString(), client.UpdateTypeSchemaRequestBody{
			Version:    catalogType.Schema.Version,
			Attributes: attributes,
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf(string(result.Body))
		}
		if err != nil {
			return errors.Wrap(err, "Unable to update catalog type schema, got error")
		}

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
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
			if attribute.BacklinkAttribute != nil {
				result.BacklinkAttribute = types.StringValue(*attribute.BacklinkAttribute)
			}

			result.Path = types.ListNull(types.StringType)
			if attribute.Path != nil {
				path := []attr.Value{}
				for _, item := range *attribute.Path {
					path = append(path, types.StringValue(item.AttributeId))
				}
				result.Path = types.ListValueMust(types.StringType, path)
			}
			break
		}
	}

	return result
}

var (
	catalogTypeLocks = map[string]*sync.Mutex{}
	catalogTypeMutex sync.Mutex
)

func (r *IncidentCatalogTypeAttributeResource) lockFor(ctx context.Context, catalogTypeID string, do func(ctx context.Context, catalogType client.CatalogTypeV2) error) error {
	catalogTypeMutex.Lock()
	defer catalogTypeMutex.Unlock()

	_, ok := catalogTypeLocks[catalogTypeID]
	if !ok {
		catalogTypeLocks[catalogTypeID] = new(sync.Mutex)
	}

	mutex := catalogTypeLocks[catalogTypeID]
	mutex.Lock()
	defer mutex.Unlock()

	typeResult, err := r.client.CatalogV2ShowTypeWithResponse(ctx, catalogTypeID)
	if err == nil && typeResult.StatusCode() >= 400 {
		err = fmt.Errorf(string(typeResult.Body))
	}
	if err != nil {
		return errors.Wrap(err, "Unable to get catalog type, got error")
	}

	return do(ctx, typeResult.JSON200.CatalogType)
}

func (*IncidentCatalogTypeAttributeResource) attributeToPayload(attribute client.CatalogTypeAttributeV2) client.CatalogTypeAttributePayloadV2 {
	var mode *client.CatalogTypeAttributePayloadV2Mode
	var path *[]client.CatalogTypeAttributePathItemPayloadV2
	if attribute.BacklinkAttribute != nil {
		mode = lo.ToPtr(client.CatalogTypeAttributePayloadV2ModeBacklink)
	}
	if attribute.Path != nil {
		mode = lo.ToPtr(client.CatalogTypeAttributePayloadV2ModePath)
		path = &[]client.CatalogTypeAttributePathItemPayloadV2{}
		for _, item := range *attribute.Path {
			*path = append(*path, client.CatalogTypeAttributePathItemPayloadV2{
				AttributeId: item.AttributeId,
			})
		}
	}

	return client.CatalogTypeAttributePayloadV2{
		Id:                lo.ToPtr(attribute.Id),
		Name:              attribute.Name,
		Type:              attribute.Type,
		Array:             attribute.Array,
		BacklinkAttribute: attribute.BacklinkAttribute,
		Path:              path,
		Mode:              mode,
	}
}
