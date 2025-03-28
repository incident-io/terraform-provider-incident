package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                   = &IncidentCatalogEntryResource{}
	_ resource.ResourceWithImportState    = &IncidentCatalogEntryResource{}
	_ resource.ResourceWithValidateConfig = &IncidentCatalogEntryResource{}
)

type IncidentCatalogEntryResource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogEntryResourceModel struct {
	ID                types.String                 `tfsdk:"id"`
	CatalogTypeID     types.String                 `tfsdk:"catalog_type_id"`
	Name              types.String                 `tfsdk:"name"`
	ExternalID        types.String                 `tfsdk:"external_id"`
	Aliases           types.List                   `tfsdk:"aliases"`
	Rank              types.Int64                  `tfsdk:"rank"`
	AttributeValues   []CatalogEntryAttributeValue `tfsdk:"attribute_values"`
	ManagedAttributes types.Set                    `tfsdk:"managed_attributes"`
}

func (m IncidentCatalogEntryResourceModel) buildAttributeValues() map[string]client.CatalogEngineParamBindingPayloadV3 {
	values := map[string]client.CatalogEngineParamBindingPayloadV3{}

	for _, attributeValue := range m.AttributeValues {
		attrID := attributeValue.Attribute.ValueString()

		// Skip attributes that aren't managed
		if !m.isAttributeManaged(attrID) {
			continue
		}

		payload := client.CatalogEngineParamBindingPayloadV3{}
		if !attributeValue.Value.IsNull() {
			payload.Value = &client.CatalogEngineParamBindingValuePayloadV3{
				Literal: lo.ToPtr(attributeValue.Value.ValueString()),
			}
		}
		if !attributeValue.ArrayValue.IsNull() {
			arrayValue := []client.CatalogEngineParamBindingValuePayloadV3{}
			for _, element := range attributeValue.ArrayValue.Elements() {
				elementString, ok := element.(types.String)
				if !ok {
					panic(fmt.Sprintf("element should have been types.String but was %T", element))
				}
				arrayValue = append(arrayValue, client.CatalogEngineParamBindingValuePayloadV3{
					Literal: lo.ToPtr(elementString.ValueString()),
				})
			}

			payload.ArrayValue = &arrayValue
		}

		values[attrID] = payload
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
		MarkdownDescription: `
This resource manages a single entry for a given catalog type. It should be used when
you're loading a small number (<100) of catalog entries and want to do so with a Terraform
for_each, or you don't want terraform to remove any entries that it is not managing.

If you're working with a large number of entries (>100) or want to be authoritative
(remove anything Terraform does not manage) then prefer ` + "`incident_catalog_entries`" + `.
		`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"catalog_type_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "catalog_type_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Required: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "name"),
				Required:            true,
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "external_id"),
				Optional:            true,
			},
			"aliases": schema.ListAttribute{
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "aliases"),
				Optional:            true,
				Computed:            true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "rank"),
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
			"managed_attributes": schema.SetAttribute{
				ElementType: types.StringType,
				MarkdownDescription: `The set of attributes that are managed by this resource. By default, all attributes are managed by this resource.

This can be used to allow other attributes of a catalog entry to be managed elsewhere, for example in another Terraform repository or the incident.io web UI.`,
				Optional: true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *IncidentCatalogEntryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	var aliases []string
	if !data.Aliases.IsUnknown() {
		if diags := data.Aliases.ElementsAs(ctx, &aliases, false); diags.HasError() {
			resp.Diagnostics.AddError("Client Error", "Unable to read aliases")
			return
		}
	}

	result, err := r.client.CatalogV3CreateEntryWithResponse(ctx, client.CatalogCreateEntryPayloadV3{
		CatalogTypeId:   data.CatalogTypeID.ValueString(),
		Name:            data.Name.ValueString(),
		ExternalId:      data.ExternalID.ValueStringPointer(),
		Rank:            rank,
		Aliases:         &aliases,
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
	data = r.buildModel(result.JSON201.CatalogEntry, data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}

	if result.StatusCode() == 404 {
		resp.Diagnostics.AddWarning("Not Found", fmt.Sprintf("Unable to read catalog entry, got status code: %d", result.StatusCode()))
		resp.State.RemoveResource(ctx)
		return
	}

	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got status code: %d", result.StatusCode()))
		return
	}

	data = r.buildModel(result.JSON200.CatalogEntry, data)
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
	var aliases []string
	if !data.Aliases.IsUnknown() {
		if diags := data.Aliases.ElementsAs(ctx, &aliases, false); diags.HasError() {
			resp.Diagnostics.AddError("Client Error", "Unable to read aliases")
			return
		}
	}

	var updateAttributes *[]string
	if !data.ManagedAttributes.IsUnknown() && !data.ManagedAttributes.IsNull() {
		var attributeIDs []string
		diags := data.ManagedAttributes.ElementsAs(ctx, &attributeIDs, false)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		updateAttributes = &attributeIDs
	}

	result, err := r.client.CatalogV3UpdateEntryWithResponse(ctx, data.ID.ValueString(), client.CatalogUpdateEntryPayloadV3{
		Name:             data.Name.ValueString(),
		Rank:             rank,
		ExternalId:       data.ExternalID.ValueStringPointer(),
		Aliases:          &aliases,
		AttributeValues:  data.buildAttributeValues(),
		UpdateAttributes: updateAttributes,
	})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update catalog entry, got error: %s", err))
		return
	}

	updatedModel := r.buildModel(result.JSON200.CatalogEntry, data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updatedModel)...)
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

