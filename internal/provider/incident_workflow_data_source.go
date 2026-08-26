package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentWorkflowDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentWorkflowDataSource{}
)

func NewIncidentWorkflowDataSource() datasource.DataSource {
	return &IncidentWorkflowDataSource{}
}

type IncidentWorkflowDataSource struct {
	client *client.ClientWithResponses
}

func (d *IncidentWorkflowDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (d *IncidentWorkflowDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client.Client
}

// Schema mirrors the workflow resource, with everything but the ID computed.
//
// Read reuses the resource's buildModel, so this has to stay in step with
// IncidentWorkflowResourceModel: an attribute missing here fails State.Set for
// every workflow read, not just the ones that happen to use it.
func (d *IncidentWorkflowDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve the full configuration of an existing workflow by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "id"),
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "name"),
				Computed:            true,
			},
			"folder": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "folder"),
				Computed:            true,
			},
			"shortform": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "shortform"),
				Computed:            true,
			},
			"trigger": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("TriggerSlimV2", "name"),
				Computed:            true,
			},
			"condition_groups": models.ConditionGroupsDataSourceAttribute(),
			"steps": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "steps"),
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"for_each": schema.StringAttribute{
							Computed: true,
						},
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"param_bindings": models.ParamBindingsDataSourceAttribute(),
					},
				},
			},
			"expressions": models.ExpressionsDataSourceAttribute(),
			"once_for": schema.ListAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "once_for"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"include_private_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "include_private_incidents"),
				Computed:            true,
				DeprecationMessage:  "Use `private_incident_scope` instead.",
			},
			"private_incident_scope": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("WorkflowV2", "private_incident_scope"),
				Computed:            true,
			},
			"include_private_escalations": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "include_private_escalations"),
				Computed:            true,
			},
			"owning_team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "owning_team_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"continue_on_step_error": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "continue_on_step_error"),
				Computed:            true,
			},
			"delay": schema.SingleNestedAttribute{
				MarkdownDescription: "Configuration controlling workflow delay behaviour",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"conditions_apply_over_delay": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelayV2", "conditions_apply_over_delay"),
						Computed:            true,
					},
					"for_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelayV2", "for_seconds"),
						Computed:            true,
					},
				},
			},
			"runs_on_incidents": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("WorkflowV2", "runs_on_incidents"),
				Computed:            true,
			},
			"runs_on_incident_modes": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "runs_on_incident_modes"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("WorkflowV2", "state"),
				Computed:            true,
			},
			"form_fields": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "form_fields") +
					"\n\nThe order of the list is the order the fields appear in the form.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "id"),
							Computed:            true,
						},
						"key": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "key"),
							Computed:            true,
						},
						"title": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "title"),
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "type"),
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "description"),
							Computed:            true,
						},
						"array": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "array"),
							Computed:            true,
						},
						"required": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "required"),
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *IncidentWorkflowDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentWorkflowResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.WorkflowsV2ShowWorkflowWithResponse(ctx, data.ID.ValueString(), &client.WorkflowsV2ShowWorkflowParams{
		SkipStepUpgrades: lo.ToPtr(true),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read workflow, got error: %s", err))
		return
	}

	// Reuse the resource's buildModel for consistency. There's no prior state to
	// reconcile against in a data source, so pass nil: we want whatever the API has.
	workflowResource := &IncidentWorkflowResource{}
	model := workflowResource.buildModel(ctx, result.JSON200.Workflow, nil)

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}
