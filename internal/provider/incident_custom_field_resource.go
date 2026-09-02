package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ resource.Resource                = &IncidentCustomFieldResource{}
	_ resource.ResourceWithImportState = &IncidentCustomFieldResource{}
)

type IncidentCustomFieldResource struct {
	resourceConfigurer
}

type IncidentCustomFieldResourceModel struct {
	ID                         types.String                                `tfsdk:"id"`
	Name                       types.String                                `tfsdk:"name"`
	Description                types.String                                `tfsdk:"description"`
	FieldType                  types.String                                `tfsdk:"field_type"`
	CatalogTypeID              types.String                                `tfsdk:"catalog_type_id"`
	FilterBy                   *IncidentCustomFieldFilterByOptionsModel    `tfsdk:"filter_by"`
	FixedFilter                *IncidentCustomFieldFixedFilterOptionsModel `tfsdk:"fixed_filter"`
	GroupByCatalogAttributeID  types.String                                `tfsdk:"group_by_catalog_attribute_id"`
	HelptextCatalogAttributeID types.String                                `tfsdk:"helptext_catalog_attribute_id"`
}

type IncidentCustomFieldFilterByOptionsModel struct {
	CustomFieldID      types.String `tfsdk:"custom_field_id"`
	CatalogAttributeID types.String `tfsdk:"catalog_attribute_id"`
}

type IncidentCustomFieldFixedFilterOptionsModel struct {
	CatalogAttributeID types.String   `tfsdk:"catalog_attribute_id"`
	Values             []types.String `tfsdk:"values"`
}

// customFieldFixedFilterDescription documents the fixed_filter block. The API schema
// references the options type without describing the attribute that holds it, so unlike
// most of this resource there's no docstring to reuse.
const customFieldFixedFilterDescription = "Restrict this catalog-backed field's options to catalog entries whose " +
	"`catalog_attribute_id` attribute references one of `values`. This is the static counterpart to `filter_by`: " +
	"rather than following whatever another custom field is set to on the incident, the filter is chosen up front " +
	"and is the same for every incident. Mutually exclusive with `filter_by`."

func NewIncidentCustomFieldResource() resource.Resource {
	return &IncidentCustomFieldResource{}
}

func (r *IncidentCustomFieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_field"
}

func (r *IncidentCustomFieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Custom Fields V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "description"),
				Required:            true,
			},
			"field_type": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("CustomFieldV2", "field_type"),
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog_type_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "catalog_type_id"),
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"filter_by": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "filter_by"),
				Attributes: map[string]schema.Attribute{
					"custom_field_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("CustomFieldFilterByOptionsV2", "custom_field_id"),
						Required:            true,
					},
					"catalog_attribute_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("CustomFieldFilterByOptionsV2", "catalog_attribute_id"),
						Required:            true,
					},
				},
			},
			"fixed_filter": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: customFieldFixedFilterDescription,
				Attributes: map[string]schema.Attribute{
					"catalog_attribute_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("CustomFieldFixedFilterOptionsV2", "catalog_attribute_id"),
						Required:            true,
					},
					"values": schema.SetAttribute{
						MarkdownDescription: apischema.Docstring("CustomFieldFixedFilterOptionsV2", "values"),
						Required:            true,
						ElementType:         types.StringType,
						Validators: []validator.Set{
							// The API stores the filter attribute and its values together, and
							// rejects an attribute with no values to filter on. Say so at plan
							// time rather than sending a request that can't succeed.
							setvalidator.SizeAtLeast(1),
						},
					},
				},
				Validators: []validator.Object{
					// Both filters live on the same field in the API, so setting one clears the
					// other: a config asking for both would silently lose one of them.
					objectvalidator.ConflictsWith(path.MatchRoot("filter_by")),
				},
			},
			"group_by_catalog_attribute_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "group_by_catalog_attribute_id"),
				Optional:            true,
			},
			"helptext_catalog_attribute_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "helptext_catalog_attribute_id"),
				Optional:            true,
			},
		},
	}
}

