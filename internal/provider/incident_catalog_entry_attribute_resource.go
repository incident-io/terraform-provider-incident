package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentCatalogEntryAttributeResource{}
	_ resource.ResourceWithImportState = &IncidentCatalogEntryAttributeResource{}
)

type IncidentCatalogEntryAttributeResource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogEntryAttributeResourceModel struct {
	ID             types.String `tfsdk:"id"`
	CatalogEntryID types.String `tfsdk:"catalog_entry_id"`
	AttributeID    types.String `tfsdk:"attribute_id"`
	Value          types.String `tfsdk:"value"`
	ArrayValue     types.List   `tfsdk:"array_value"`
}

func NewIncidentCatalogEntryAttributeResource() resource.Resource {
	return &IncidentCatalogEntryAttributeResource{}
}

func (r *IncidentCatalogEntryAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_entry_attribute"
}

func (r *IncidentCatalogEntryAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `
This resource manages a single attribute value for a catalog entry. It provides a convenient way to
update individual attributes on existing catalog entries without having to manage the entire entry.

Note: This resource modifies the catalog entry's attribute values. If you are also managing the same
catalog entry with the ` + "`incident_catalog_entry`" + ` resource, make sure to exclude this attribute
from the managed_attributes set to avoid conflicts.
		`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for this resource (catalog_entry_id:attribute_id)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"catalog_entry_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the catalog entry to update",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"attribute_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the attribute to set",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The value to set for this attribute (for non-array attributes)",
				Optional:            true,
			},
			"array_value": schema.ListAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "The array value to set for this attribute (for array attributes)",
				Optional:            true,
			},
		},
	}
}

func (r *IncidentCatalogEntryAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client.Client
}

