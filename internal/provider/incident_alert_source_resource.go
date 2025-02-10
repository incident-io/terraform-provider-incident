package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ resource.ResourceWithConfigure      = &IncidentAlertSourceResource{}
	_ resource.ResourceWithImportState    = &IncidentAlertSourceResource{}
	_ resource.ResourceWithValidateConfig = &IncidentAlertSourceResource{}
)

type IncidentAlertSourceResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

// ValidateConfig allows us to validate that jira_options is only set when the source type is jira
func (r *IncidentAlertSourceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.AlertSourceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.JiraOptions != nil && data.SourceType.ValueString() != "jira" {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"jira_options can only be set when source_type is jira",
			"These options only apply to the 'jira' source type"))
		return
	}

	if data.JiraOptions == nil && data.SourceType.ValueString() == "jira" {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"jira_options must be set when source_type is jira",
			"These options are required for the 'jira' source type, to specify which projects to watch for new issues."))
		return
	}
}

func NewIncidentAlertSourceResource() resource.Resource {
	return &IncidentAlertSourceResource{}
}

func (r *IncidentAlertSourceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source"
}

func (r *IncidentAlertSourceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("AlertSources V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "id"),
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "name"),
			},
			"source_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "source_type"),
				PlanModifiers: []planmodifier.String{
					// This cannot be changed once the source is set up.
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secret_token": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "secret_token"),
			},
			"template": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "template"),
				Attributes: map[string]schema.Attribute{
					"expressions": models.ExpressionsAttribute(),
					"title": schema.SingleNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "title"),
						Attributes:          models.ParamBindingValueAttributes(),
					},
					"description": schema.SingleNestedAttribute{
						Required:            true,
						Attributes:          models.ParamBindingValueAttributes(),
						MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "description"),
					},
					"attributes": schema.ListNestedAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertTemplatePayloadV2", "attributes"),
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"alert_attribute_id": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplateAttributePayloadV2", "alert_attribute_id"),
								},
								"binding": schema.SingleNestedAttribute{
									Required:            true,
									MarkdownDescription: apischema.Docstring("AlertTemplateAttributePayloadV2", "binding"),
									Attributes:          models.ParamBindingAttributes(),
								},
							},
						},
					},
				},
			},
			"jira_options": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "jira_options"),
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"project_ids": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: apischema.Docstring("AlertSourceJiraOptionsV2", "project_ids"),
					},
				},
			},
			"email_address": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceEmailOptionsV2", "email_address"),
			},
		},
	}
}

func (r *IncidentAlertSourceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client.Client
	r.terraformVersion = client.TerraformVersion
}

func (r *IncidentAlertSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.AlertSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertSourcesV2CreateWithResponse(ctx, client.AlertSourcesCreatePayloadV2{
		Name:        data.Name.ValueString(),
		SourceType:  client.AlertSourcesCreatePayloadV2SourceType(data.SourceType.ValueString()),
		Template:    data.Template.ToPayload(),
		JiraOptions: data.JiraOptions.ToPayload(),
	})

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert source, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an alert source with id=%s", result.JSON200.AlertSource.Id))

	data = models.AlertSourceResourceModel{}.FromAPI(result.JSON200.AlertSource)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.AlertSourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertSourcesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source, got error: %s", err))
		return
	}

	if result.StatusCode() == 404 {
		resp.State.RemoveResource(ctx)
		return
	}

	if result.StatusCode() >= 400 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source, got error: %s", string(result.Body)))
		return
	}

	data = models.AlertSourceResourceModel{}.FromAPI(result.JSON200.AlertSource)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.AlertSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertSourcesV2UpdateWithResponse(ctx, data.ID.ValueString(), client.AlertSourcesUpdatePayloadV2{
		Name:        data.Name.ValueString(),
		Template:    data.Template.ToPayload(),
		JiraOptions: data.JiraOptions.ToPayload(),
	})

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert source, got error: %s", err))
		return
	}

	data = models.AlertSourceResourceModel{}.FromAPI(result.JSON200.AlertSource)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AlertSourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.AlertSourcesV2DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert source, got error: %s", err))
		return
	}
}

func (r *IncidentAlertSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
