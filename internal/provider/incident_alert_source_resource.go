package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
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

// ValidateConfig checks that jira_options is only set when the source type is
// 'jira', and never set otherwise.
func (r *IncidentAlertSourceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.AlertSourceResourceModel
	// We can't validate the whole model here because we want to support dynamic values for
	// attributes used in the resource.

	diagnostic := req.Config.GetAttribute(ctx, path.Root("template"), &data.Template)
	if diagnostic.HasError() {
		// If attribute_values is unknown, don't attempt to validate the managed
		// attributes. We have to return early here because the call to req.Config.Get
		// fails to marshal into the []CatalogEntryAttributeValue in this case.
		return
	}

	req.Config.GetAttribute(ctx, path.Root("source_type"), &data.SourceType)
	req.Config.GetAttribute(ctx, path.Root("jira_options"), &data.JiraOptions)
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

	// Validate visible_to_teams only set when is_private is true
	if data.Template != nil && data.Template.VisibleToTeams != nil {
		if data.Template.IsPrivate.IsNull() || !data.Template.IsPrivate.ValueBool() {
			resp.Diagnostics.Append(diag.NewErrorDiagnostic(
				"visible_to_teams can only be set when is_private is true",
				"The visible_to_teams field specifies which teams can view private alerts, so it requires is_private to be true."))
			return
		}
	}

	// Validate visible_to_teams must be set when is_private is true
	if data.Template != nil && !data.Template.IsPrivate.IsNull() && data.Template.IsPrivate.ValueBool() {
		if data.Template.VisibleToTeams == nil {
			resp.Diagnostics.Append(diag.NewErrorDiagnostic(
				"visible_to_teams must be set when is_private is true",
				"Private alert sources require specifying which teams can view the alerts."))
			return
		}
	}

	// Validate that branches operations have valid root references.
	if data.Template != nil {
		for i, expr := range data.Template.Expressions {
			hasBranches := false
			for _, op := range expr.Operations {
				if op.Branches != nil {
					hasBranches = true
					break
				}
			}

			if !hasBranches {
				continue
			}

			rootRef := expr.RootReference.ValueString()
			if rootRef != "" && rootRef != "." {
				resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
					path.Root("template").AtName("expressions").AtListIndex(i).AtName("root_reference"),
					"Invalid root_reference for branches operation",
					fmt.Sprintf(
						"Expression %q uses a branches (if/else) operation, which requires "+
							"root_reference to be \".\" (the whole scope). Got %q instead.\n\n"+
							"When using branches operations, set root_reference = \".\" and have "+
							"conditions reference absolute paths like \"alert.attributes.xxx\".",
						expr.Label.ValueString(),
						rootRef,
					),
				))
			}
		}
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
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Sources V2"), `We'd generally recommend building alert sources in our [web dashboard](https://app.incident.io/~/alerts/configuration), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing alert source and copy the resulting Terraform without persisting it.`),
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
				Computed: true,
				// We do *not* mark this as sensitive, since it is no more sensitive
				// than other values in the Terraform state.
				//
				// If we marked this as sensitive, it would not appear in CLI output,
				// which makes setting up new alert sources more difficult than
				// necessary.
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "secret_token"),
				PlanModifiers: []planmodifier.String{
					// This does not change after creation
					stringplanmodifier.UseStateForUnknown(),
				},
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
					"attributes": schema.SetNestedAttribute{
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
					"is_private": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
						MarkdownDescription: apischema.Docstring("AlertTemplateV2", "is_private"),
					},
					"visible_to_teams": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("AlertTemplateV2", "visible_to_teams"),
						Attributes:          models.ParamBindingAttributes(),
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
				PlanModifiers: []planmodifier.String{
					// This does not change after creation
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"http_custom_options": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "http_custom_options"),
				Attributes: map[string]schema.Attribute{
					"transform_expression": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertSourceHTTPCustomOptionsV2", "transform_expression"),
					},
					"deduplication_key_path": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("AlertSourceHTTPCustomOptionsV2", "deduplication_key_path"),
					},
				},
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("AlertSourceV2", "owning_team_ids"),
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

	result, err := lockForAlertConfig(ctx, func(ctx context.Context) (*client.AlertSourcesV2CreateResponse, error) {
		var owningTeamIDs *[]string
		if !data.OwningTeamIDs.IsNull() {
			teamIDs := []string{}
			for _, elem := range data.OwningTeamIDs.Elements() {
				if str, ok := elem.(types.String); ok {
					teamIDs = append(teamIDs, str.ValueString())
				}
			}

			owningTeamIDs = &teamIDs
		}

		return r.client.AlertSourcesV2CreateWithResponse(ctx, client.AlertSourcesCreatePayloadV2{
			Name:              data.Name.ValueString(),
			SourceType:        client.AlertSourcesCreatePayloadV2SourceType(data.SourceType.ValueString()),
			Template:          data.Template.ToPayload(),
			JiraOptions:       data.JiraOptions.ToPayload(),
			HttpCustomOptions: data.HTTPCustomOptions.ToPayload(),
			OwningTeamIds:     owningTeamIDs,
		})
	})

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert source, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON200.AlertSource.Id, resp.Diagnostics, client.AlertSource, r.terraformVersion)

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
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Alert source with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source, got error: %s", err))
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

	result, err := lockForAlertConfig(ctx, func(ctx context.Context) (*client.AlertSourcesV2UpdateResponse, error) {
		var owningTeamIDs *[]string
		if !data.OwningTeamIDs.IsNull() {
			teamIDs := []string{}
			for _, elem := range data.OwningTeamIDs.Elements() {
				if str, ok := elem.(types.String); ok {
					teamIDs = append(teamIDs, str.ValueString())
				}
			}

			owningTeamIDs = &teamIDs
		}

		return r.client.AlertSourcesV2UpdateWithResponse(ctx, data.ID.ValueString(), client.AlertSourcesUpdatePayloadV2{
			Name:              data.Name.ValueString(),
			Template:          data.Template.ToPayload(),
			JiraOptions:       data.JiraOptions.ToPayload(),
			HttpCustomOptions: data.HTTPCustomOptions.ToPayload(),
			OwningTeamIds:     owningTeamIDs,
		})
	})

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert source, got error: %s", err))
		return
	}

	claimResource(ctx, r.client, result.JSON200.AlertSource.Id, resp.Diagnostics, client.AlertSource, r.terraformVersion)

	data = models.AlertSourceResourceModel{}.FromAPI(result.JSON200.AlertSource)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.AlertSourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := lockForAlertConfig(ctx, func(ctx context.Context) (*client.AlertSourcesV2DeleteResponse, error) {
		return r.client.AlertSourcesV2DeleteWithResponse(ctx, data.ID.ValueString())
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert source, got error: %s", err))
		return
	}
}

func (r *IncidentAlertSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResource(ctx, r.client, req.ID, resp.Diagnostics, client.AlertSource, r.terraformVersion)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
