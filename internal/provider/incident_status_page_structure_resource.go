package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ resource.Resource                   = &IncidentStatusPageStructureResource{}
	_ resource.ResourceWithConfigure      = &IncidentStatusPageStructureResource{}
	_ resource.ResourceWithImportState    = &IncidentStatusPageStructureResource{}
	_ resource.ResourceWithValidateConfig = &IncidentStatusPageStructureResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentStatusPageStructureResource{}
)

type IncidentStatusPageStructureResource struct {
	resourceConfigurer
}

func NewIncidentStatusPageStructureResource() resource.Resource {
	return &IncidentStatusPageStructureResource{}
}

func (r *IncidentStatusPageStructureResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_structure"
}

func (r *IncidentStatusPageStructureResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	// The flags on a placed component, set the same way on its own and inside a group.
	componentDisplayAttributes := func() map[string]schema.Attribute {
		return map[string]schema.Attribute{
			"hidden": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether the component is hidden from the page. Left out, a component already " +
					"on the page keeps its setting and a new one is shown.",
			},
			"display_uptime": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether the page shows the component's uptime. Left out, a component already " +
					"on the page keeps its setting and a new one shows it.",
			},
		}
	}
	groupComponentAttributes := componentDisplayAttributes()
	groupComponentAttributes["component_id"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: apischema.Docstring("StatusPageStructureComponentPayloadV2", "component_id"),
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
	}
	itemAttributes := componentDisplayAttributes()
	itemAttributes["component_id"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: apischema.Docstring("StatusPageStructureComponentPayloadV2", "component_id") + ", shown on its own.",
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
	}
	itemAttributes["group"] = schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "A named group of components.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("StatusPageStructureGroupV2", "id") + ". Assigned by incident.io, and kept while the group keeps its name.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("StatusPageStructureGroupPayloadV2", "name"),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "A description shown under the group's name. Left out, the group has none, " +
					"so a description set in the dashboard has to be written here to survive an apply.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"hidden": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether the group is hidden from the page. Left out, a group already on the " +
					"page keeps its setting and a new one is hidden only when every component in it is. " +
					"A visible group needs a visible component.",
			},
			"display_aggregated_uptime": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether the page shows uptime aggregated across the group's components. Left " +
					"out, a group already on the page keeps its setting and a new one shows it when a " +
					"component allows. It needs a visible component that shows uptime.",
			},
			"components": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "The components in this group, in display order.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: groupComponentAttributes,
				},
			},
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the structure of a standalone status page: which components it shows, in what " +
			"order, how they are grouped, and how each is displayed. The page itself is created in the incident.io " +
			"dashboard; look it up with the `incident_status_page` data source. Components come from " +
			"`incident_status_page_component`.\n\n" +
			"A group keeps its ID across applies as long as its name does, so links to it stay valid. Renaming a " +
			"group creates a new one. Display settings left out of the configuration keep whatever the page already " +
			"has, so a structure adopted from the dashboard keeps its hidden components and uptime settings until " +
			"the configuration says otherwise.\n\n" +
			"Terraform claims the structure, so it cannot be edited in the dashboard; set `unlock_in_dashboard` to " +
			"leave it editable there. Destroying this resource leaves the page's structure in place, since a page " +
			"always has one; it only stops Terraform managing it. The API key needs the \"Configure status pages\" " +
			"permission. Parent and customer pages build their structure from the catalog and cannot be managed here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the status page, which has exactly one structure.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status_page_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the status page whose structure this is. Changing it manages a different page's structure.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_uptime_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "How the page shows uptime against its components: `chart_and_percentage`, " +
					"`chart_only` or `nothing`. Left out, the page keeps its current setting.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(client.StatusPagesSetStatusPageStructurePayloadV2DisplayUptimeModeChartAndPercentage),
						string(client.StatusPagesSetStatusPageStructurePayloadV2DisplayUptimeModeChartOnly),
						string(client.StatusPagesSetStatusPageStructurePayloadV2DisplayUptimeModeNothing),
					),
				},
			},
			"items": schema.ListNestedAttribute{
				Required: true,
				MarkdownDescription: "The components and groups the page shows, in display order. Each item is either a " +
					"single `component_id`, with its own `hidden` and `display_uptime`, or a `group`, not both.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: itemAttributes,
				},
			},
			"unlock_in_dashboard": unlockInDashboardAttribute(),
		},
	}
}

