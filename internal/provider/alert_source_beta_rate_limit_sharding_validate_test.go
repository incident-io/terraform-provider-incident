package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestAlertSourceBetaValidateRateLimitSharding(t *testing.T) {
	shardingConfig := func(t *testing.T, keyPath tftypes.Value) map[string]tftypes.Value {
		t.Helper()

		shardingType := attributeType(t, "rate_limit_sharding")

		return map[string]tftypes.Value{
			"source_type": tftypes.NewValue(tftypes.String, "http"),
			"rate_limit_sharding": objectWith(t, shardingType, map[string]tftypes.Value{
				"rate_limit_shard_key_path": keyPath,
			}),
		}
	}

	hasShardKeyError := func(t *testing.T, keyPath tftypes.Value) bool {
		t.Helper()

		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, shardingConfig(t, keyPath)))
		for _, d := range diags.Errors() {
			if strings.Contains(d.Summary(), "Empty shard key path") {
				return true
			}
		}

		return false
	}

	t.Run("rejects an empty path", func(t *testing.T) {
		if !hasShardKeyError(t, tftypes.NewValue(tftypes.String, "")) {
			t.Error("an empty shard key path should be rejected")
		}
	})

	// The value is only known at apply when it comes from a variable or another resource, and
	// ValueString reads unknown as empty — so without an IsUnknown guard this rejects a config
	// that turns out fine.
	t.Run("accepts an unknown path", func(t *testing.T) {
		if hasShardKeyError(t, tftypes.NewValue(tftypes.String, tftypes.UnknownValue)) {
			t.Error("an unknown shard key path should not be rejected at plan time")
		}
	})

	t.Run("accepts a real path", func(t *testing.T) {
		if hasShardKeyError(t, tftypes.NewValue(tftypes.String, "$.metadata.team")) {
			t.Error("a real shard key path should be accepted")
		}
	})

	// Create and Update call the same check, which is what catches the empty path a variable only
	// resolves to at apply. ValidateConfig cannot: the value is unknown when it runs.
	//
	// Covers the check's logic, not that either write still calls it: nothing unit-tests Create or
	// Update. That needs an acceptance case whose path is unknown at plan and empty at apply.
	t.Run("the write path check agrees", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			keyPath types.String
			wantErr bool
		}{
			{"empty", types.StringValue(""), true},
			{"unknown", types.StringUnknown(), false},
			{"null", types.StringNull(), false},
			{"real", types.StringValue("$.priority"), false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var diags diag.Diagnostics
				validateRateLimitSharding(&alertSourceRateLimitSharding{RateLimitShardKeyPath: tc.keyPath}, &diags)

				if got := diags.HasError(); got != tc.wantErr {
					t.Errorf("HasError() = %v, want %v", got, tc.wantErr)
				}
			})
		}
	})

	t.Run("no block is accepted", func(t *testing.T) {
		var diags diag.Diagnostics
		validateRateLimitSharding(nil, &diags)

		if diags.HasError() {
			t.Errorf("a missing block should be accepted, got %v", diags.Errors())
		}
	})
}