func (r *IncidentCatalogEntryAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogEntryAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// First, get the current catalog entry to preserve other attributes
	entryResult, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.CatalogEntryID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}
	if entryResult.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got status code: %d", entryResult.StatusCode()))
		return
	}

	entry := entryResult.JSON200.CatalogEntry

	// Build the attribute values map, preserving existing values
	attributeValues := make(map[string]client.CatalogEngineParamBindingPayloadV3)
	for attrID, binding := range entry.AttributeValues {
		payload := client.CatalogEngineParamBindingPayloadV3{}
		if binding.Value != nil {
			payload.Value = &client.CatalogEngineParamBindingValuePayloadV3{
				Literal: binding.Value.Literal,
			}
		}
		if binding.ArrayValue != nil {
			arrayValue := []client.CatalogEngineParamBindingValuePayloadV3{}
			for _, val := range *binding.ArrayValue {
				arrayValue = append(arrayValue, client.CatalogEngineParamBindingValuePayloadV3{
					Literal: val.Literal,
				})
			}
			payload.ArrayValue = &arrayValue
		}
		attributeValues[attrID] = payload
	}

	// Set the new attribute value
	newPayload := client.CatalogEngineParamBindingPayloadV3{}
	if !data.Value.IsNull() {
		newPayload.Value = &client.CatalogEngineParamBindingValuePayloadV3{
			Literal: lo.ToPtr(data.Value.ValueString()),
		}
	}
	if !data.ArrayValue.IsNull() {
		arrayValue := []client.CatalogEngineParamBindingValuePayloadV3{}
		for _, element := range data.ArrayValue.Elements() {
			elementString, ok := element.(types.String)
			if !ok {
				resp.Diagnostics.AddError("Type Error", fmt.Sprintf("Array element should be string but was %T", element))
				return
			}
			arrayValue = append(arrayValue, client.CatalogEngineParamBindingValuePayloadV3{
				Literal: lo.ToPtr(elementString.ValueString()),
			})
		}
		newPayload.ArrayValue = &arrayValue
	}
	attributeValues[data.AttributeID.ValueString()] = newPayload

	// Update the catalog entry with the new attribute value
	updateResult, err := r.client.CatalogV3UpdateEntryWithResponse(ctx, data.CatalogEntryID.ValueString(), client.CatalogUpdateEntryPayloadV3{
		Name:            entry.Name,
		Rank:            lo.ToPtr(entry.Rank),
		ExternalId:      entry.ExternalId,
		Aliases:         &entry.Aliases,
		AttributeValues: attributeValues,
	})
	if err == nil && updateResult.StatusCode() >= 400 {
		err = fmt.Errorf(string(updateResult.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update catalog entry attribute, got error: %s", err))
		return
	}

	// Set the resource ID and update the state
	data.ID = types.StringValue(fmt.Sprintf("%s:%s", data.CatalogEntryID.ValueString(), data.AttributeID.ValueString()))

	tflog.Trace(ctx, fmt.Sprintf("created catalog entry attribute resource with id=%s", data.ID.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogEntryAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogEntryAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the catalog entry
	result, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.CatalogEntryID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}

	if result.StatusCode() == 404 {
		resp.Diagnostics.AddWarning("Not Found", "Catalog entry not found, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got status code: %d", result.StatusCode()))
		return
	}

	entry := result.JSON200.CatalogEntry
	attributeID := data.AttributeID.ValueString()

	// Check if the attribute exists on the entry
	binding, exists := entry.AttributeValues[attributeID]
	if !exists {
		resp.Diagnostics.AddWarning("Attribute Not Found", "Attribute not found on catalog entry, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	// Update the model with current values
	data.Value = types.StringNull()
	data.ArrayValue = types.ListNull(types.StringType)

	if binding.Value != nil && binding.Value.Literal != nil {
		data.Value = types.StringValue(*binding.Value.Literal)
	}

	if binding.ArrayValue != nil {
		elements := make([]types.String, len(*binding.ArrayValue))
		for i, val := range *binding.ArrayValue {
			if val.Literal != nil {
				elements[i] = types.StringValue(*val.Literal)
			} else {
				elements[i] = types.StringValue("")
			}
		}

		listValue, diags := types.ListValueFrom(ctx, types.StringType, elements)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		data.ArrayValue = listValue
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogEntryAttributeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCatalogEntryAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// First, get the current catalog entry to preserve other attributes
	entryResult, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.CatalogEntryID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}
	if entryResult.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got status code: %d", entryResult.StatusCode()))
		return
	}

	entry := entryResult.JSON200.CatalogEntry

	// Build the attribute values map, preserving existing values
	attributeValues := make(map[string]client.CatalogEngineParamBindingPayloadV3)
	for attrID, binding := range entry.AttributeValues {
		payload := client.CatalogEngineParamBindingPayloadV3{}
		if binding.Value != nil {
			payload.Value = &client.CatalogEngineParamBindingValuePayloadV3{
				Literal: binding.Value.Literal,
			}
		}
		if binding.ArrayValue != nil {
			arrayValue := []client.CatalogEngineParamBindingValuePayloadV3{}
			for _, val := range *binding.ArrayValue {
				arrayValue = append(arrayValue, client.CatalogEngineParamBindingValuePayloadV3{
					Literal: val.Literal,
				})
			}
			payload.ArrayValue = &arrayValue
		}
		attributeValues[attrID] = payload
	}

	// Update the specific attribute value
	newPayload := client.CatalogEngineParamBindingPayloadV3{}
	if !data.Value.IsNull() {
		newPayload.Value = &client.CatalogEngineParamBindingValuePayloadV3{
			Literal: lo.ToPtr(data.Value.ValueString()),
		}
	}
	if !data.ArrayValue.IsNull() {
		arrayValue := []client.CatalogEngineParamBindingValuePayloadV3{}
		for _, element := range data.ArrayValue.Elements() {
			elementString, ok := element.(types.String)
			if !ok {
				resp.Diagnostics.AddError("Type Error", fmt.Sprintf("Array element should be string but was %T", element))
				return
			}
			arrayValue = append(arrayValue, client.CatalogEngineParamBindingValuePayloadV3{
				Literal: lo.ToPtr(elementString.ValueString()),
			})
		}
		newPayload.ArrayValue = &arrayValue
	}
	attributeValues[data.AttributeID.ValueString()] = newPayload

	// Update the catalog entry
	updateResult, err := r.client.CatalogV3UpdateEntryWithResponse(ctx, data.CatalogEntryID.ValueString(), client.CatalogUpdateEntryPayloadV3{
		Name:            entry.Name,
		Rank:            lo.ToPtr(entry.Rank),
		ExternalId:      entry.ExternalId,
		Aliases:         &entry.Aliases,
		AttributeValues: attributeValues,
	})
	if err == nil && updateResult.StatusCode() >= 400 {
		err = fmt.Errorf(string(updateResult.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update catalog entry attribute, got error: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentCatalogEntryAttributeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogEntryAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the current catalog entry
	entryResult, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.CatalogEntryID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}
	if entryResult.StatusCode() == 404 {
		// Entry doesn't exist, nothing to do
		return
	}
	if entryResult.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got status code: %d", entryResult.StatusCode()))
		return
	}

	entry := entryResult.JSON200.CatalogEntry

	// Build the attribute values map, excluding the attribute we're deleting
	attributeValues := make(map[string]client.CatalogEngineParamBindingPayloadV3)
	for attrID, binding := range entry.AttributeValues {
		if attrID == data.AttributeID.ValueString() {
			continue // Skip the attribute we're deleting
		}

		payload := client.CatalogEngineParamBindingPayloadV3{}
		if binding.Value != nil {
			payload.Value = &client.CatalogEngineParamBindingValuePayloadV3{
				Literal: binding.Value.Literal,
			}
		}
		if binding.ArrayValue != nil {
			arrayValue := []client.CatalogEngineParamBindingValuePayloadV3{}
			for _, val := range *binding.ArrayValue {
				arrayValue = append(arrayValue, client.CatalogEngineParamBindingValuePayloadV3{
					Literal: val.Literal,
				})
			}
			payload.ArrayValue = &arrayValue
		}
		attributeValues[attrID] = payload
	}

	// Update the catalog entry without the deleted attribute
	_, err = r.client.CatalogV3UpdateEntryWithResponse(ctx, data.CatalogEntryID.ValueString(), client.CatalogUpdateEntryPayloadV3{
		Name:            entry.Name,
		Rank:            lo.ToPtr(entry.Rank),
		ExternalId:      entry.ExternalId,
		Aliases:         &entry.Aliases,
		AttributeValues: attributeValues,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete catalog entry attribute, got error: %s", err))
		return
	}
}

func (r *IncidentCatalogEntryAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import state expects format: "catalog_entry_id:attribute_id"
	idParts := strings.Split(req.ID, ":")
	if len(idParts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"Import ID must be in the format 'catalog_entry_id:attribute_id'",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog_entry_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("attribute_id"), idParts[1])...)
}
