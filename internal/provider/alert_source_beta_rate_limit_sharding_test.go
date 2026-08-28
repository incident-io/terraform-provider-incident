package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// The drift harness in examples/experiment is what proves the round-trip end to end. These cover
// the three mappings it would fail on, so a break points at the direction that caused it.
func TestAlertSourceBetaRateLimitSharding(t *testing.T) {
	t.Run("reads a shard key path", func(t *testing.T) {
		source := alertSourceV3("http")
		source.RateLimitSharding = &client.AlertSourceRateLimitShardingV3{
			RateLimitShardKeyPath: "$.metadata.team",
		}

		model := fromAPI(t, source, &alertSourceBetaModel{})
		if model.RateLimitSharding == nil {
			t.Fatal("rate_limit_sharding should be set")
		}
		if got := model.RateLimitSharding.RateLimitShardKeyPath.ValueString(); got != "$.metadata.team" {
			t.Errorf("unexpected shard key path %q", got)
		}
	})

	// Both spellings of "not sharding" have to read back as no block, or they diff against a
	// config that never set one and fail the apply as an inconsistent result.
	t.Run("reads an absent object as no block", func(t *testing.T) {
		model := fromAPI(t, alertSourceV3("http"), &alertSourceBetaModel{})
		if model.RateLimitSharding != nil {
			t.Errorf("expected no block, got %+v", model.RateLimitSharding)
		}
	})

	t.Run("reads an empty path as no block", func(t *testing.T) {
		source := alertSourceV3("http")
		source.RateLimitSharding = &client.AlertSourceRateLimitShardingV3{RateLimitShardKeyPath: ""}

		model := fromAPI(t, source, &alertSourceBetaModel{})
		if model.RateLimitSharding != nil {
			t.Errorf("expected no block, got %+v", model.RateLimitSharding)
		}
	})

	// The update payload is what makes the value clearable: the API reads an omitted block as
	// "leave the stored path alone", so a removed block has to send an empty path instead.
	t.Run("update sends an empty path when the config has no block", func(t *testing.T) {
		payload := rateLimitShardingUpdatePayload(nil)
		if payload == nil {
			t.Fatal("update should always send the block")
		}
		if payload.RateLimitShardKeyPath != "" {
			t.Errorf("expected an empty path, got %q", payload.RateLimitShardKeyPath)
		}
	})

	t.Run("update sends the configured path", func(t *testing.T) {
		payload := rateLimitShardingUpdatePayload(&alertSourceRateLimitSharding{
			RateLimitShardKeyPath: types.StringValue("$.priority"),
		})
		if payload == nil || payload.RateLimitShardKeyPath != "$.priority" {
			t.Errorf("unexpected payload %+v", payload)
		}
	})

	// Create differs from update: a new source has nothing to clear, so sending nothing leaves it
	// unsharded rather than needing an explicit empty path.
	t.Run("create sends nothing when the config has no block", func(t *testing.T) {
		var sharding *alertSourceRateLimitSharding
		if payload := sharding.toPayload(); payload != nil {
			t.Errorf("expected no payload, got %+v", payload)
		}
	})
}
