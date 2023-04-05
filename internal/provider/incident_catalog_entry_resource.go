package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	_ resource.Resource                = &IncidentCatalogEntryResource{}
	_ resource.ResourceWithImportState = &IncidentCatalogEntryResource{}
)

type IncidentCatalogEntryResource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogEntryResourceModel struct {
	ID              types.String                 `tfsdk:"id"`
	CatalogTypeID   types.String                 `tfsdk:"catalog_type_id"`
	Name            types.String                 `tfsdk:"name"`
	Alias           types.String                 `tfsdk:"alias"`
	Rank            types.Int64                  `tfsdk:"rank"`
	AttributeValues []CatalogEntryAttributeValue `tfsdk:"attribute_values"`
}

func (m IncidentCatalogEntryResourceModel) buildAttributeValues() map[string]client.CatalogAttributeBindingPayloadV2 {
	values := map[string]client.CatalogAttributeBindingPayloadV2{}
	for _, attributeValue := range m.AttributeValues {
		payload := client.CatalogAttributeBindingPayloadV2{}
		if !attributeValue.Value.IsNull() {
			payload.Value = &client.CatalogAttributeValuePayloadV2{
				Literal: lo.ToPtr(attributeValue.Value.ValueString()),
			}
		}
		if !attributeValue.ArrayValue.IsNull() {
			arrayValue := []client.CatalogAttributeValuePayloadV2{}
			for _, element := range attributeValue.ArrayValue.Elements() {
				elementString, ok := element.(types.String)
				if !ok {
					panic(fmt.Sprintf("element should have been types.String but was %T", element))
				}
				arrayValue = append(arrayValue, client.CatalogAttributeValuePayloadV2{
					Literal: lo.ToPtr(elementString.ValueString()),
				})
			}

			payload.ArrayValue = &arrayValue
		}

		values[attributeValue.Attribute.ValueString()] = payload
	}

	return values
}

type CatalogEntryAttributeValue struct {
	Attribute  types.String `tfsdk:"attribute"`
	Value      types.String `tfsdk:"value"`
	ArrayValue types.List   `tfsdk:"array_value"`
}

func NewIncidentCatalogEntryResource() resource.Resource {
	return &IncidentCatalogEntryResource{}
}

func (r *IncidentCatalogEntryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_entry"
}

func (r *IncidentCatalogEntryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Catalog V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CatalogEntryV2ResponseBody", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"catalog_type_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2ResponseBody", "catalog_type_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Required: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2ResponseBody", "name"),
				Required:            true,
			},
			"alias": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2ResponseBody", "alias"),
				Optional:            true,
				Computed:            true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2ResponseBody", "rank"),
				Optional:            true,
				Computed:            true,
			},
			"attribute_values": schema.SetNestedAttribute{
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"attribute": schema.StringAttribute{
							Description: `The ID of this attribute, usually loaded from the incident_catalog_type_attribute resource.`,
							Required:    true,
						},
						"value": schema.StringAttribute{
							Description: `The value of this attribute, in a format suitable for this attribute type.`,
							Optional:    true,
						},
						"array_value": schema.ListAttribute{
							ElementType: types.StringType,
							Description: `The value of this element of the array, in a format suitable for this attribute type.`,
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (r *IncidentCatalogEntryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentCatalogEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rank *int32
	if !data.Rank.IsNull() {
		rank = lo.ToPtr(int32(data.Rank.ValueInt64()))
	}
	var alias *string
	if !data.Alias.IsUnknown() {
		alias = lo.ToPtr(data.Alias.ValueString())
	}
	result, err := r.client.CatalogV2CreateEntryWithResponse(ctx, client.CreateEntryRequestBody{
		CatalogTypeId:   data.CatalogTypeID.ValueString(),
		Name:            data.Name.ValueString(),
		Rank:            rank,
		Alias:           alias,
		AttributeValues: data.buildAttributeValues(),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create catalog entry, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a catalog entry resource with id=%s", result.JSON201.CatalogEntry.Id))
	data = r.buildModel(result.JSON201.CatalogEntry)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV2ShowEntryWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident severity, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CatalogEntry)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogEntryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCatalogEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rank *int32
	if !data.Rank.IsNull() {
		rank = lo.ToPtr(int32(data.Rank.ValueInt64()))
	}
	var alias *string
	if !data.Alias.IsUnknown() {
		alias = lo.ToPtr(data.Alias.ValueString())
	}
	result, err := r.client.CatalogV2UpdateEntryWithResponse(ctx, data.ID.ValueString(), client.UpdateEntryRequestBody{
		Name:            data.Name.ValueString(),
		Rank:            rank,
		Alias:           alias,
		AttributeValues: data.buildAttributeValues(),
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update catalog entry, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.CatalogEntry)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogEntryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CatalogV2DestroyEntry(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete catalog entry, got error: %s", err))
		return
	}
}

func (r *IncidentCatalogEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentCatalogEntryResource) buildModel(entry client.CatalogEntryV2) *IncidentCatalogEntryResourceModel {
	values := []CatalogEntryAttributeValue{}
	for attributeID, binding := range entry.AttributeValues {
		value := CatalogEntryAttributeValue{
			Attribute:  types.StringValue(attributeID),
			ArrayValue: types.ListNull(types.StringType),
		}
		// The API can behave weirdly in the case of empty arrays and omit the field entirely.
		// This is painful for us as terraform will see the omission as a diff against the
		// state, so we paper over the issue by instantiating an empty array value if we think
		// we're seeing the weirdness.
		if binding.Value == nil && binding.ArrayValue == nil {
			binding.ArrayValue = lo.ToPtr([]client.CatalogAttributeValueV2{})
		}

		if binding.Value != nil {
			value.Value = types.StringValue(*binding.Value.Literal)
		}
		if binding.ArrayValue != nil {
			elements := []attr.Value{}
			for _, value := range *binding.ArrayValue {
				elements = append(elements, types.StringValue(*value.Literal))
			}

			value.ArrayValue = types.ListValueMust(types.StringType, elements)
		}

		values = append(values, value)
	}

	// This ensures we get a stable read of the resource, rather than hitting
	// non-deterministic map key iteration.
	sort.Slice(values, func(i, j int) bool {
		return values[i].Attribute.ValueString() < values[j].Attribute.ValueString()
	})

	alias := types.StringNull()
	if entry.Alias != nil {
		alias = types.StringValue(*entry.Alias)
	}

	return &IncidentCatalogEntryResourceModel{
		ID:              types.StringValue(entry.Id),
		CatalogTypeID:   types.StringValue(entry.CatalogTypeId),
		Name:            types.StringValue(entry.Name),
		Alias:           alias,
		Rank:            types.Int64Value(int64(entry.Rank)),
		AttributeValues: values,
	}
}
