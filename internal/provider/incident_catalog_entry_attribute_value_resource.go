package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var (
	_ resource.Resource                     = &IncidentCatalogEntryAttributeValueResource{}
	_ resource.ResourceWithConfigValidators = &IncidentCatalogEntryAttributeValueResource{}
	_ resource.ResourceWithImportState      = &IncidentCatalogEntryAttributeValueResource{}
)

type IncidentCatalogEntryAttributeValueResource struct {
	resourceConfigurer
}

type IncidentCatalogEntryAttributeValueResourceModel struct {
	EntryID     types.String `tfsdk:"entry_id"`
	AttributeID types.String `tfsdk:"attribute_id"`
	Value       types.String `tfsdk:"value"`
	ArrayValue  types.List   `tfsdk:"array_value"`
}

func NewIncidentCatalogEntryAttributeValueResource() resource.Resource {
	return &IncidentCatalogEntryAttributeValueResource{}
}

func (r *IncidentCatalogEntryAttributeValueResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_entry_attribute_value"
}

func (r *IncidentCatalogEntryAttributeValueResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `
This resource sets one attribute on a catalog entry that something else owns, usually an
entry that incident.io syncs, such as a schedule, escalation path or user. The sync owns
the entry and its synced attributes, so you can't manage it with ` + "`incident_catalog_entry`" + `:
look it up with the ` + "`incident_catalog_entry`" + ` data source instead.

Use it for attributes you added to the catalog type yourself, for example with
` + "`incident_catalog_type_attribute`" + ` or in the incident.io web UI. Terraform writes only
this attribute, and puts it back on the next apply if someone changes it elsewhere. When
you destroy the resource, Terraform clears the attribute and leaves the entry in place.

Don't also manage the attribute with ` + "`incident_catalog_entry`" + ` or
` + "`incident_catalog_entries`" + `: the two would overwrite each other.
		`,
		Attributes: map[string]schema.Attribute{
			"entry_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the catalog entry, usually from the `incident_catalog_entry` data source.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"attribute_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the attribute, usually from the `incident_catalog_type_attribute` resource or data source.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The value of the attribute, in a format suitable for its type. Set this or `array_value`.",
				Optional:            true,
				Validators: []validator.String{
					// The API drops empty values, which would make the applied value differ
					// from the config. Destroy the resource to clear the attribute.
					stringvalidator.LengthAtLeast(1),
				},
			},
			"array_value": schema.ListAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "The values of an array attribute, each in a format suitable for its type. Set this or `value`.",
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
			},
		},
	}
}

func (r *IncidentCatalogEntryAttributeValueResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot("value"), path.MatchRoot("array_value")),
	}
}

func (r *IncidentCatalogEntryAttributeValueResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogEntryAttributeValueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.write(ctx, data, &resp.State, &resp.Diagnostics)
}

func (r *IncidentCatalogEntryAttributeValueResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogEntryAttributeValueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.EntryID.ValueString(), nil)
	if err != nil {
		if httpErr := (client.HTTPError{}); errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Catalog entry with ID %s not found: removing from state.", data.EntryID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, r.buildModel(result.JSON200.CatalogEntry, data.AttributeID.ValueString()))...)
}

func (r *IncidentCatalogEntryAttributeValueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCatalogEntryAttributeValueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.write(ctx, data, &resp.State, &resp.Diagnostics)
}

func (r *IncidentCatalogEntryAttributeValueResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogEntryAttributeValueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Writing the attribute with no value clears it. If the sync has already removed the
	// entry, there's nothing left to clear.
	_, diags := writeCatalogEntryPartially(ctx, r.client, data.EntryID.ValueString(), catalogEntryPartialWrite{
		UpdateAttributes: []string{data.AttributeID.ValueString()},
	})
	resp.Diagnostics.Append(diags...)
}

func (r *IncidentCatalogEntryAttributeValueResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	entryID, attributeID, ok := strings.Cut(req.ID, ":")
	if !ok || entryID == "" || attributeID == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected an import ID of the form <entry_id>:<attribute_id>, got %q.", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("entry_id"), entryID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("attribute_id"), attributeID)...)
}

func (r *IncidentCatalogEntryAttributeValueResource) write(ctx context.Context, data *IncidentCatalogEntryAttributeValueResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	attributeID := data.AttributeID.ValueString()
	attributeValues, valueDiags := buildCatalogEntryAttributeValues([]CatalogEntryAttributeValue{{
		Attribute:  data.AttributeID,
		Value:      data.Value,
		ArrayValue: data.ArrayValue,
	}}, func(string) bool { return true })
	diags.Append(valueDiags...)
	if diags.HasError() {
		return
	}

	entry, writeDiags := writeCatalogEntryPartially(ctx, r.client, data.EntryID.ValueString(), catalogEntryPartialWrite{
		UpdateAttributes: []string{attributeID},
		AttributeValues:  attributeValues,
	})
	diags.Append(writeDiags...)
	if diags.HasError() {
		return
	}
	if entry == nil {
		diags.AddAttributeError(path.Root("entry_id"), "Catalog entry not found",
			fmt.Sprintf("No catalog entry has the ID %q. Terraform doesn't create the entries it sets attributes on.", data.EntryID.ValueString()))
		return
	}

	diags.Append(state.Set(ctx, r.buildModel(*entry, attributeID))...)
}

// buildModel reads back the one attribute this resource owns. An attribute with no value
// leaves both value and array_value null, so Terraform plans to set it again.
func (r *IncidentCatalogEntryAttributeValueResource) buildModel(entry client.CatalogEntryV3, attributeID string) *IncidentCatalogEntryAttributeValueResourceModel {
	model := &IncidentCatalogEntryAttributeValueResourceModel{
		EntryID:     types.StringValue(entry.Id),
		AttributeID: types.StringValue(attributeID),
		Value:       types.StringNull(),
		ArrayValue:  types.ListNull(types.StringType),
	}

	binding, ok := entry.AttributeValues[attributeID]
	if !ok {
		return model
	}
	if binding.Value != nil && binding.Value.Literal != nil {
		model.Value = types.StringValue(*binding.Value.Literal)
	}
	if binding.ArrayValue != nil && len(*binding.ArrayValue) > 0 {
		model.ArrayValue = types.ListValueMust(types.StringType, lo.Map(*binding.ArrayValue, func(element client.CatalogEntryEngineParamBindingValueV3, _ int) attr.Value {
			return types.StringValue(lo.FromPtr(element.Literal))
		}))
	}

	return model
}