// ValidateConfig checks each item is one thing: a component or a group. Both attributes are
// optional so either shape fits, which means a plan could otherwise carry both or neither,
// or a group with a component's display flags beside it.
func (r *IncidentStatusPageStructureResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data *models.StatusPageStructureModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	for idx, item := range data.Items {
		// A component id from a resource created in the same apply is unknown, not null,
		// so an unknown counts as set.
		hasComponent := !item.ComponentID.IsNull()
		hasGroup := item.Group != nil

		switch {
		case hasComponent && hasGroup:
			resp.Diagnostics.AddAttributeError(path.Root("items").AtListIndex(idx), "Ambiguous item",
				"Set either component_id or group on an item, not both.")
		case !hasComponent && !hasGroup:
			resp.Diagnostics.AddAttributeError(path.Root("items").AtListIndex(idx), "Empty item",
				"Set one of component_id or group on each item.")
		case hasGroup && (!item.Hidden.IsNull() || !item.DisplayUptime.IsNull()):
			resp.Diagnostics.AddAttributeError(path.Root("items").AtListIndex(idx), "Display flags on a group item",
				"hidden and display_uptime belong to a component. Set them on the group's components, or set the group's own hidden and display_aggregated_uptime.")
		}
	}
}

// ModifyPlan carries display settings the configuration leaves out from the prior state,
// matched by component and group rather than by position, so a reorder plans no change to
// them and the plan shows the settings an apply will keep. A create has no prior state, and
// a change of page is a replacement whose prior state belongs to another page.
func (r *IncidentStatusPageStructureResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan, state models.StatusPageStructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.StatusPageID.Equal(state.StatusPageID) {
		return
	}

	plan.CarryDisplaySettings(state)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *IncidentStatusPageStructureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config models.StatusPageStructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.set(ctx, config, plan.UnlockInDashboard)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set status page structure, got error: %s", err))
		return
	}

	// A create unclaims as well as claims: a destroy leaves the claim on the page, so a
	// structure managed again with unlock_in_dashboard would otherwise stay locked.
	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, state.StatusPageID.ValueString(), &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageStructure, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, state.StatusPageID.ValueString(), &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageStructure)
	}

	tflog.Trace(ctx, fmt.Sprintf("set the structure of status page id=%s", state.StatusPageID.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *IncidentStatusPageStructureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.StatusPageStructureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.show(ctx, data.StatusPageID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Status page with ID %s not found: removing its structure from state.", data.StatusPageID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read status page structure, got error: %s", err))
		return
	}

	state := models.StatusPageStructureModel{}.FromAPI(
		data.StatusPageID.ValueString(), current.CurrentStructure, string(current.DisplayUptimeMode), data.UnlockInDashboard)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentStatusPageStructureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config models.StatusPageStructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.set(ctx, config, plan.UnlockInDashboard)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set status page structure, got error: %s", err))
		return
	}

	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, state.StatusPageID.ValueString(), &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageStructure, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, state.StatusPageID.ValueString(), &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageStructure)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Delete only forgets the structure. A page always has one, and an empty structure is
// not valid, so there is nothing to remove on the incident.io side. The claim stays too:
// the structure is still what Terraform last applied, and whoever takes it over in the
// dashboard disconnects it there.
func (r *IncidentStatusPageStructureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.StatusPageStructureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Leaving the structure of status page id=%s in place: a page always has one.", data.StatusPageID.ValueString()))
}

// ImportState takes the status page's ID, since that is the structure's own id.
func (r *IncidentStatusPageStructureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeStatusPageStructure, r.terraformVersion,
		r.markImportedAsManaged)

	current, err := r.show(ctx, req.ID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError("Status Page Not Found", fmt.Sprintf("No status page with ID %q exists.", req.ID))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read status page structure, got error: %s", err))
		return
	}

	state := models.StatusPageStructureModel{}.FromAPI(req.ID, current.CurrentStructure, string(current.DisplayUptimeMode), types.BoolNull())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// set replaces the page's structure with the configured one. The current structure is read
// first so a group the configuration names the same keeps its ID.
func (r *IncidentStatusPageStructureResource) set(ctx context.Context, config models.StatusPageStructureModel, unlockInDashboard types.Bool) (*models.StatusPageStructureModel, error) {
	statusPageID := config.StatusPageID.ValueString()

	current, err := r.show(ctx, statusPageID)
	if err != nil {
		return nil, err
	}

	result, err := r.client.StatusPagesV2SetStatusPageStructureWithResponse(ctx, statusPageID, config.ToPayload(current.CurrentStructure))
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	state := models.StatusPageStructureModel{}.FromAPI(
		statusPageID, result.JSON200.CurrentStructure, string(result.JSON200.DisplayUptimeMode), unlockInDashboard)

	return &state, nil
}

func (r *IncidentStatusPageStructureResource) show(ctx context.Context, statusPageID string) (*client.StatusPagesShowStatusPageStructureResultV2, error) {
	result, err := r.client.StatusPagesV2ShowStatusPageStructureWithResponse(ctx, statusPageID)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return result.JSON200, nil
}
