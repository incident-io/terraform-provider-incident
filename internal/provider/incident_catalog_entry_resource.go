package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var (
	_ resource.Resource                   = &IncidentCatalogEntryResource{}
	_ resource.ResourceWithImportState    = &IncidentCatalogEntryResource{}
	_ resource.ResourceWithValidateConfig = &IncidentCatalogEntryResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentCatalogEntryResource{}
)

type IncidentCatalogEntryResource struct {
	resourceConfigurer
}

type IncidentCatalogEntryResourceModel struct {
	ID                      types.String                 `tfsdk:"id"`
	CatalogTypeID           types.String                 `tfsdk:"catalog_type_id"`
	ExistingEntryIdentifier types.String                 `tfsdk:"existing_entry_identifier"`
	Name                    types.String                 `tfsdk:"name"`
	ExternalID              types.String                 `tfsdk:"external_id"`
	Aliases                 types.List                   `tfsdk:"aliases"`
	CustomAliases           types.Set                    `tfsdk:"custom_aliases"`
	Rank                    types.Int64                  `tfsdk:"rank"`
	AttributeValues         []CatalogEntryAttributeValue `tfsdk:"attribute_values"`
	ManagedAttributes       types.Set                    `tfsdk:"managed_attributes"`
}

// adopts reports whether this resource adopts an entry that already exists, rather than
// creating one. Something else, usually an incident.io sync, owns an adopted entry: it
// sets the name, external ID and synced aliases, and creates and deletes the entry.
func (m IncidentCatalogEntryResourceModel) adopts() bool {
	return !m.ExistingEntryIdentifier.IsNull()
}

func (m IncidentCatalogEntryResourceModel) buildAttributeValues() (map[string]client.CatalogEngineParamBindingPayloadV3, diag.Diagnostics) {
	var diags diag.Diagnostics
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
			for idx, element := range attributeValue.ArrayValue.Elements() {
				elementString, ok := element.(types.String)
				if !ok {
					diags.AddAttributeError(
						path.Root("attribute_values"),
						"Unexpected attribute value type",
						fmt.Sprintf("array_value[%d] of attribute %q should have been a string but was %T. Please report this issue to the provider developers.", idx, attrID, element),
					)
					return nil, diags
				}
				arrayValue = append(arrayValue, client.CatalogEngineParamBindingValuePayloadV3{
					Literal: lo.ToPtr(elementString.ValueString()),
				})
			}

			payload.ArrayValue = &arrayValue
		}

		values[attrID] = payload
	}

	return values, diags
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

## Extending entries that incident.io syncs

incident.io syncs the entries of some catalog types, such as Schedule, Escalation path, User
and the types an integration syncs. The sync creates and deletes these entries, and owns their
name, external ID, aliases and synced attributes, so Terraform can't create them.

To extend one of these entries, set ` + "`existing_entry_identifier`" + ` to adopt it. Terraform
then finds the entry rather than creating it, and writes only what you give it:

- ` + "`custom_aliases`" + `: aliases your organisation adds to the entry, which syncs keep. When set,
  Terraform owns the entry's whole custom alias list. Leave it out to leave them alone.
- ` + "`attribute_values`" + ` for the attributes listed in ` + "`managed_attributes`" + `: attributes you
  added to the catalog type yourself, for example in the incident.io web UI. Terraform doesn't
  touch any other attribute.

Terraform never deletes an adopted entry. When you destroy the resource, it clears the custom
aliases (if it owned them) and the managed attributes, and leaves the entry in place. If no
entry matches the identifier, Terraform fails rather than creating one. Nothing stops people
editing the entry in the dashboard: Terraform puts back what it owns on the next apply.

