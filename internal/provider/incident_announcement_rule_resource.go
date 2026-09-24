package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ resource.Resource                = &IncidentAnnouncementRuleResource{}
	_ resource.ResourceWithConfigure   = &IncidentAnnouncementRuleResource{}
	_ resource.ResourceWithImportState = &IncidentAnnouncementRuleResource{}
)

type IncidentAnnouncementRuleResource struct {
	resourceConfigurer
}

func NewIncidentAnnouncementRuleResource() resource.Resource {
	return &IncidentAnnouncementRuleResource{}
}

func (r *IncidentAnnouncementRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_announcement_rule"
}

func (r *IncidentAnnouncementRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	emptySet := setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{}))

	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Announcement Rules V2"),
			"Set `slack_channel_ids` when your organisation uses Slack, or `microsoft_teams_channel_ids` "+
				"when it uses Microsoft Teams.\n\nAn API key can only manage a rule whose "+
				"`private_incident_scope` is `none`: announcing private incidents needs a permission no API "+
				"key role grants, so a rule scoped to `all` or `owning_teams` has to be managed in the dashboard."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "name"),
			},
			"condition_groups": models.ConditionGroupsAttribute(),
			"slack_channel_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             emptySet,
				MarkdownDescription: apischema.Docstring("AnnouncementRulesCreatePayloadV2", "slack_channel_ids"),
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"microsoft_teams_channel_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             emptySet,
				MarkdownDescription: apischema.Docstring("AnnouncementRulesCreatePayloadV2", "microsoft_teams_channel_ids"),
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"update_sharing_mode": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: EnumValuesDescription("AnnouncementRuleV2", "update_sharing_mode"),
				Validators: []validator.String{
					stringvalidator.OneOf(enumValues("AnnouncementRuleV2", "update_sharing_mode")...),
				},
			},
			"mode": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: EnumValuesDescription("AnnouncementRuleV2", "mode"),
				Validators: []validator.String{
					stringvalidator.OneOf(enumValues("AnnouncementRuleV2", "mode")...),
				},
			},
			"private_incident_scope": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: DescribeEnumValues(
					apischema.Docstring("AnnouncementRuleV2", "private_incident_scope")+". Defaults to `none`.",
					"AnnouncementRuleV2", "private_incident_scope"),
				Validators: []validator.String{
					stringvalidator.OneOf(enumValues("AnnouncementRuleV2", "private_incident_scope")...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"conditions_no_longer_apply_behaviour": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: DescribeEnumValues(
					apischema.Docstring("AnnouncementRuleV2", "conditions_no_longer_apply_behaviour")+". Defaults to `leave_in_place`.",
					"AnnouncementRuleV2", "conditions_no_longer_apply_behaviour"),
				Validators: []validator.String{
					stringvalidator.OneOf(enumValues("AnnouncementRuleV2", "conditions_no_longer_apply_behaviour")...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"template_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AnnouncementRulesCreatePayloadV2", "template_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             emptySet,
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "owning_team_ids"),
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "created_at"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("AnnouncementRuleV2", "updated_at"),
			},
		},
	}
}

func (r *IncidentAnnouncementRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AnnouncementRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AnnouncementRulesV2CreateWithResponse(ctx, data.ToCreatePayload())
	if err == nil && result.JSON201 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create announcement rule '%s', got error: %s", data.Name.ValueString(), err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an announcement rule with id=%s", result.JSON201.AnnouncementRule.Id))

	state := models.AnnouncementRuleModel{}.FromAPI(result.JSON201.AnnouncementRule, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AnnouncementRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.show(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Announcement rule with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement rule, got error: %s", err))
		return
	}

	state := models.AnnouncementRuleModel{}.FromAPI(*rule, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AnnouncementRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AnnouncementRulesV2UpdateWithResponse(ctx, data.ID.ValueString(), data.ToUpdatePayload())
	if err == nil && result.JSON200 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update announcement rule, got error: %s", err))
		return
	}

	state := models.AnnouncementRuleModel{}.FromAPI(result.JSON200.AnnouncementRule, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AnnouncementRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.AnnouncementRulesV2DestroyWithResponse(ctx, data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete announcement rule, got error: %s", err))
		return
	}
}

func (r *IncidentAnnouncementRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	rule, err := r.show(ctx, req.ID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError("Announcement Rule Not Found", fmt.Sprintf("No announcement rule with ID %q exists.", req.ID))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement rule, got error: %s", err))
		return
	}

	state := models.AnnouncementRuleModel{}.FromAPI(*rule, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementRuleResource) show(ctx context.Context, id string) (*client.AnnouncementRuleV2, error) {
	result, err := r.client.AnnouncementRulesV2ShowWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON200.AnnouncementRule, nil
}
