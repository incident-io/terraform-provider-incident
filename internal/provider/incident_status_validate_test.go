package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// validateIncidentStatusRank runs the resource's ValidateConfig against the real schema,
// with rank set to the given raw value and everything else valid.
func validateIncidentStatusRank(t *testing.T, rank tftypes.Value) resource.ValidateConfigResponse {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentStatusResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"id":          tftypes.NewValue(tftypes.String, nil),
			"name":        tftypes.NewValue(tftypes.String, "Cleaning up"),
			"description": tftypes.NewValue(tftypes.String, "We're cleaning up"),
			"category":    tftypes.NewValue(tftypes.String, "live"),
			"rank":        rank,
		}),
	}

	r, ok := NewIncidentStatusResource().(*IncidentStatusResource)
	if !ok {
		t.Fatalf("NewIncidentStatusResource did not return a *IncidentStatusResource")
	}

	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: config}, &resp)

	return resp
}

func TestIncidentStatusValidateRank(t *testing.T) {
	t.Run("accepts a rank the API can order", func(t *testing.T) {
		for _, rank := range []int64{0, 1, maxIncidentStatusRank} {
			resp := validateIncidentStatusRank(t, tftypes.NewValue(tftypes.Number, rank))
			if resp.Diagnostics.HasError() {
				t.Errorf("rank %d: unexpected error: %+v", rank, resp.Diagnostics)
			}
		}
	})

	t.Run("rejects a rank outside the window", func(t *testing.T) {
		for _, rank := range []int64{-1, maxIncidentStatusRank + 1} {
			resp := validateIncidentStatusRank(t, tftypes.NewValue(tftypes.Number, rank))
			if !resp.Diagnostics.HasError() {
				t.Errorf("rank %d: expected an error", rank)
			}
		}
	})

	// An absent rank is a config leaving the order to us; an unknown one is a rank still
	// to be settled, e.g. read off another resource. Neither is something to reject.
	t.Run("passes over an absent or unknown rank", func(t *testing.T) {
		for name, rank := range map[string]tftypes.Value{
			"absent":  tftypes.NewValue(tftypes.Number, nil),
			"unknown": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		} {
			resp := validateIncidentStatusRank(t, rank)
			if resp.Diagnostics.HasError() {
				t.Errorf("%s rank: unexpected error: %+v", name, resp.Diagnostics)
			}
		}
	})
}
