package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

var (
	_ resource.Resource                   = &IncidentStatusResource{}
	_ resource.ResourceWithImportState    = &IncidentStatusResource{}
	_ resource.ResourceWithValidateConfig = &IncidentStatusResource{}
)

// maxIncidentStatusRank is the highest rank the API sorts correctly, which it documents
// on the payload.
const maxIncidentStatusRank = 999999

type IncidentStatusResource struct {
	resourceConfigurer
}

type IncidentStatusResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Category    types.String `tfsdk:"category"`
	Rank        types.Int64  `tfsdk:"rank"`
}

func NewIncidentStatusResource() resource.Resource {
	return &IncidentStatusResource{}
}

func (r *IncidentStatusResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status"
}

func (r *IncidentStatusResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Incident Statuses V1"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentStatusV1", "description"),
				Required:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("IncidentStatusV1", "category") +
					" Changing it replaces the status, as a status can't move between " +
					"categories. Set `rank` if you want the replacement to keep its place.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// Optional but not Computed, so leaving it out means Terraform doesn't manage
			// the order. Computed would adopt whatever rank the API assigned and then
			// re-assert it, fighting anyone who reorders the lifecycle in the dashboard.
			"rank": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "Where this status sits within its category, lowest rank " +
					"first. Set it on every status in a category to fix their order: without it " +
					"each status lands after the last one created, which for a single apply " +
					"means whichever order Terraform happened to create them in. No two statuses " +
					"in a category can share a rank, and ranks needn't be consecutive — leaving " +
					"gaps (10, 20, 30) means you can insert a status between two others without " +
					"renumbering them. Because a rank belongs to one status at a time, swapping " +
					"a pair of them in a single apply fails: move a status to a rank nothing " +
					"holds instead.",
			},
		},
	}
}

// ValidateConfig mirrors the window the API accepts, so a rank it would reject fails at
// plan time rather than part-way through an apply.
func (r *IncidentStatusResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	rankPath := path.Root("rank")

	var rank types.Int64
	diags := req.Config.GetAttribute(ctx, rankPath, &rank)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || rank.IsNull() || rank.IsUnknown() {
		return
	}

	if value := rank.ValueInt64(); value < 0 || value > maxIncidentStatusRank {
		resp.Diagnostics.AddAttributeError(
			rankPath,
			"Invalid rank",
			fmt.Sprintf("Rank runs from 0 to %d, so %d isn't a position.", maxIncidentStatusRank, value),
		)
	}
}

func (r *IncidentStatusResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentStatusesV1CreateWithResponse(ctx, client.IncidentStatusesV1CreateJSONRequestBody{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Category:    client.IncidentStatusesCreatePayloadV1Category(data.Category.ValueString()),
		// Omitted when the config leaves rank out, which is what has the API append this
		// status to its category.
		Rank: data.Rank.ValueInt64Pointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create incident status, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an incident status resource with id=%s", result.JSON201.IncidentStatus.Id))
	data = r.buildModel(result.JSON201.IncidentStatus, data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentStatusResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentStatusesV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Incident status with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident status, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.IncidentStatus, data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentStatusResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentStatusesV1UpdateWithResponse(ctx, data.ID.ValueString(), client.IncidentStatusesV1UpdateJSONRequestBody{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		// Omitted when the config leaves rank out, which leaves the status where it is —
		// including wherever someone has since moved it in the dashboard.
		Rank: data.Rank.ValueInt64Pointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update incident status, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.IncidentStatus, data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentStatusResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentStatusResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.IncidentStatusesV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete incident status, got error: %s", err))
		return
	}
}

func (r *IncidentStatusResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildModel turns an API status into state. prior is the plan or state we were working
// from, which decides whether rank is ours to track: every status has one, but adopting
// it for a config that never asked would both break the plan (a null rank planned, a
// number applied) and start a fight with anyone reordering in the dashboard.
//
// On an import prior holds only the ID, so an imported status arrives with no rank. That
// resolves itself on the first apply, once there's a config to compare against.
func (r *IncidentStatusResource) buildModel(status client.IncidentStatusV1, prior *IncidentStatusResourceModel) *IncidentStatusResourceModel {
	rank := types.Int64Null()
	if prior != nil && !prior.Rank.IsNull() {
		rank = types.Int64Value(status.Rank)
	}

	return &IncidentStatusResourceModel{
		ID:          types.StringValue(status.Id),
		Name:        types.StringValue(status.Name),
		Description: types.StringValue(status.Description),
		Category:    types.StringValue(string(status.Category)),
		Rank:        rank,
	}
}
