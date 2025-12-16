package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentEscalationPathDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentEscalationPathDataSource{}
)

func NewIncidentEscalationPathDataSource() datasource.DataSource {
	return &IncidentEscalationPathDataSource{}
}

type IncidentEscalationPathDataSource struct {
	client                        *client.ClientWithResponses
	escalationPathTypeID          string
	escalationPathTypeIDOnce      sync.Once
	escalationPathTypeIDLookupErr error
}

func (d *IncidentEscalationPathDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path"
}

func (d *IncidentEscalationPathDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *IncidentEscalationPathDataSource) getEscalationPathTypeID(ctx context.Context) (string, error) {
	d.escalationPathTypeIDOnce.Do(func() {
		typesResult, err := d.client.CatalogV3ListTypesWithResponse(ctx)
		if err == nil && typesResult.StatusCode() >= 400 {
			err = fmt.Errorf(string(typesResult.Body))
		}
		if err != nil {
			d.escalationPathTypeIDLookupErr = fmt.Errorf("unable to list catalog types, got error: %s", err)
			return
		}

		for _, catalogType := range typesResult.JSON200.CatalogTypes {
			if catalogType.Name == "Escalation Path" {
				d.escalationPathTypeID = catalogType.Id
				return
			}
		}

		d.escalationPathTypeIDLookupErr = fmt.Errorf("catalog type Escalation Path not found")
	})

	if d.escalationPathTypeIDLookupErr != nil {
		return "", d.escalationPathTypeIDLookupErr
	}

	return d.escalationPathTypeID, nil
}

func (d *IncidentEscalationPathDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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
			"path": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathV2", "path"),
				NestedObject:        d.getPathSchema(4),
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
										MarkdownDescription: apischema.Docstring("WeekdayIntervalV2", "weekday"),
									},
								},
							},
						},
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

// getPathSchema returns the nested schema for escalation path nodes.
// Terraform doesn't support recursive schemas so we manually unpack to a finite depth.
func (d *IncidentEscalationPathDataSource) getPathSchema(depth int) schema.NestedAttributeObject {
	result := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "id"),
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "type"),
			},
			"level": schema.SingleNestedAttribute{
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
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "type"),
								},
								"urgency": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "urgency"),
								},
								"schedule_mode": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "schedule_mode"),
								},
							},
						},
					},
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
					"time_to_ack_seconds": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_seconds"),
					},
					"time_to_ack_interval_condition": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_interval_condition"),
					},
					"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_weekday_interval_config_id"),
					},
					"ack_mode": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "ack_mode"),
					},
				},
			},
			"repeat": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "repeat"),
				Attributes: map[string]schema.Attribute{
					"repeat_times": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2", "repeat_times"),
					},
					"to_node": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeRepeatV2", "to_node"),
					},
				},
			},
			"notify_channel": schema.SingleNestedAttribute{
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
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "id"),
								},
								"type": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "type"),
								},
								"urgency": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "urgency"),
								},
								"schedule_mode": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "schedule_mode"),
								},
							},
						},
					},
					"time_to_ack_seconds": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "time_to_ack_seconds"),
					},
					"time_to_ack_interval_condition": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "time_to_ack_interval_condition"),
					},
					"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "time_to_ack_weekday_interval_config_id"),
					},
				},
			},
		},
	}

	// Only include if_else attribute if we haven't reached the maximum nesting depth
	if depth > 0 {
		result.Attributes["if_else"] = schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "if_else"),
			Attributes: map[string]schema.Attribute{
				"conditions": schema.ListNestedAttribute{
					Computed:            true,
					MarkdownDescription: "The prerequisite conditions that must all be satisfied",
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"operation": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "The logical operation to be applied",
							},
							"param_bindings": schema.ListNestedAttribute{
								Computed:            true,
								MarkdownDescription: apischema.Docstring("ConditionV2", "param_bindings"),
								NestedObject: schema.NestedAttributeObject{
									Attributes: map[string]schema.Attribute{
										"array_value": schema.ListNestedAttribute{
											Computed:            true,
											MarkdownDescription: "The array of literal or reference parameter values",
											NestedObject: schema.NestedAttributeObject{
												Attributes: map[string]schema.Attribute{
													"literal": schema.StringAttribute{
														Computed:            true,
														MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
													},
													"reference": schema.StringAttribute{
														Computed:            true,
														MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
													},
												},
											},
										},
										"value": schema.SingleNestedAttribute{
											Computed:            true,
											MarkdownDescription: "The literal or reference parameter value",
											Attributes: map[string]schema.Attribute{
												"literal": schema.StringAttribute{
													Computed:            true,
													MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
												},
												"reference": schema.StringAttribute{
													Computed:            true,
													MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
												},
											},
										},
									},
								},
							},
							"subject": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "The subject of the condition, on which the operation is applied",
							},
						},
					},
				},
				"else_path": schema.ListNestedAttribute{
					Computed:            true,
					MarkdownDescription: apischema.Docstring("EscalationPathNodeIfElseV2", "else_path"),
					NestedObject:        d.getPathSchema(depth - 1),
				},
				"then_path": schema.ListNestedAttribute{
					Computed:            true,
					MarkdownDescription: "Then path nodes",
					NestedObject:        d.getPathSchema(depth - 1),
				},
			},
		}
	}

	return result
}

func (d *IncidentEscalationPathDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentEscalationPathResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var escalationPath *client.EscalationPathV2
	if !data.ID.IsNull() {
		// Lookup by ID
		result, err := d.client.EscalationsV2ShowPathWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path, got error: %s", err))
			return
		}
		escalationPath = &result.JSON200.EscalationPath
	} else if !data.Name.IsNull() {
		// Lookup by name using catalog API
		escalationPathName := data.Name.ValueString()

		// Step 1: Get the cached EscalationPath catalog type ID
		escalationPathTypeID, err := d.getEscalationPathTypeID(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", err.Error())
			return
		}

		// Step 2: Find catalog entry by name
		entriesResult, err := d.client.CatalogV3ListEntriesWithResponse(ctx, &client.CatalogV3ListEntriesParams{
			CatalogTypeId: escalationPathTypeID,
			Identifier:    &escalationPathName,
			PageSize:      1,
		})
		if err == nil && entriesResult.StatusCode() >= 400 {
			err = fmt.Errorf(string(entriesResult.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list catalog entries, got error: %s", err))
			return
		}

		if len(entriesResult.JSON200.CatalogEntries) == 0 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find escalation path with name: %s", escalationPathName))
			return
		}

		catalogEntry := entriesResult.JSON200.CatalogEntries[0]
		if catalogEntry.ExternalId == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Catalog entry for escalation path '%s' has no external ID", escalationPathName))
			return
		}

		// Step 3: Fetch escalation path by ID using the external ID from catalog entry
		escalationPathID := *catalogEntry.ExternalId
		result, err := d.client.EscalationsV2ShowPathWithResponse(ctx, escalationPathID)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path with ID %s, got error: %s", escalationPathID, err))
			return
		}
		escalationPath = &result.JSON200.EscalationPath
	} else {
		resp.Diagnostics.AddError("Client Error", "Either 'id' or 'name' must be provided")
		return
	}

	// Reuse the resource's buildModel function for consistency
	resource := &IncidentEscalationPathResource{}
	modelResp := resource.buildModel(*escalationPath)

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}
