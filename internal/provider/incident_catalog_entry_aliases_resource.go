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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var (
	_ resource.Resource                = &IncidentCatalogEntryAliasesResource{}
	_ resource.ResourceWithImportState = &IncidentCatalogEntryAliasesResource{}
)

type IncidentCatalogEntryAliasesResource struct {
	resourceConfigurer
}

type IncidentCatalogEntryAliasesResourceModel struct {
	EntryID types.String `tfsdk:"entry_id"`
	Aliases types.Set    `tfsdk:"aliases"`
}

func NewIncidentCatalogEntryAliasesResource() resource.Resource {
	return &IncidentCatalogEntryAliasesResource{}
}

func (r *IncidentCatalogEntryAliasesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_entry_aliases"
}

func (r *IncidentCatalogEntryAliasesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `
This resource adds your own aliases to a catalog entry that incident.io syncs, such as a
schedule, escalation path or user. The sync owns the entry and its other aliases, so you
can't manage it with ` + "`incident_catalog_entry`" + `: look it up with the
` + "`incident_catalog_entry`" + ` data source instead.

Syncs keep these aliases, and the entry lists them in its aliases too. Terraform owns the
whole list: it removes aliases added to it elsewhere, for example in the dashboard, on the
next apply. When you destroy the resource, Terraform clears the list and leaves the entry
in place.

Only catalog types that incident.io syncs take these aliases. Changing them on the User
type, or on another type that team membership passes through, needs an API key with the
` + "`team.memberships_update`" + ` scope ("Update team memberships"), because aliases on those
entries decide who belongs to a team.
		`,
		Attributes: map[string]schema.Attribute{
			"entry_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the catalog entry, usually from the `incident_catalog_entry` data source.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"aliases": schema.SetAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "Aliases your organisation adds to the entry, beside the ones its sync sets.",
				Required:            true,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						// The API trims aliases and drops empty ones, which would make the
						// applied value differ from the config.
						stringvalidator.RegexMatches(regexp.MustCompile(`^\S(.*\S)?$`), "must not be empty, or start or end with whitespace"),
					),
				},
			},
		},
	}
}

func (r *IncidentCatalogEntryAliasesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentCatalogEntryAliasesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.write(ctx, data, &resp.State, &resp.Diagnostics)
}

func (r *IncidentCatalogEntryAliasesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentCatalogEntryAliasesResourceModel
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

	resp.Diagnostics.Append(resp.State.Set(ctx, r.buildModel(ctx, result.JSON200.CatalogEntry))...)
}

func (r *IncidentCatalogEntryAliasesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentCatalogEntryAliasesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.write(ctx, data, &resp.State, &resp.Diagnostics)
}

func (r *IncidentCatalogEntryAliasesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentCatalogEntryAliasesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the sync has already removed the entry, there's nothing left to clear.
	_, diags := writeCatalogEntryPartially(ctx, r.client, data.EntryID.ValueString(), catalogEntryPartialWrite{
		CustomAliases: &[]string{},
	})
	resp.Diagnostics.Append(diags...)
}

func (r *IncidentCatalogEntryAliasesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("entry_id"), req, resp)
}

func (r *IncidentCatalogEntryAliasesResource) write(ctx context.Context, data *IncidentCatalogEntryAliasesResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	aliases := []string{}
	diags.Append(data.Aliases.ElementsAs(ctx, &aliases, false)...)
	if diags.HasError() {
		return
	}

	entry, writeDiags := writeCatalogEntryPartially(ctx, r.client, data.EntryID.ValueString(), catalogEntryPartialWrite{
		CustomAliases: &aliases,
	})
	diags.Append(writeDiags...)
	if diags.HasError() {
		return
	}
	if entry == nil {
		diags.AddAttributeError(path.Root("entry_id"), "Catalog entry not found",
			fmt.Sprintf("No catalog entry has the ID %q. Terraform doesn't create the entries it adds aliases to.", data.EntryID.ValueString()))
		return
	}

	diags.Append(state.Set(ctx, r.buildModel(ctx, *entry))...)
}

func (r *IncidentCatalogEntryAliasesResource) buildModel(ctx context.Context, entry client.CatalogEntryV3) *IncidentCatalogEntryAliasesResourceModel {
	aliases, _ := types.SetValueFrom(ctx, types.StringType, lo.Ternary(entry.CustomAliases == nil, []string{}, entry.CustomAliases))

	return &IncidentCatalogEntryAliasesResourceModel{
		EntryID: types.StringValue(entry.Id),
		Aliases: aliases,
	}
}
