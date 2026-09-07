package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ datasource.DataSource              = &EscalationPathBetaDataSource{}
	_ datasource.DataSourceWithConfigure = &EscalationPathBetaDataSource{}
)

func NewEscalationPathBetaDataSource() datasource.DataSource {
	return &EscalationPathBetaDataSource{}
}

type EscalationPathBetaDataSource struct {
	dataSourceConfigurer
}

func (d *EscalationPathBetaDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path_beta"
}

func (d *EscalationPathBetaDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	// Build the node schema for datasource
	nodeAttrs := map[string]schema.Attribute{
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
				"if":   escalationPathBetaBranchIfAttributeDataSource(),
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
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about an existing escalation path by ID or name.",
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
							NestedObject: schema.NestedAttributeObject{
								Attributes: nodeAttrs,
							},
						},
					},
				},
			},
			"working_hours": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "working_hours"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
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
					},
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
		},
	}
}

func (d *EscalationPathBetaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data escalationPathBetaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch all escalation paths
	result, err := d.client.EscalationsV2ListPathsWithResponse(ctx, &client.EscalationsV2ListPathsParams{})
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("%s", result.Body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list escalation paths, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list escalation paths, unexpected response: %s", result.Status()))
		return
	}

	// Filter by ID or name
	var foundEP *client.EscalationPathV2
	for _, ep := range result.JSON200.EscalationPaths {
		ep := ep
		if (!data.ID.IsNull() && data.ID.ValueString() != "" && ep.Id == data.ID.ValueString()) ||
			(!data.Name.IsNull() && data.Name.ValueString() != "" && ep.Name == data.Name.ValueString()) {
			foundEP = &ep
			break
		}
	}

	if foundEP == nil {
		if !data.ID.IsNull() && data.ID.ValueString() != "" {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Unable to find escalation path with ID: %s", data.ID.ValueString()))
		} else if !data.Name.IsNull() && data.Name.ValueString() != "" {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Unable to find escalation path with name: %s", data.Name.ValueString()))
		} else {
			resp.Diagnostics.AddError("Missing Filter", "Either id or name must be provided")
		}
		return
	}

	// Convert API response to model using the resource's buildModel logic
	r := &escalationPathBetaResource{}
	model := r.buildModel(ctx, *foundEP, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// Helper functions for datasource schema elements
func escalationPathLevelAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "level"),
		Attributes: map[string]schema.Attribute{
			"targets": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "targets"),
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
			},
			"time_to_ack_seconds": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_seconds"),
			},
		},
	}
}

func escalationPathNotifyChannelAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "notify_channel"),
		Attributes: map[string]schema.Attribute{
			"targets": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "targets"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("EscalationPathChannelTargetV2", "id"),
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: EnumValuesDescription("EscalationPathChannelTargetV2", "type"),
						},
					},
				},
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
		},
	}
}

func escalationPathEscalationPathAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "escalation_path"),
		Attributes: map[string]schema.Attribute{
			"escalation_path_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeEscalationPathV2", "escalation_path_id"),
			},
		},
	}
}

func escalationPathBetaBranchIfAttributeDataSource() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Conditions to test.",
		Attributes: map[string]schema.Attribute{
			"alert_properties": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Test the alert property.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Name of the alert property.",
						},
						"value": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Value to match.",
						},
					},
				},
			},
		},
	}
}
