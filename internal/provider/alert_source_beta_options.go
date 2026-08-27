package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// Per-source-type options, shaped the same way the V2 resource shapes them so a config moving
// to the beta resource doesn't have to relearn them.
//
// Two fields the API returns inside its options are lifted out or left in place to match V2:
// email_address sits at the top level of the resource, while ping_url stays inside
// heartbeat_options.

type alertSourceJiraOptions struct {
	ProjectIDs []types.String `tfsdk:"project_ids"`
}

type alertSourceHeartbeatOptions struct {
	IntervalSeconds    types.Int64  `tfsdk:"interval_seconds"`
	FailureThreshold   types.Int64  `tfsdk:"failure_threshold"`
	GracePeriodSeconds types.Int64  `tfsdk:"grace_period_seconds"`
	PingURL            types.String `tfsdk:"ping_url"`
}

type alertSourceEmailOptions struct {
	TransformExpression types.String   `tfsdk:"transform_expression"`
	Redactions          []types.String `tfsdk:"redactions"`
}

type alertSourceHTTPCustomOptions struct {
	TransformExpression  types.String `tfsdk:"transform_expression"`
	DeduplicationKeyPath types.String `tfsdk:"deduplication_key_path"`
}

// Not per-source-type, unlike everything else here: most source types accept it, and which ones
// is decided server-side, so the provider carries no list.
type alertSourceRateLimitSharding struct {
	RateLimitShardKeyPath types.String `tfsdk:"rate_limit_shard_key_path"`
}

func jiraOptionsAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "jira_options"),
		Attributes: map[string]schema.Attribute{
			// Required, where V2 has it Optional: the API stores no options at all for an
			// empty list, so it would read back as an absent block and fail the apply.
			// ValidateConfig rejects an empty list for the same reason.
			"project_ids": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("AlertSourceJiraOptionsV3", "project_ids"),
			},
		},
	}
}

func heartbeatOptionsAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "heartbeat_options"),
		Attributes: map[string]schema.Attribute{
			"interval_seconds": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsPayloadV3", "interval_seconds"),
			},
			"failure_threshold": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsPayloadV3", "failure_threshold"),
			},
			"grace_period_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsPayloadV3", "grace_period_seconds"),
			},
			"ping_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHeartbeatOptionsV3", "ping_url"),
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIncludingNull{},
				},
			},
		},
	}
}

func emailOptionsAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "email_options"),
		Attributes: map[string]schema.Attribute{
			"transform_expression": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceEmailOptionsPayloadV3", "transform_expression"),
			},
			"redactions": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: EnumValuesDescription("AlertSourceEmailOptionsPayloadV3", "redactions"),
			},
		},
	}
}

func httpCustomOptionsAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "http_custom_options"),
		Attributes: map[string]schema.Attribute{
			"transform_expression": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHTTPCustomOptionsV3", "transform_expression"),
			},
			"deduplication_key_path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceHTTPCustomOptionsV3", "deduplication_key_path"),
			},
		},
	}
}

func rateLimitShardingAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV3", "rate_limit_sharding"),
		Attributes: map[string]schema.Attribute{
			"rate_limit_shard_key_path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceRateLimitShardingV3", "rate_limit_shard_key_path"),
			},
		},
	}
}

// validateRateLimitSharding rejects an empty shard key path once the value is known. Storing one
// leaves the plan claiming a block the read omits, which Terraform aborts on as an inconsistent
// result after apply — naming nothing that points at the cause.
//
// Called from ValidateConfig and from both writes, because a literal is known at plan time but a
// path from a variable or another resource is only known at apply. Unknown is skipped: ValueString
// reads it as empty, which would reject a config that turns out fine.
func validateRateLimitSharding(sharding *alertSourceRateLimitSharding, diags *diag.Diagnostics) {
	if sharding == nil {
		return
	}

	keyPath := sharding.RateLimitShardKeyPath
	if keyPath.IsNull() || keyPath.IsUnknown() || keyPath.ValueString() != "" {
		return
	}

	diags.AddAttributeError(
		path.Root("rate_limit_sharding").AtName("rate_limit_shard_key_path"),
		"Empty shard key path",
		"Give a JSON path to shard on, or remove the rate_limit_sharding block to apply one limit to the whole source.",
	)
}

func (o *alertSourceJiraOptions) toPayload() *client.AlertSourceJiraOptionsV3 {
	if o == nil {
		return nil
	}

	return &client.AlertSourceJiraOptionsV3{
		ProjectIds: lo.Map(o.ProjectIDs, func(id types.String, _ int) string {
			return id.ValueString()
		}),
	}
}

func (o *alertSourceHeartbeatOptions) toPayload() *client.AlertSourceHeartbeatOptionsPayloadV3 {
	if o == nil {
		return nil
	}

	return &client.AlertSourceHeartbeatOptionsPayloadV3{
		IntervalSeconds:    o.IntervalSeconds.ValueInt64(),
		FailureThreshold:   o.FailureThreshold.ValueInt64Pointer(),
		GracePeriodSeconds: o.GracePeriodSeconds.ValueInt64Pointer(),
	}
}

