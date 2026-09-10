package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
)

var (
	_ datasource.DataSource                   = &EscalationPathBetaDataSource{}
	_ datasource.DataSourceWithConfigure      = &EscalationPathBetaDataSource{}
	_ datasource.DataSourceWithValidateConfig = &EscalationPathBetaDataSource{}
)

func NewEscalationPathBetaDataSource() datasource.DataSource {
	return &EscalationPathBetaDataSource{}
}

type EscalationPathBetaDataSource struct {
	dataSourceConfigurer
}

func (d *EscalationPathBetaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path_beta"
}

func (d *EscalationPathBetaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an escalation path by `id` or `name` and read it in the same flat `sequences` shape as `incident_escalation_path_beta`. Exactly one lookup field should be set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "id"),
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "name"),
			},
			"start": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The key of the sequence this escalation path begins with.",
			},
			"sequences": schema.MapNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Named sequences of nodes, keyed by a name you choose. Each sequence either ends with a `branch` node or runs off the end of the escalation path. Branches reference other sequences by key.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"nodes": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "The nodes in this sequence, in the order they run.",
							NestedObject:        escalationPathBetaNodeDataSourceSchema(),
						},
					},
				},
			},
			"working_hours": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "working_hours"),
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
			"team_ids": schema.SetAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "team_ids"),
				ElementType:         types.StringType,
			},
			"kind": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("EscalationPathV2", "kind"),
			},
			"template_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "For a templated path, the `incident_escalation_path_template` it is built from.",
			},
			"param_bindings": schema.MapNestedAttribute{
				Computed:            true,
				MarkdownDescription: "For a templated path, the value bound to each of the template's params, keyed by the param's name.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: paramBindingDataSourceAttributes(),
				},
			},
		},
	}
}

// paramBindingDataSourceAttributes is the computed form of models.ParamBindingAttributes,
// which is what lets the data source share the resource's model.
func paramBindingDataSourceAttributes() map[string]schema.Attribute {
	value := map[string]schema.Attribute{
		"literal": schema.StringAttribute{
			CustomType:          jsontypes.NormalizedJSONOrStringType{},
			MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
			Computed:            true,
		},
		"reference": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
			Computed:            true,
		},
	}
	return map[string]schema.Attribute{
		"array_value": schema.ListNestedAttribute{
			MarkdownDescription: "The array of literal or reference parameter values",
			Computed:            true,
			NestedObject:        schema.NestedAttributeObject{Attributes: value},
		},
		"value": schema.SingleNestedAttribute{
			MarkdownDescription: "The literal or reference parameter value",
			Computed:            true,
			Attributes:          value,
		},
		"value_literal": schema.StringAttribute{
			CustomType: jsontypes.NormalizedJSONOrStringType{},
			Computed:   true,
		},
		"value_reference": schema.StringAttribute{Computed: true},
		"expression_ref":  schema.StringAttribute{Computed: true},
		"values": schema.ListAttribute{
			ElementType: jsontypes.NormalizedJSONOrStringType{},
			Computed:    true,
		},
	}
}

func weekdayIntervalConfigDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "id"),
		},
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "name"),
		},
		"timezone": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "timezone"),
		},
		"weekday_intervals": schema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "weekday_intervals"),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"start_time": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("WeekdayIntervalV2", "start_time"),
					},
					"end_time": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("WeekdayIntervalV2", "end_time"),
					},
					"weekday": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: EnumValuesDescription("WeekdayIntervalV2", "weekday"),
					},
				},
			},
		},
	}
}

// escalationPathBetaNodeDataSourceSchema is the computed form of escalationPathBetaNodeSchema.
// Read reuses the resource's buildModel, so a block missing here fails State.Set for every
// escalation path rather than only the ones using it.
func escalationPathBetaNodeDataSourceSchema() schema.NestedAttributeObject {
	return schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "An id for this node, unique within the escalation path, so a `loop` can name it.",
			},
			"level":           escalationPathLevelAttributeDataSource(),
			"notify_channel":  escalationPathNotifyChannelAttributeDataSource(),
			"delay":           escalationPathDelayAttributeDataSource(),
			"escalation_path": escalationPathEscalationPathAttributeDataSource(),
			"branch": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Send the escalation down one of two sequences, depending on what `if` tests. A branch must be the last node in its sequence.",
				Attributes: map[string]schema.Attribute{
					"if": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: "What the branch tests. Set exactly one of these: a branch tests one thing, so combining them means nesting a second branch inside the first.",
						Attributes: map[string]schema.Attribute{
							"working_hours_active": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "The `id` of one of this escalation path's `working_hours`, met while those hours are active.",
							},
							"priority_one_of": schema.SetAttribute{
								Computed:            true,
								MarkdownDescription: "Alert priority ids, met when the escalation came in at one of them.",
								ElementType:         types.StringType,
							},
						},
					},
					"then": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The key of the sequence to continue down when the condition is met.",
					},
					"else": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The key of the sequence to continue down when the condition is not met.",
					},
				},
			},
			"loop": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Go back to an earlier node and run from there again.",
				Attributes: map[string]schema.Attribute{
					"back_to": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The `id` of the node to repeat from.",
					},
					"times": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2", "repeat_times"),
					},
				},
			},
		},
	}
}

