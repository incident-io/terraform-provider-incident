package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
)

// The data source mirrors of the per-source-type options blocks in
// incident_alert_source_options.go. A data source's schema comes from a different package
// to a resource's, so the two can't share one definition even though they describe the
// same thing.
//
// What they do share is the model: these decode into the same alertSourceJiraOptions and
// friends, so a field added to one of those structs without being added here fails
// TestAlertSourceDataSourceSchemaMatchesModel rather than reading back empty.
//
// Everything is Computed, because a data source describes what the API holds. That also
// means none of the defaults or plan modifiers the resource carries belong here: a default
// fills in what an author left out, and a data source has no author.

func jiraOptionsDataSourceAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "jira_options"),
		Attributes: map[string]schema.Attribute{
			"project_ids": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("AlertSourceJiraOptionsV3", "project_ids"),
			},
		},
	}
}

func heartbeatOptionsDataSourceAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "heartbeat_options"),
		Attributes: map[string]schema.Attribute{
			"interval_seconds": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsPayloadV3", "interval_seconds"),
			},
			"failure_threshold": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsPayloadV3", "failure_threshold"),
			},
			"grace_period_seconds": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsPayloadV3", "grace_period_seconds"),
			},
			"ping_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsV3", "ping_url"),
			},
		},
	}
}

func emailOptionsDataSourceAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "email_options"),
		Attributes: map[string]schema.Attribute{
			"transform_expression": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceEmailOptionsPayloadV3", "transform_expression"),
			},
			"redactions": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: EnumValuesDescription("AlertSourceEmailOptionsPayloadV3", "redactions"),
			},
		},
	}
}

func httpCustomOptionsDataSourceAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "http_custom_options"),
		Attributes: map[string]schema.Attribute{
			"transform_expression": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHTTPCustomOptionsV3", "transform_expression"),
			},
			"deduplication_key_path": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHTTPCustomOptionsV3", "deduplication_key_path"),
			},
		},
	}
}

func rateLimitShardingDataSourceAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "rate_limit_sharding"),
		Attributes: map[string]schema.Attribute{
			"rate_limit_shard_key_path": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceRateLimitShardingV3", "rate_limit_shard_key_path"),
			},
		},
	}
}