func (r *IncidentCatalogEntryResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data IncidentCatalogEntryResourceModel

	var attributeValues types.Set
	diag := req.Config.GetAttribute(ctx, path.Root("attribute_values"), &attributeValues)
	if diag.HasError() || attributeValues.IsUnknown() {
		// If attribute_values is unknown, don't attempt to validate the managed
		// attributes.
		return
	}

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If managed_attributes is not set, all attributes are valid
	if data.ManagedAttributes.IsNull() || data.ManagedAttributes.IsUnknown() {
		return
	}

	// Extract the managed attribute IDs
	managedAttributesMap := map[string]bool{}
	for _, attrIDElem := range data.ManagedAttributes.Elements() {
		if attrIDElem.IsUnknown() {
			// If any element in the list is unknown (e.g. a reference to an attribute
			// that hasn't been created yet), we give up and assume all attributes are
			// managed.
			//
			// This won't happen at apply-time, so the effect on the user is relatively
			// small, but it does meant that if you're creating a totally new config we
			// can't fully validate it on an initial `terraform plan`.
			continue
		}

		attrIDStr, ok := attrIDElem.(types.String)
		if !ok {
			continue
		}

		managedAttributesMap[attrIDStr.ValueString()] = true
	}

	// Check that each attribute in attribute_values is managed
	for idx, attributeValue := range data.AttributeValues {
		if attributeValue.Attribute.IsUnknown() {
			// Likewise here we give up trying to validate when the attribute ID isn't yet known.
			continue
		}

		attributeID := attributeValue.Attribute.ValueString()
		if !managedAttributesMap[attributeID] {
			resp.Diagnostics.AddAttributeError(
				path.Root("attribute_values").AtListIndex(idx),
				"Unmanaged Attribute",
				fmt.Sprintf("Attribute ID %q is specified in attribute_values but is not in the managed_attributes set. "+
					"Either add it to managed_attributes or remove it from attribute_values.", attributeID),
			)
		}
	}
}

// isAttributeManaged checks if the given attribute should be managed by this resource.
func (m *IncidentCatalogEntryResourceModel) isAttributeManaged(attributeID string) bool {
	// If managedAttributes is not set, all attributes are managed
	if m.ManagedAttributes.IsNull() || m.ManagedAttributes.IsUnknown() {
		return true
	}

	// Extract the managed attribute IDs
	var managedAttributeIDs []string
	diags := m.ManagedAttributes.ElementsAs(context.Background(), &managedAttributeIDs, false)
	if diags.HasError() || len(managedAttributeIDs) == 0 {
		// If there's an error or the list is empty, consider all attributes managed
		return true
	}

	// Check if the attribute is in the managed list
	for _, managedID := range managedAttributeIDs {
		if managedID == attributeID {
			return true
		}
	}

	// Not found in the managed list
	return false
}

func (r *IncidentCatalogEntryResource) buildModel(entry client.CatalogEntryV3, data *IncidentCatalogEntryResourceModel) *IncidentCatalogEntryResourceModel {
	values := []CatalogEntryAttributeValue{}

	for attributeID, binding := range entry.AttributeValues {
		// Skip attributes that aren't managed
		if !data.isAttributeManaged(attributeID) {
			continue
		}

		value := CatalogEntryAttributeValue{
			Attribute:  types.StringValue(attributeID),
			ArrayValue: types.ListNull(types.StringType),
		}
		// The API can behave weirdly in the case of empty arrays and omit the field entirely.
		// This is painful for us as terraform will see the omission as a diff against the
		// state, so we paper over the issue by instantiating an empty array value if we think
		// we're seeing the weirdness.
		if binding.Value == nil && binding.ArrayValue == nil {
			binding.ArrayValue = lo.ToPtr([]client.CatalogEntryEngineParamBindingValueV3{})
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

	aliases := []attr.Value{}
	for _, alias := range entry.Aliases {
		aliases = append(aliases, types.StringValue(alias))
	}

	return &IncidentCatalogEntryResourceModel{
		ID:              types.StringValue(entry.Id),
		CatalogTypeID:   types.StringValue(entry.CatalogTypeId),
		Name:            types.StringValue(entry.Name),
		ExternalID:      types.StringPointerValue(entry.ExternalId),
		Aliases:         types.ListValueMust(types.StringType, aliases),
		Rank:            types.Int64Value(int64(entry.Rank)),
		AttributeValues: values,
		// These are managed in config only
		ManagedAttributes: data.ManagedAttributes,
	}
}
