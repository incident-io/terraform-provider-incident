package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
	_ resource.Resource                = &IncidentAnnouncementTemplateResource{}
	_ resource.ResourceWithConfigure   = &IncidentAnnouncementTemplateResource{}
	_ resource.ResourceWithImportState = &IncidentAnnouncementTemplateResource{}
)

type IncidentAnnouncementTemplateResource struct {
	resourceConfigurer
}

func NewIncidentAnnouncementTemplateResource() resource.Resource {
	return &IncidentAnnouncementTemplateResource{}
}

func (r *IncidentAnnouncementTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_announcement_template"
}

// nonEmptyString rejects "", which the API stores as "not set" and reads back absent: a
// config saying "" would never match what it reads back. Removing the attribute is how
// you clear one.
var nonEmptyString = []validator.String{stringvalidator.LengthAtLeast(1)}

func (r *IncidentAnnouncementTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Announcement Templates V2"),
			"`fields` and `actions` appear on the post in the order they're listed.\n\n"+
				"Which template is the organisation's default is set in the dashboard, and the default "+
				"template can't be deleted: import it to manage its fields, but remove it from state rather "+
				"than destroying it."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AnnouncementTemplatesCreatePayloadV2", "name"),
			},
			"is_default": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "is_default"),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"fields": schema.ListNestedAttribute{
				Required: true,
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "fields") +
					". Set `fields = []` for a post with no fields.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field_type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: EnumValuesDescription("AnnouncementTemplateFieldPayloadV2", "field_type"),
							Validators: []validator.String{
								stringvalidator.OneOf(enumValues("AnnouncementTemplateFieldPayloadV2", "field_type")...),
							},
						},
						"emoji": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldPayloadV2", "emoji"),
							Validators:          nonEmptyString,
						},
						"custom_field_id": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldPayloadV2", "custom_field_id"),
							Validators:          nonEmptyString,
						},
						"incident_role_id": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldPayloadV2", "incident_role_id"),
							Validators:          nonEmptyString,
						},
						"incident_timestamp_id": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldPayloadV2", "incident_timestamp_id"),
							Validators:          nonEmptyString,
						},
						"rich_text": schema.SingleNestedAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateFieldPayloadV2", "rich_text"),
							Attributes: map[string]schema.Attribute{
								"type": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: EnumValuesDescription("AnnouncementTemplateRichTextV2", "type"),
									Validators: []validator.String{
										stringvalidator.OneOf(enumValues("AnnouncementTemplateRichTextV2", "type")...),
									},
								},
								"contents": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: apischema.Docstring("AnnouncementTemplateRichTextV2", "contents"),
									Validators:          nonEmptyString,
								},
							},
						},
					},
				},
			},
			"actions": schema.ListNestedAttribute{
				Required: true,
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "actions") +
					". Set `actions = []` for a post with no actions.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action_type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: EnumValuesDescription("AnnouncementTemplateActionPayloadV2", "action_type"),
							Validators: []validator.String{
								stringvalidator.OneOf(enumValues("AnnouncementTemplateActionPayloadV2", "action_type")...),
							},
						},
						"emoji": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: apischema.Docstring("AnnouncementTemplateActionPayloadV2", "emoji"),
							Validators:          nonEmptyString,
						},
					},
				},
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: apischema.Docstring("AnnouncementTemplateV2", "owning_team_ids"),
			},
		},
	}
}

func (r *IncidentAnnouncementTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AnnouncementTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AnnouncementTemplatesV2CreateWithResponse(ctx, data.ToCreatePayload())
	if err == nil && result.JSON201 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create announcement template '%s', got error: %s", data.Name.ValueString(), err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an announcement template with id=%s", result.JSON201.AnnouncementTemplate.Id))

	state := models.AnnouncementTemplateModel{}.FromAPI(result.JSON201.AnnouncementTemplate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AnnouncementTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	template, err := r.show(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Announcement template with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement template, got error: %s", err))
		return
	}

	state := models.AnnouncementTemplateModel{}.FromAPI(*template)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AnnouncementTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AnnouncementTemplatesV2UpdateWithResponse(ctx, data.ID.ValueString(), data.ToUpdatePayload())
	if err == nil && result.JSON200 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update announcement template, got error: %s", err))
		return
	}

	state := models.AnnouncementTemplateModel{}.FromAPI(result.JSON200.AnnouncementTemplate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AnnouncementTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// incident.io refuses to delete the default template, or one a rule or workflow still
	// uses, and says which: that message is the useful part of the failure.
	if _, err := r.client.AnnouncementTemplatesV2DestroyWithResponse(ctx, data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete announcement template, got error: %s", err))
		return
	}
}

func (r *IncidentAnnouncementTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	template, err := r.show(ctx, req.ID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError("Announcement Template Not Found", fmt.Sprintf("No announcement template with ID %q exists.", req.ID))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read announcement template, got error: %s", err))
		return
	}

	state := models.AnnouncementTemplateModel{}.FromAPI(*template)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentAnnouncementTemplateResource) show(ctx context.Context, id string) (*client.AnnouncementTemplateV2, error) {
	result, err := r.client.AnnouncementTemplatesV2ShowWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON200.AnnouncementTemplate, nil
}
