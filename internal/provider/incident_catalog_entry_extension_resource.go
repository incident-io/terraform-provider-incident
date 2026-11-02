package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var _ resource.Resource = &IncidentCatalogEntryExtensionResource{}

type IncidentCatalogEntryExtensionResource struct {
	resourceConfigurer
}

type IncidentCatalogEntryExtensionResourceModel struct {
	ID              types.String                 `tfsdk:"id"`
	CatalogTypeID   types.String                 `tfsdk:"catalog_type_id"`
	Identifier      types.String                 `tfsdk:"identifier"`
	CustomAliases   types.Set                    `tfsdk:"custom_aliases"`
	AttributeValues []CatalogEntryAttributeValue `tfsdk:"attribute_values"`
}

// attributeIDs lists the attributes this resource owns: the ones in attribute_values.
func (m IncidentCatalogEntryExtensionResourceModel) attributeIDs() []string {
	return lo.Map(m.AttributeValues, func(value CatalogEntryAttributeValue, _ int) string {
		return value.Attribute.ValueString()
	})
}

func NewIncidentCatalogEntryExtensionResource() resource.Resource {
	return &IncidentCatalogEntryExtensionResource{}
}

func (r *IncidentCatalogEntryExtensionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_entry_extension"
}

func (r *IncidentCatalogEntryExtensionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `
This resource extends a catalog entry that already exists, usually one that incident.io
syncs, such as a schedule, escalation path or user. The sync creates and deletes those
entries, and owns their name, external ID, aliases and synced attributes, so you can't
manage them with ` + "`incident_catalog_entry`" + `.

Terraform finds the entry rather than creating it, and writes only:

- ` + "`custom_aliases`" + `: aliases your organisation adds to the entry, which syncs keep.
- ` + "`attribute_values`" + `: attributes you added to the catalog type yourself, for example in
  the incident.io web UI. Terraform doesn't touch any attribute you don't list.

Terraform never creates or deletes the entry. If no entry matches ` + "`identifier`" + `, it fails.
When you destroy the resource, or stop setting something, Terraform clears what it wrote
and leaves the entry in place. Nothing stops people editing the entry in the dashboard:
Terraform puts back what it owns on the next apply.

Changing ` + "`custom_aliases`" + ` on the User type, or on another type that team membership passes
through, needs an API key with the ` + "`team.memberships_update`" + ` scope ("Update team
memberships"), because aliases on those entries decide who belongs to a team.
		`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the catalog entry this resource extends.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"catalog_type_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the catalog type the entry belongs to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"identifier": schema.StringAttribute{
				MarkdownDescription: "Finds the entry to extend: its ID, external ID or one of its aliases (or its name, if the type uses names as identifiers). For a schedule or escalation path, use its ID. Terraform fails if no entry, or more than one, matches.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"custom_aliases": schema.SetAttribute{
				ElementType: types.StringType,
				MarkdownDescription: `Aliases your organisation adds to the entry. incident.io syncs keep them, and the entry lists them in its aliases too. Only catalog types that incident.io syncs take custom aliases.

When set, Terraform owns the entry's whole custom alias list: it removes custom aliases added elsewhere. Set it to ` + "`[]`" + ` to clear them. Leave it out of a new resource to leave them as they are.`,
				Optional: true,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						// The API trims aliases and drops empty ones, which would make the
						// applied value differ from the config.
						stringvalidator.RegexMatches(regexp.MustCompile(`^\S(.*\S)?$`), "must not be empty, or start or end with whitespace"),
					),
				},
			},
			"attribute_values": schema.SetNestedAttribute{
				MarkdownDescription: "Values for attributes you added to the catalog type. Terraform owns only the attributes listed here, and clears an attribute when you remove it.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"attribute": schema.StringAttribute{
							Description: `The ID of this attribute, usually loaded from the incident_catalog_type_attribute resource or data source.`,
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

func (r *IncidentCatalogEntryExtensionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogEntryExtensionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entry, catalogType, diags := r.findEntry(ctx, data.CatalogTypeID.ValueString(), data.Identifier.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	entry, diags = r.writeEntry(ctx, *entry, catalogType, data, data.CustomAliases.IsNull(), data.attributeIDs())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("extended catalog entry with id=%s", entry.Id))
	resp.Diagnostics.Append(resp.State.Set(ctx, r.buildModel(ctx, *entry, data))...)
}

func (r *IncidentCatalogEntryExtensionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogEntryExtensionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.ID.ValueString(), nil)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Catalog entry with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, r.buildModel(ctx, result.JSON200.CatalogEntry, data))...)
}

func (r *IncidentCatalogEntryExtensionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state *IncidentCatalogEntryExtensionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read the entry fresh, so we send back the name and rank it has now, not the ones
	// from the last refresh.
	current, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}

	// Also write the attributes and custom aliases Terraform no longer sets, which clears
	// them.
	leaveCustomAliases := data.CustomAliases.IsNull() && state.CustomAliases.IsNull()
	updateAttributes := lo.Union(state.attributeIDs(), data.attributeIDs())

	entry, diags := r.writeEntry(ctx, current.JSON200.CatalogEntry, current.JSON200.CatalogType, data, leaveCustomAliases, updateAttributes)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, r.buildModel(ctx, *entry, data))...)
}

func (r *IncidentCatalogEntryExtensionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogEntryExtensionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	leaveCustomAliases := data.CustomAliases.IsNull()
	updateAttributes := data.attributeIDs()
	if leaveCustomAliases && len(updateAttributes) == 0 {
		return
	}

	current, err := r.client.CatalogV3ShowEntryWithResponse(ctx, data.ID.ValueString(), nil)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			// The sync has already removed the entry, so there's nothing left to clear.
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return
	}

	// Clearing what Terraform wrote is the same write with nothing set.
	cleared := &IncidentCatalogEntryExtensionResourceModel{
		CustomAliases: types.SetValueMust(types.StringType, nil),
	}
	_, diags := r.writeEntry(ctx, current.JSON200.CatalogEntry, current.JSON200.CatalogType, cleared, leaveCustomAliases, updateAttributes)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Cleared what Terraform wrote on catalog entry %s, and left the entry in place", data.ID.ValueString()))
}

// findEntry finds the entry that identifier points at. It never creates one: the entry
// belongs to whatever syncs it, so a missing entry is an error the user has to fix, for
// example by waiting for the sync.
func (r *IncidentCatalogEntryExtensionResource) findEntry(ctx context.Context, catalogTypeID, identifier string) (*client.CatalogEntryV3, client.CatalogTypeV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	result, err := r.client.CatalogV3ListEntriesWithResponse(ctx, &client.CatalogV3ListEntriesParams{
		CatalogTypeId: catalogTypeID,
		Identifier:    &identifier,
		// Two is enough to tell a unique match from an ambiguous one.
		PageSize: 2,
	})
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to find the catalog entry to extend, got error: %s", err))
		return nil, client.CatalogTypeV3{}, diags
	}

	entries := result.JSON200.CatalogEntries
	catalogType := result.JSON200.CatalogType
	switch {
	case len(entries) == 0:
		diags.AddAttributeError(
			path.Root("identifier"),
			"Catalog entry not found",
			fmt.Sprintf("No entry in the %q catalog type has the identifier %q, so there is nothing to extend. "+
				"Terraform doesn't create the entries it extends: check the identifier, and that the entry exists (for example, that its sync has run).",
				catalogType.Name, identifier),
		)
		return nil, catalogType, diags
	case len(entries) > 1:
		diags.AddAttributeError(
			path.Root("identifier"),
			"Catalog entry identifier is ambiguous",
			fmt.Sprintf("More than one entry in the %q catalog type has the identifier %q. Use the entry's ID or external ID instead.",
				catalogType.Name, identifier),
		)
		return nil, catalogType, diags
	}

	return &entries[0], catalogType, diags
}

// writeEntry writes the custom aliases, unless leaveCustomAliases, and the attributes in
// updateAttributes, taking their values from data: an attribute with no value there is
// cleared. It sends back the entry's current name and rank, which an update always
// writes, and leaves out aliases and the external ID, so the API keeps the synced ones.
func (r *IncidentCatalogEntryExtensionResource) writeEntry(ctx context.Context, entry client.CatalogEntryV3, catalogType client.CatalogTypeV3, data *IncidentCatalogEntryExtensionResourceModel, leaveCustomAliases bool, updateAttributes []string) (*client.CatalogEntryV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	var customAliases *[]string
	if !leaveCustomAliases {
		if !catalogType.SupportsCustomAliases {
			diags.AddAttributeError(
				path.Root("custom_aliases"),
				"Catalog type doesn't take custom aliases",
				fmt.Sprintf("Entries of the %q catalog type don't take custom aliases. Only catalog types that incident.io syncs do, "+
					"and not those whose aliases you edit elsewhere, such as alert priorities.", catalogType.Name),
			)
			return nil, diags
		}

		// A null set means Terraform stopped setting them, so clear them.
		aliases := []string{}
		if !data.CustomAliases.IsNull() {
			diags.Append(data.CustomAliases.ElementsAs(ctx, &aliases, false)...)
			if diags.HasError() {
				return nil, diags
			}
		}
		customAliases = &aliases
	}

	// Nothing to write, so don't touch the entry.
	if customAliases == nil && len(updateAttributes) == 0 {
		return &entry, diags
	}

	attributeValues, valueDiags := buildCatalogEntryAttributeValues(data.AttributeValues, func(string) bool { return true })
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
		// key can set, which would clear ones Terraform doesn't own.
		UpdateAttributes: &updateAttributes,
	})
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to update catalog entry, got error: %s", err))
		return nil, diags
	}

	return &result.JSON200.CatalogEntry, diags
}

// buildModel reads back only what this resource owns: the custom aliases, if it sets
// them, and the attributes listed in data.
func (r *IncidentCatalogEntryExtensionResource) buildModel(ctx context.Context, entry client.CatalogEntryV3, data *IncidentCatalogEntryExtensionResourceModel) *IncidentCatalogEntryExtensionResourceModel {
	customAliases := types.SetNull(types.StringType)
	if !data.CustomAliases.IsNull() {
		customAliases, _ = types.SetValueFrom(ctx, types.StringType, lo.Ternary(entry.CustomAliases == nil, []string{}, entry.CustomAliases))
	}

	var attributeValues []CatalogEntryAttributeValue
	if data.AttributeValues != nil {
		owned := data.attributeIDs()
		attributeValues = catalogEntryAttributeValuesFromAPI(entry.AttributeValues, func(attributeID string) bool {
			return lo.Contains(owned, attributeID)
		})
	}

	return &IncidentCatalogEntryExtensionResourceModel{
		ID:              types.StringValue(entry.Id),
		CatalogTypeID:   data.CatalogTypeID,
		Identifier:      data.Identifier,
		CustomAliases:   customAliases,
		AttributeValues: attributeValues,
	}
}