Changing ` + "`custom_aliases`" + ` on the User type, or on another type that team membership passes
through, needs an API key with the ` + "`team.memberships_update`" + ` scope ("Update team
memberships"), because aliases on those entries decide who belongs to a team.
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
			"existing_entry_identifier": schema.StringAttribute{
				MarkdownDescription: `Adopt an entry that already exists in this catalog type, rather than creating one. Set it to the entry's ID, external ID or one of its aliases (or its name, if the type uses names as identifiers). Use this to extend entries that incident.io syncs, such as schedules, escalation paths and users.

When set, ` + "`name`" + `, ` + "`aliases`" + `, ` + "`external_id`" + ` and ` + "`rank`" + ` are read-only, Terraform writes only ` + "`custom_aliases`" + ` and the attributes in ` + "`managed_attributes`" + `, and destroying the resource leaves the entry in place. Terraform fails if no entry, or more than one, matches.`,
				Optional: true,
				PlanModifiers: []planmodifier.String{
					// Pointing at a different entry means letting go of this one, but setting it
					// on a resource already in state (for example an imported one) adopts the
					// entry Terraform already tracks, which Update checks.
					stringplanmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = !req.StateValue.IsNull()
						},
						"Changing which entry is adopted replaces the resource.",
						"Changing which entry is adopted replaces the resource.",
					),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "name") + ". Required, unless `existing_entry_identifier` is set, when it is read-only.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "external_id") + ". Read-only when `existing_entry_identifier` is set.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"aliases": schema.ListAttribute{
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "aliases") + ". Read-only when `existing_entry_identifier` is set, and then includes the entry's custom aliases.",
				Optional:            true,
				Computed:            true,
			},
			"custom_aliases": schema.SetAttribute{
				ElementType: types.StringType,
				MarkdownDescription: `Aliases your organisation adds to an adopted entry, which incident.io syncs keep and also lists in ` + "`aliases`" + `. Only for entries adopted with ` + "`existing_entry_identifier`" + `, on catalog types that incident.io syncs.

When set, Terraform owns the entry's whole custom alias list: it removes custom aliases added elsewhere, and clears the list when you destroy the resource. Set it to ` + "`[]`" + ` to clear them. Leave it out to leave them as they are. Changing them on the User type, or another type that team membership passes through, needs the ` + "`team.memberships_update`" + ` scope.`,
				Optional: true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "rank") + ". Read-only when `existing_entry_identifier` is set.",
				Optional:            true,
				Computed:            true,
			},
			"attribute_values": schema.SetNestedAttribute{
				MarkdownDescription: "The attribute values of this entry. Required, unless `existing_entry_identifier` is set.",
				Optional:            true,
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

This can be used to allow other attributes of a catalog entry to be managed elsewhere, for example in another Terraform repository or the incident.io web UI.

When ` + "`managed_attributes`" + ` is set, destroying the Terraform resource will clear only those attributes instead of deleting the catalog entry. This enables partial management of entries owned by external systems (e.g., Schedules from the on-call product).

When ` + "`existing_entry_identifier`" + ` is set, Terraform manages no attributes unless you list them here.`,
				Optional: true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *IncidentCatalogEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.adopts() {
		entry, catalogType, diags := r.findEntryToAdopt(ctx, data.CatalogTypeID.ValueString(), data.ExistingEntryIdentifier.ValueString())
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		entry, diags = r.writeAdoptedEntry(ctx, *entry, catalogType, data)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		tflog.Trace(ctx, fmt.Sprintf("adopted an existing catalog entry with id=%s", entry.Id))
		data = r.buildModel(*entry, data)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

	externalID := data.ExternalID.ValueStringPointer()
	if externalID != nil && *externalID == "" {
		externalID = nil
	}

	attributeValues, diags := data.buildAttributeValues()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV3CreateEntryWithResponse(ctx, client.CatalogCreateEntryPayloadV3{
		CatalogTypeId:   data.CatalogTypeID.ValueString(),
		Name:            data.Name.ValueString(),
		ExternalId:      externalID,
		Rank:            rank,
		Aliases:         &aliases,
		AttributeValues: attributeValues,
	})
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

	result, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.ID.ValueString(), nil)
	if err != nil {
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Catalog entry with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
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

	if data.adopts() {
		resp.Diagnostics.Append(r.updateAdoptedEntry(ctx, req, data, &resp.State)...)
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

	externalID := data.ExternalID.ValueStringPointer()
	if externalID != nil && *externalID == "" {
		externalID = nil
	}

	attributeValues, diags := data.buildAttributeValues()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV3UpdateEntryWithResponse(ctx, data.ID.ValueString(), client.CatalogUpdateEntryPayloadV3{
		Name:             data.Name.ValueString(),
		Rank:             rank,
		ExternalId:       externalID,
		Aliases:          &aliases,
		AttributeValues:  attributeValues,
		UpdateAttributes: updateAttributes,
	})
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

	if data.adopts() {
		resp.Diagnostics.Append(r.releaseAdoptedEntry(ctx, data)...)
		return
	}

	// If managed_attributes is set, we're only managing specific attributes on an entry
	// that may be owned elsewhere. Instead of deleting the entry, clear the managed
	// attributes by sending an update with empty values.
	if !data.ManagedAttributes.IsNull() && !data.ManagedAttributes.IsUnknown() {
		var managedAttributeIDs []string
		diags := data.ManagedAttributes.ElementsAs(ctx, &managedAttributeIDs, false)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		// Only proceed if there are actually managed attributes to clear
		if len(managedAttributeIDs) > 0 {
			// Clear the managed attributes by sending an update with empty attribute values
			// and specifying the managed attributes in UpdateAttributes (this clears them)
			_, err := r.client.CatalogV3UpdateEntryWithResponse(ctx, data.ID.ValueString(), client.CatalogUpdateEntryPayloadV3{
				Name:             data.Name.ValueString(),
				AttributeValues:  map[string]client.CatalogEngineParamBindingPayloadV3{},
				UpdateAttributes: &managedAttributeIDs,
			})
			if err != nil {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to clear managed attributes on catalog entry, got error: %s", err))
				return
			}

			tflog.Info(ctx, fmt.Sprintf("Cleared managed attributes on catalog entry %s instead of deleting (entry is partially managed)", data.ID.ValueString()))
			return
		}
	}

	// If no managed_attributes set, delete the entire entry as before.
	//
	// oapi-codegen 2.8.0 started emitting the schema's `deprecated` flag as a real
	// Go "Deprecated:" marker, so staticcheck now reports this call. The replacement
	// is CatalogV3DestroyEntry, but moving the delete path from v2 to v3 is a
	// functional change that wants its own PR and acceptance run, not a codegen bump.
	_, err := r.client.CatalogV2DestroyEntry(ctx, data.ID.ValueString()) //nolint:staticcheck // migrate to CatalogV3DestroyEntry separately.
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete catalog entry, got error: %s", err))
		return
	}
}

// findEntryToAdopt finds the entry that existing_entry_identifier points at. It never
// creates one: an adopted entry belongs to whatever syncs it, so a missing entry is an
// error the user has to fix, for example by waiting for the sync.
func (r *IncidentCatalogEntryResource) findEntryToAdopt(ctx context.Context, catalogTypeID, identifier string) (*client.CatalogEntryV3, client.CatalogTypeV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	result, err := r.client.CatalogV3ListEntriesWithResponse(ctx, &client.CatalogV3ListEntriesParams{
		CatalogTypeId: catalogTypeID,
		Identifier:    &identifier,
		// Two is enough to tell a unique match from an ambiguous one.
		PageSize: 2,
	})
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to find the catalog entry to adopt, got error: %s", err))
		return nil, client.CatalogTypeV3{}, diags
	}

	entries := result.JSON200.CatalogEntries
	catalogType := result.JSON200.CatalogType
	switch {
	case len(entries) == 0:
		diags.AddAttributeError(
			path.Root("existing_entry_identifier"),
			"Catalog entry not found",
			fmt.Sprintf("No entry in the %q catalog type has the identifier %q, so there is nothing to adopt. "+
				"Terraform doesn't create entries it adopts: check the identifier, and that the entry exists (for example, that its sync has run).",
				catalogType.Name, identifier),
		)
		return nil, catalogType, diags
	case len(entries) > 1:
		diags.AddAttributeError(
			path.Root("existing_entry_identifier"),
			"Catalog entry identifier is ambiguous",
			fmt.Sprintf("More than one entry in the %q catalog type has the identifier %q. Use the entry's ID or external ID instead.",
				catalogType.Name, identifier),
		)
		return nil, catalogType, diags
	}

	return &entries[0], catalogType, diags
}

// writeAdoptedEntry writes what Terraform owns on an adopted entry: its custom aliases,
// if set, and the managed attributes. It sends back the entry's current name and rank,
// which an update always writes, and leaves out aliases and the external ID, so the API
// keeps the synced ones.
func (r *IncidentCatalogEntryResource) writeAdoptedEntry(ctx context.Context, entry client.CatalogEntryV3, catalogType client.CatalogTypeV3, data *IncidentCatalogEntryResourceModel) (*client.CatalogEntryV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	var customAliases *[]string
	if !data.CustomAliases.IsNull() {
		if !catalogType.SupportsCustomAliases {
			diags.AddAttributeError(
				path.Root("custom_aliases"),
				"Catalog type doesn't take custom aliases",
				fmt.Sprintf("Entries of the %q catalog type don't take custom aliases. Only catalog types that incident.io syncs do, "+
					"and not those whose aliases you edit elsewhere, such as alert priorities.", catalogType.Name),
			)
			return nil, diags
		}

		aliases := []string{}
		diags.Append(data.CustomAliases.ElementsAs(ctx, &aliases, false)...)
		if diags.HasError() {
			return nil, diags
		}
		customAliases = &aliases
	}

	managedAttributeIDs, attrDiags := data.managedAttributeIDs(ctx)
	diags.Append(attrDiags...)
	if diags.HasError() {
		return nil, diags
	}

	// Nothing to write, so don't touch the entry.
	if customAliases == nil && len(managedAttributeIDs) == 0 {
		return &entry, diags
	}

	attributeValues, valueDiags := data.buildAttributeValues()
	diags.Append(valueDiags...)
	if diags.HasError() {
		return nil, diags
	}

	result, err := r.client.CatalogV3UpdateEntryWithResponse(ctx, entry.Id, client.CatalogUpdateEntryPayloadV3{
		Name:            entry.Name,
		Rank:            lo.ToPtr(entry.Rank),
		CustomAliases:   customAliases,
		AttributeValues: attributeValues,
		// Always set, even when empty: without it the API replaces every attribute an API
		// key can set, which would clear ones Terraform doesn't manage.
		UpdateAttributes: &managedAttributeIDs,
	})
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to update adopted catalog entry, got error: %s", err))
		return nil, diags
	}

	return &result.JSON200.CatalogEntry, diags
}

func (r *IncidentCatalogEntryResource) updateAdoptedEntry(ctx context.Context, req resource.UpdateRequest, data *IncidentCatalogEntryResourceModel, state *tfsdk.State) diag.Diagnostics {
	var diags diag.Diagnostics

	var priorIdentifier types.String
	diags.Append(req.State.GetAttribute(ctx, path.Root("existing_entry_identifier"), &priorIdentifier)...)
	if diags.HasError() {
		return diags
	}

	// A resource already in state starts adopting, for example after an import. Check the
	// identifier points at the entry Terraform already tracks, rather than writing to one
	// entry and recording another.
	if priorIdentifier.IsNull() {
		found, _, findDiags := r.findEntryToAdopt(ctx, data.CatalogTypeID.ValueString(), data.ExistingEntryIdentifier.ValueString())
		diags.Append(findDiags...)
		if diags.HasError() {
			return diags
		}
		if found.Id != data.ID.ValueString() {
			diags.AddAttributeError(
				path.Root("existing_entry_identifier"),
				"Catalog entry identifier doesn't match",
				fmt.Sprintf("existing_entry_identifier points at entry %s, but this resource manages entry %s. "+
					"Set it to an identifier of %s, or remove the resource from state and adopt the other entry.",
					found.Id, data.ID.ValueString(), data.ID.ValueString()),
			)
			return diags
		}
	}

	// Read the entry fresh, so we send back the name and rank it has now, not the ones
	// from the last refresh.
	current, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.ID.ValueString(), nil)
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to read adopted catalog entry, got error: %s", err))
		return diags
	}

	entry, writeDiags := r.writeAdoptedEntry(ctx, current.JSON200.CatalogEntry, current.JSON200.CatalogType, data)
	diags.Append(writeDiags...)
	if diags.HasError() {
		return diags
	}

	diags.Append(state.Set(ctx, r.buildModel(*entry, data))...)
	return diags
}

// releaseAdoptedEntry undoes what Terraform owned on an adopted entry, and leaves the
// entry itself to whatever syncs it: it clears the custom aliases, if Terraform owned
// them, and the managed attributes.
func (r *IncidentCatalogEntryResource) releaseAdoptedEntry(ctx context.Context, data *IncidentCatalogEntryResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	managedAttributeIDs, attrDiags := data.managedAttributeIDs(ctx)
	diags.Append(attrDiags...)
	if diags.HasError() {
		return diags
	}

	var customAliases *[]string
	if !data.CustomAliases.IsNull() {
		customAliases = &[]string{}
	}

	if customAliases == nil && len(managedAttributeIDs) == 0 {
		return diags
	}

	current, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.ID.ValueString(), nil)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			// The sync has already removed the entry, so there's nothing left to clear.
			return diags
		}
		diags.AddError("Client Error", fmt.Sprintf("Unable to read adopted catalog entry, got error: %s", err))
		return diags
	}

	entry := current.JSON200.CatalogEntry
	_, err = r.client.CatalogV3UpdateEntryWithResponse(ctx, entry.Id, client.CatalogUpdateEntryPayloadV3{
		Name:             entry.Name,
		Rank:             lo.ToPtr(entry.Rank),
		CustomAliases:    customAliases,
		AttributeValues:  map[string]client.CatalogEngineParamBindingPayloadV3{},
		UpdateAttributes: &managedAttributeIDs,
	})
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to clear what Terraform owned on adopted catalog entry, got error: %s", err))
		return diags
	}

	tflog.Info(ctx, fmt.Sprintf("Released adopted catalog entry %s instead of deleting it", entry.Id))
	return diags
}

// ModifyPlan marks aliases as unknown when an adopted entry's custom aliases change, as
// the API lists custom aliases in aliases too.
func (r *IncidentCatalogEntryResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Creates already plan aliases as unknown, and destroys have nothing to plan.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var identifier types.String
	var planned, prior types.Set
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("existing_entry_identifier"), &identifier)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("custom_aliases"), &planned)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("custom_aliases"), &prior)...)
	if resp.Diagnostics.HasError() || identifier.IsNull() {
		return
	}

	// Leaving custom_aliases out leaves them alone, so aliases can't change.
	if planned.IsNull() || planned.Equal(prior) {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("aliases"), types.ListUnknown(types.StringType))...)
}

func (r *IncidentCatalogEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentCatalogEntryResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data IncidentCatalogEntryResourceModel

	resp.Diagnostics.Append(validateCatalogEntryMode(ctx, req.Config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var attributeValues types.Set
	diag := req.Config.GetAttribute(ctx, path.Root("attribute_values"), &attributeValues)
	if diag.HasError() || attributeValues.IsUnknown() {
		// If attribute_values is unknown, don't attempt to validate the managed
		// attributes. We have to return early here because the call to req.Config.Get
		// fails to marshal into the []CatalogEntryAttributeValue in this case.
		return
	}

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If managed_attributes is not set, all attributes are valid, unless the entry is
	// adopted, when Terraform manages none.
	if data.ManagedAttributes.IsUnknown() || data.ExistingEntryIdentifier.IsUnknown() {
		return
	}
	if data.ManagedAttributes.IsNull() && !data.adopts() {
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
			detail := fmt.Sprintf("Attribute ID %q is specified in attribute_values but is not in the managed_attributes set. "+
				"Either add it to managed_attributes or remove it from attribute_values.", attributeID)
			if data.ManagedAttributes.IsNull() {
				detail = fmt.Sprintf("Attribute ID %q is specified in attribute_values, but an entry adopted with existing_entry_identifier "+
					"only manages the attributes listed in managed_attributes. Add it to managed_attributes.", attributeID)
			}
			resp.Diagnostics.AddAttributeError(
				path.Root("attribute_values").AtListIndex(idx),
				"Unmanaged Attribute",
				detail,
			)
		}
	}
}

// isAttributeManaged checks if the given attribute should be managed by this resource.
func (m *IncidentCatalogEntryResourceModel) isAttributeManaged(attributeID string) bool {
	// If managedAttributes is not set, all attributes are managed, unless the entry is
	// adopted, when none are.
	if m.ManagedAttributes.IsNull() || m.ManagedAttributes.IsUnknown() {
		return !m.adopts()
	}

	// Extract the managed attribute IDs
	var managedAttributeIDs []string
	diags := m.ManagedAttributes.ElementsAs(context.Background(), &managedAttributeIDs, false)
	if diags.HasError() {
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

// managedAttributeIDs returns the attributes Terraform manages on an adopted entry, which
// is none when managed_attributes is unset.
func (m *IncidentCatalogEntryResourceModel) managedAttributeIDs(ctx context.Context) ([]string, diag.Diagnostics) {
	attributeIDs := []string{}
	if m.ManagedAttributes.IsNull() || m.ManagedAttributes.IsUnknown() {
		return attributeIDs, nil
	}

	diags := m.ManagedAttributes.ElementsAs(ctx, &attributeIDs, false)
	return attributeIDs, diags
}

// validateCatalogEntryMode checks the config fits the resource's mode: creating an entry
// needs a name and attribute values, while adopting one leaves the fields its sync owns
// alone.
func validateCatalogEntryMode(ctx context.Context, config tfsdk.Config) diag.Diagnostics {
	var diags diag.Diagnostics

	var identifier types.String
	diags.Append(config.GetAttribute(ctx, path.Root("existing_entry_identifier"), &identifier)...)
	if diags.HasError() || identifier.IsUnknown() {
		return diags
	}

	if identifier.IsNull() {
		var name types.String
		var attributeValues, customAliases types.Set
		diags.Append(config.GetAttribute(ctx, path.Root("name"), &name)...)
		diags.Append(config.GetAttribute(ctx, path.Root("attribute_values"), &attributeValues)...)
		diags.Append(config.GetAttribute(ctx, path.Root("custom_aliases"), &customAliases)...)
		if diags.HasError() {
			return diags
		}

		if name.IsNull() {
			diags.AddAttributeError(path.Root("name"), "Missing required argument",
				"name is required, unless existing_entry_identifier is set to adopt an entry that already exists.")
		}
		if attributeValues.IsNull() {
			diags.AddAttributeError(path.Root("attribute_values"), "Missing required argument",
				"attribute_values is required, unless existing_entry_identifier is set to adopt an entry that already exists.")
		}
		if !customAliases.IsNull() {
			diags.AddAttributeError(path.Root("custom_aliases"), "Invalid argument",
				"custom_aliases can only be set with existing_entry_identifier, as only entries that incident.io syncs take custom aliases. "+
					"On an entry Terraform creates, set aliases instead.")
		}

		return diags
	}

	// The sync owns these on an adopted entry, so Terraform only reads them.
	var name, externalID types.String
	var aliases types.List
	var rank types.Int64
	diags.Append(config.GetAttribute(ctx, path.Root("name"), &name)...)
	diags.Append(config.GetAttribute(ctx, path.Root("external_id"), &externalID)...)
	diags.Append(config.GetAttribute(ctx, path.Root("aliases"), &aliases)...)
	diags.Append(config.GetAttribute(ctx, path.Root("rank"), &rank)...)
	if diags.HasError() {
		return diags
	}

	for _, field := range []struct {
		attribute string
		value     attr.Value
	}{
		{"name", name},
		{"external_id", externalID},
		{"aliases", aliases},
		{"rank", rank},
	} {
		if !field.value.IsNull() {
			attribute := field.attribute
			diags.AddAttributeError(path.Root(attribute), "Invalid argument",
				fmt.Sprintf("%s can't be set with existing_entry_identifier: whatever syncs the adopted entry owns it, so Terraform only reads it. "+
					"Use custom_aliases to add aliases, and attribute_values with managed_attributes to set your own attributes.", attribute))
		}
	}

	return diags
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

	// An adopted entry with no managed attributes needs no attribute_values, so keep them
	// unset rather than reading back an empty set.
	if data.adopts() && data.AttributeValues == nil && len(values) == 0 {
		values = nil
	}

	// Terraform only tracks custom aliases when it owns them.
	customAliases := types.SetNull(types.StringType)
	if !data.CustomAliases.IsNull() {
		customAliases = types.SetValueMust(types.StringType, lo.Map(entry.CustomAliases, func(alias string, _ int) attr.Value {
			return types.StringValue(alias)
		}))
	}

	return &IncidentCatalogEntryResourceModel{
		ID:              types.StringValue(entry.Id),
		CatalogTypeID:   types.StringValue(entry.CatalogTypeId),
		Name:            types.StringValue(entry.Name),
		ExternalID:      types.StringPointerValue(entry.ExternalId),
		Aliases:         types.ListValueMust(types.StringType, aliases),
		CustomAliases:   customAliases,
		Rank:            types.Int64Value(int64(entry.Rank)),
		AttributeValues: values,
		// These are managed in config only
		ExistingEntryIdentifier: data.ExistingEntryIdentifier,
		ManagedAttributes:       data.ManagedAttributes,
	}
}