func escalationPathTargetsAttributeDataSource(docType string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring(docType, "targets"),
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "id"),
				},
				"type": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "type"),
				},
				"urgency": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "urgency"),
				},
				"schedule_mode": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "schedule_mode"),
				},
				"selected_rota_id": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "selected_rota_id"),
				},
			},
		},
	}
}

func escalationPathLevelAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "level"),
		Attributes: map[string]schema.Attribute{
			"targets": escalationPathTargetsAttributeDataSource("EscalationPathNodeLevelV2"),
			"round_robin_config": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathRoundRobinConfigV2", "enabled"),
					},
					"rotate_after_seconds": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathRoundRobinConfigV2", "rotate_after_seconds"),
					},
				},
			},
			"retry_config": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"attempts": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathRetryConfigV2", "attempts"),
					},
					"interval_seconds": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathRetryConfigV2", "interval_seconds"),
					},
				},
			},
			"time_to_ack_seconds": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_seconds"),
			},
			"time_to_ack_interval_condition": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("EscalationPathNodeLevelV2", "time_to_ack_interval_condition"),
			},
			"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_weekday_interval_config_id"),
			},
			"ack_mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("EscalationPathNodeLevelV2", "ack_mode"),
			},
		},
	}
}

func escalationPathNotifyChannelAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "notify_channel"),
		Attributes: map[string]schema.Attribute{
			"targets": escalationPathTargetsAttributeDataSource("EscalationPathNodeNotifyChannelV2"),
			"time_to_ack_seconds": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "time_to_ack_seconds"),
			},
			"time_to_ack_interval_condition": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("EscalationPathNodeNotifyChannelV2", "time_to_ack_interval_condition"),
			},
			"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "time_to_ack_weekday_interval_config_id"),
			},
		},
	}
}

func escalationPathDelayAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "delay"),
		Attributes: map[string]schema.Attribute{
			"delay_seconds": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeDelayV2", "delay_seconds"),
			},
			"delay_interval_condition": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("EscalationPathNodeDelayV2", "delay_interval_condition"),
			},
			"delay_weekday_interval_config_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeDelayV2", "delay_weekday_interval_config_id"),
			},
		},
	}
}

func escalationPathEscalationPathAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed: true,
		MarkdownDescription: "Reassign the escalation to another escalation path, " +
			"continuing from that path's first node.",
		Attributes: map[string]schema.Attribute{
			"escalation_path_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeEscalationPathV2", "escalation_path_id"),
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional
// and Computed so either can be used, which means setting both would otherwise silently
// ignore one of them.
func (d *EscalationPathBetaDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *escalationPathBetaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the same
	// apply, or either attribute behind an unresolved conditional — is non-null, so
	// judging it here would reject a config that's actually fine.
	if data.ID.IsUnknown() || data.Name.IsUnknown() {
		return
	}

	switch {
	case !data.ID.IsNull() && !data.Name.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case data.ID.IsNull() && data.Name.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
	}
}

func (d *EscalationPathBetaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data escalationPathBetaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var escalationPath *client.EscalationPathV2
	if !data.ID.IsNull() {
		result, err := d.client.EscalationsV2ShowPathWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path, unexpected response: %s", result.Status()))
			return
		}
		escalationPath = &result.JSON200.EscalationPath
	} else {
		lookup := &IncidentEscalationPathDataSource{dataSourceConfigurer: d.dataSourceConfigurer}
		escalationPathTypeID, err := lookup.getEscalationPathTypeID(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", err.Error())
			return
		}

		name := data.Name.ValueString()
		entriesResult, err := d.client.CatalogV3ListEntriesWithResponse(ctx, &client.CatalogV3ListEntriesParams{
			CatalogTypeId: escalationPathTypeID,
			Identifier:    &name,
			PageSize:      1,
		})
		if err == nil && entriesResult.StatusCode() >= 400 {
			err = fmt.Errorf("%s", entriesResult.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list catalog entries, got error: %s", err))
			return
		}
		if entriesResult.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list catalog entries, unexpected response: %s", entriesResult.Status()))
			return
		}
		if len(entriesResult.JSON200.CatalogEntries) == 0 {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Unable to find escalation path with name: %s", name))
			return
		}

		catalogEntry := entriesResult.JSON200.CatalogEntries[0]
		if catalogEntry.ExternalId == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Catalog entry for escalation path '%s' has no external ID", name))
			return
		}

		result, err := d.client.EscalationsV2ShowPathWithResponse(ctx, *catalogEntry.ExternalId)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path with ID %s, got error: %s", *catalogEntry.ExternalId, err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path, unexpected response: %s", result.Status()))
			return
		}
		escalationPath = &result.JSON200.EscalationPath
	}

	model := (&escalationPathBetaResource{}).buildModel(ctx, *escalationPath, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}