func (r *IncidentCustomFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCustomFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := client.CustomFieldsV2CreateJSONRequestBody{
		Name:                       data.Name.ValueString(),
		Description:                data.Description.ValueString(),
		FieldType:                  client.CustomFieldsCreatePayloadV2FieldType(data.FieldType.ValueString()),
		CatalogTypeId:              data.CatalogTypeID.ValueStringPointer(),
		GroupByCatalogAttributeId:  data.GroupByCatalogAttributeID.ValueStringPointer(),
		HelptextCatalogAttributeId: data.HelptextCatalogAttributeID.ValueStringPointer(),
	}

	if data.FilterBy != nil {
		payload.FilterBy = &client.CustomFieldFilterByOptionsV2{
			CatalogAttributeId: data.FilterBy.CatalogAttributeID.ValueString(),
			CustomFieldId:      data.FilterBy.CustomFieldID.ValueString(),
		}
	}
	payload.FixedFilter = data.FixedFilter.toPayload()

	result, err := r.client.CustomFieldsV2CreateWithResponse(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create custom field '%s', got error: %s", data.Name.ValueString(), err))
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

	result, err := r.client.CustomFieldsV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Custom field with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
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

	payload := client.CustomFieldsV2UpdateJSONRequestBody{
		Name:                       data.Name.ValueString(),
		Description:                data.Description.ValueString(),
		GroupByCatalogAttributeId:  data.GroupByCatalogAttributeID.ValueStringPointer(),
		HelptextCatalogAttributeId: data.HelptextCatalogAttributeID.ValueStringPointer(),
	}
	if data.FilterBy != nil {
		payload.FilterBy = &client.CustomFieldFilterByOptionsV2{
			CatalogAttributeId: data.FilterBy.CatalogAttributeID.ValueString(),
			CustomFieldId:      data.FilterBy.CustomFieldID.ValueString(),
		}
	}
	payload.FixedFilter = data.FixedFilter.toPayload()

	result, err := r.client.CustomFieldsV2UpdateWithResponse(ctx, data.ID.ValueString(), payload)
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

	_, err := r.client.CustomFieldsV2DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete custom field, got error: %s", err))
		return
	}
}

func (r *IncidentCustomFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentCustomFieldResource) buildModel(cf client.CustomFieldV2) *IncidentCustomFieldResourceModel {
	res := &IncidentCustomFieldResourceModel{
		ID:                         types.StringValue(cf.Id),
		Name:                       types.StringValue(cf.Name),
		Description:                types.StringValue(cf.Description),
		FieldType:                  types.StringValue(string(cf.FieldType)),
		CatalogTypeID:              types.StringPointerValue(cf.CatalogTypeId),
		GroupByCatalogAttributeID:  types.StringPointerValue(cf.GroupByCatalogAttributeId),
		HelptextCatalogAttributeID: types.StringPointerValue(cf.HelptextCatalogAttributeId),
	}

	if cf.FilterBy != nil {
		res.FilterBy = &IncidentCustomFieldFilterByOptionsModel{
			CustomFieldID:      types.StringValue(cf.FilterBy.CustomFieldId),
			CatalogAttributeID: types.StringValue(cf.FilterBy.CatalogAttributeId),
		}
	}

	if cf.FixedFilter != nil {
		res.FixedFilter = &IncidentCustomFieldFixedFilterOptionsModel{
			CatalogAttributeID: types.StringValue(cf.FixedFilter.CatalogAttributeId),
			Values: lo.Map(cf.FixedFilter.Values, func(value string, _ int) types.String {
				return types.StringValue(value)
			}),
		}
	}

	return res
}

// toPayload builds the API's fixed filter options, or nil when the block is absent. The
// API takes the attribute and its values together, so both travel as one optional object.
func (m *IncidentCustomFieldFixedFilterOptionsModel) toPayload() *client.CustomFieldFixedFilterOptionsV2 {
	if m == nil {
		return nil
	}

	return &client.CustomFieldFixedFilterOptionsV2{
		CatalogAttributeId: m.CatalogAttributeID.ValueString(),
		Values: lo.Map(m.Values, func(value types.String, _ int) string {
			return value.ValueString()
		}),
	}
}
