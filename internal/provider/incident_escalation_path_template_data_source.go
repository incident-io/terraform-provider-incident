package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentEscalationPathTemplateDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentEscalationPathTemplateDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentEscalationPathTemplateDataSource{}
)

func NewIncidentEscalationPathTemplateDataSource() datasource.DataSource {
	return &IncidentEscalationPathTemplateDataSource{}
}

type IncidentEscalationPathTemplateDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentEscalationPathTemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path_template"
}

// Schema is the computed form of incident_escalation_path_template's. Read reuses the
// resource's buildModel, so an attribute missing here fails State.Set for every template
// rather than only the ones using it.
func (d *IncidentEscalationPathTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an escalation path template by `id` or `name` and read it in the same flat " +
			"`sequences` shape as `incident_escalation_path_template`, with its `params` keyed by name. " +
			"Exactly one lookup field should be set.",
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardDataSourceAttribute(),
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "description"),
				Computed:            true,
			},
			"params": schema.MapNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "params") +
					" Keyed by the parameter's name, which is what a templated path binds it under in `param_bindings`.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"label": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "label"),
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "type"),
							Computed:            true,
						},
						"array": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "array"),
							Computed:            true,
						},
						"optional": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "optional"),
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "description"),
							Computed:            true,
						},
					},
				},
			},
			"expressions": models.ExpressionsDataSourceAttribute(),
			"start": schema.StringAttribute{
				MarkdownDescription: "The key of the sequence this template begins with.",
				Computed:            true,
			},
			"sequences": schema.MapNestedAttribute{
				MarkdownDescription: "Named sequences of nodes, keyed by name. Each sequence either ends with a `branch` node or runs off the end of the escalation path. Branches reference other sequences by key. A level or notify_channel target carries either an `id` or a `binding` to one of the template's params or expressions.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"nodes": schema.ListNestedAttribute{
							MarkdownDescription: "The nodes in this sequence, in the order they run.",
							Computed:            true,
							NestedObject:        escalationPathTemplateNodeDataSourceSchema(),
						},
					},
				},
			},
			"working_hours": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "working_hours"),
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: weekdayIntervalConfigDataSourceAttributes(),
				},
			},
			"repeat_config": schema.SingleNestedAttribute{
				MarkdownDescription: "Controls if an escalation will repeat after acknowledgement, when the alert is unresolved. When configured, it will repeat after the specified delay.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"repeat_after_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRepeatConfigV2", "repeat_after_seconds"),
						Computed:            true,
					},
					"delay_repeat_on_activity": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRepeatConfigV2", "delay_repeat_on_activity"),
						Computed:            true,
					},
				},
			},
		},
	}
}

// escalationPathTemplateNodeDataSourceSchema is escalationPathNodeDataSourceSchema with the
// template's targets in its level and notify_channel blocks, as the resource does with
// escalationPathNodeSchema.
func escalationPathTemplateNodeDataSourceSchema() schema.NestedAttributeObject {
	node := escalationPathNodeDataSourceSchema()

	level := escalationPathLevelAttributeDataSource()
	level.Attributes["targets"] = escalationPathTemplateTargetsAttributeDataSource("EscalationPathNodeLevelWithBindingV2")
	node.Attributes["level"] = level

	notifyChannel := escalationPathNotifyChannelAttributeDataSource()
	notifyChannel.Attributes["targets"] = escalationPathTemplateTargetsAttributeDataSource("EscalationPathNodeNotifyChannelWithBindingV2")
	node.Attributes["notify_channel"] = notifyChannel

	return node
}

func escalationPathTemplateTargetsAttributeDataSource(docType string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring(docType, "targets"),
		Computed:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathTargetWithBindingV2", "id"),
					Computed:            true,
				},
				"type": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetWithBindingV2", "type"),
					Computed:            true,
				},
				"urgency": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetWithBindingV2", "urgency"),
					Computed:            true,
				},
				"schedule_mode": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetWithBindingV2", "schedule_mode"),
					Computed:            true,
				},
				"selected_rota_id": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathTargetWithBindingV2", "selected_rota_id"),
					Computed:            true,
				},
				"binding": schema.SingleNestedAttribute{
					MarkdownDescription: "Who this target resolves to, decided per templated path. Its `value.reference` names one of the template's `params`, or one of its `expressions`.",
					Computed:            true,
					Attributes:          models.ParamBindingDataSourceAttributes(),
				},
			},
		},
	}
}

func (d *IncidentEscalationPathTemplateDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *escalationPathTemplateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	validateLookupByIDOrName(data.ID, data.Name, &resp.Diagnostics)
}

func (d *IncidentEscalationPathTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data escalationPathTemplateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var template *client.EscalationPathTemplateV2
	if !data.ID.IsNull() {
		result, err := d.client.EscalationPathTemplatesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path template, got error: %s", err))
			return
		}
		template = &result.JSON200.EscalationPathTemplate
	} else {
		found, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path template by name, got error: %s", err))
			return
		}
		template = found
	}

	model := (&escalationPathTemplateResource{}).buildModel(ctx, *template, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// escalationPathTemplateListPageSize is the largest page the list endpoint accepts.
const escalationPathTemplateListPageSize = 25

// findByName looks for an exact name match, and requires exactly one. The list endpoint's
// search narrows the pages to walk but matches more loosely than an exact name, so each
// page is still filtered, and the walk continues until the cursor runs out. Nothing stops
// two templates sharing a name, so more than one match is reported rather than resolved
// arbitrarily.
func (d *IncidentEscalationPathTemplateDataSource) findByName(ctx context.Context, name string) (*client.EscalationPathTemplateV2, error) {
	var (
		after   *string
		matches []client.EscalationPathTemplateV2
	)

	for {
		result, err := d.client.EscalationPathTemplatesV2ListWithResponse(ctx, &client.EscalationPathTemplatesV2ListParams{
			PageSize: lo.ToPtr(int64(escalationPathTemplateListPageSize)),
			After:    after,
			Search:   lo.ToPtr(name),
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing escalation path templates: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.EscalationPathTemplates, func(template client.EscalationPathTemplateV2, _ int) bool {
			return template.Name == name
		})...)

		if result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no escalation path template found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d escalation path templates named %q; look it up by id instead", len(matches), name)
	}
}