func (o *alertSourceEmailOptions) toPayload() *client.AlertSourceEmailOptionsPayloadV3 {
	if o == nil {
		return nil
	}

	return &client.AlertSourceEmailOptionsPayloadV3{
		TransformExpression: o.TransformExpression.ValueStringPointer(),
		Redactions: lo.Map(o.Redactions, func(redaction types.String, _ int) client.AlertSourceEmailOptionsPayloadV3Redactions {
			return client.AlertSourceEmailOptionsPayloadV3Redactions(redaction.ValueString())
		}),
	}
}

// emailOptionsUpdatePayload sends an empty options block where the config has none, so removing
// the block removes what it held. Omitting it tells the API to leave the stored options alone, so
// redactions the config dropped would survive and read straight back. Only for an email source:
// any other type rejects the block outright.
func emailOptionsUpdatePayload(
	options *alertSourceEmailOptions,
	sourceType types.String,
) *client.AlertSourceEmailOptionsPayloadV3 {
	if options != nil {
		return options.toPayload()
	}

	if sourceType.ValueString() != "email" {
		return nil
	}

	// An empty string clears the transform, where a nil would leave it.
	return &client.AlertSourceEmailOptionsPayloadV3{
		TransformExpression: lo.ToPtr(""),
		Redactions:          []client.AlertSourceEmailOptionsPayloadV3Redactions{},
	}
}

func (o *alertSourceHTTPCustomOptions) toPayload() *client.AlertSourceHTTPCustomOptionsV3 {
	if o == nil {
		return nil
	}

	return &client.AlertSourceHTTPCustomOptionsV3{
		TransformExpression:  o.TransformExpression.ValueString(),
		DeduplicationKeyPath: o.DeduplicationKeyPath.ValueString(),
	}
}

func jiraOptionsFromAPI(options *client.AlertSourceJiraOptionsV3) *alertSourceJiraOptions {
	if options == nil {
		return nil
	}

	return &alertSourceJiraOptions{
		ProjectIDs: lo.Map(options.ProjectIds, func(id string, _ int) types.String {
			return types.StringValue(id)
		}),
	}
}

func heartbeatOptionsFromAPI(options *client.AlertSourceHeartbeatOptionsV3) *alertSourceHeartbeatOptions {
	if options == nil {
		return nil
	}

	return &alertSourceHeartbeatOptions{
		IntervalSeconds:    types.Int64Value(options.IntervalSeconds),
		FailureThreshold:   types.Int64Value(options.FailureThreshold),
		GracePeriodSeconds: types.Int64Value(options.GracePeriodSeconds),
		PingURL:            types.StringValue(options.PingUrl),
	}
}

func emailOptionsFromAPI(options *client.AlertSourceEmailOptionsV3) *alertSourceEmailOptions {
	if options == nil {
		return nil
	}

	return &alertSourceEmailOptions{
		TransformExpression: types.StringPointerValue(options.TransformExpression),
		Redactions: lo.Map(options.Redactions, func(redaction client.AlertSourceEmailOptionsV3Redactions, _ int) types.String {
			return types.StringValue(string(redaction))
		}),
	}
}

func httpCustomOptionsFromAPI(options *client.AlertSourceHTTPCustomOptionsV3) *alertSourceHTTPCustomOptions {
	if options == nil {
		return nil
	}

	return &alertSourceHTTPCustomOptions{
		TransformExpression:  types.StringValue(options.TransformExpression),
		DeduplicationKeyPath: types.StringValue(options.DeduplicationKeyPath),
	}
}

func (o *alertSourceRateLimitSharding) toPayload() *client.AlertSourceRateLimitShardingV3 {
	if o == nil {
		return nil
	}

	return &client.AlertSourceRateLimitShardingV3{
		RateLimitShardKeyPath: o.RateLimitShardKeyPath.ValueString(),
	}
}

// rateLimitShardingUpdatePayload sends an empty path where the config has no block, so removing
// the block stops the source sharding. Omitting it tells the API to leave the stored path alone,
// which would make the value unclearable from HCL.
//
// Safe for every source type, including those that cannot shard: the write path only rejects a
// non-empty path, and accepts an empty one as the no-op it is.
func rateLimitShardingUpdatePayload(sharding *alertSourceRateLimitSharding) *client.AlertSourceRateLimitShardingV3 {
	if sharding != nil {
		return sharding.toPayload()
	}

	return &client.AlertSourceRateLimitShardingV3{RateLimitShardKeyPath: ""}
}

// rateLimitShardingFromAPI maps an absent object to a nil block, matching what the config spells.
// The empty-path guard is defensive: the API omits the object rather than sending one back empty,
// but reading that as a block would diff against a config that never set one.
func rateLimitShardingFromAPI(sharding *client.AlertSourceRateLimitShardingV3) *alertSourceRateLimitSharding {
	if sharding == nil || sharding.RateLimitShardKeyPath == "" {
		return nil
	}

	return &alertSourceRateLimitSharding{
		RateLimitShardKeyPath: types.StringValue(sharding.RateLimitShardKeyPath),
	}
}
