package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// The V2 resource has no acceptance case setting rate_limit_sharding, so these are the only cover
// its mapping has.
func TestAlertSourceRateLimitShardingModel(t *testing.T) {
	t.Run("reads a shard key path", func(t *testing.T) {
		model := AlertSourceRateLimitShardingModel{}.FromAPI(&client.AlertSourceRateLimitShardingV2{
			RateLimitShardKeyPath: "$.metadata.team",
		})
		if model == nil {
			t.Fatal("expected a block")
		}
		if got := model.RateLimitShardKeyPath.ValueString(); got != "$.metadata.team" {
			t.Errorf("unexpected path %q", got)
		}
	})

	// Both spellings of "not sharding" have to read back as no block, or they diff against a
	// config that never set one and fail the apply as an inconsistent result.
	t.Run("reads absent and empty as no block", func(t *testing.T) {
		if model := (AlertSourceRateLimitShardingModel{}).FromAPI(nil); model != nil {
			t.Errorf("absent should read as no block, got %+v", model)
		}

		empty := &client.AlertSourceRateLimitShardingV2{RateLimitShardKeyPath: ""}
		if model := (AlertSourceRateLimitShardingModel{}).FromAPI(empty); model != nil {
			t.Errorf("empty should read as no block, got %+v", model)
		}
	})

	t.Run("create sends nothing when the config has no block", func(t *testing.T) {
		var sharding *AlertSourceRateLimitShardingModel
		if payload := sharding.ToPayload(); payload != nil {
			t.Errorf("expected no payload, got %+v", payload)
		}
	})

	// Update always sends the block, because the API reads an omission as "leave the stored path
	// alone" and the value would otherwise be unclearable from HCL.
	t.Run("update sends an empty path when the config has no block", func(t *testing.T) {
		var sharding *AlertSourceRateLimitShardingModel

		payload := sharding.ToUpdatePayload()
		if payload == nil {
			t.Fatal("update should always send the block")
		}
		if payload.RateLimitShardKeyPath != "" {
			t.Errorf("expected an empty path, got %q", payload.RateLimitShardKeyPath)
		}
	})

	t.Run("validate", func(t *testing.T) {
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
				(&AlertSourceRateLimitShardingModel{RateLimitShardKeyPath: tc.keyPath}).Validate(&diags)

				if got := diags.HasError(); got != tc.wantErr {
					t.Errorf("HasError() = %v, want %v", got, tc.wantErr)
				}
			})
		}

		t.Run("no block", func(t *testing.T) {
			var diags diag.Diagnostics
			var sharding *AlertSourceRateLimitShardingModel
			sharding.Validate(&diags)

			if diags.HasError() {
				t.Errorf("a missing block should be accepted, got %v", diags.Errors())
			}
		})
	})
}
