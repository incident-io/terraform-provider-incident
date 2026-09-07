package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestAlertSourceAttributeBetaDataSourceSchema(t *testing.T) {
	ctx := context.Background()
	d := NewAlertSourceAttributeBetaDataSource()

	var metaResp datasource.MetadataResponse
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_alert_source_attribute_beta" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, name := range []string{
		"alert_source_id", "alert_attribute_id", "merge_strategy",
		"value_literal", "value_reference", "expression_ref", "values", "value", "array_value",
	} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing expected attribute %q", name)
		}
	}

	for _, name := range []string{"expression", "named_expression"} {
		if _, ok := schemaResp.Schema.Blocks[name]; !ok {
			t.Errorf("schema missing expected block %q", name)
		}
	}
}

func TestAlertSourceAttributeBetaDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&AlertSourceAttributeBetaDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	attribute := client.AlertSourceAttributeV3{
		AlertSourceId:    testAlertSourceID,
		AlertAttributeId: testAlertAttributeID,
		MergeStrategy:    client.AlertSourceAttributeV3MergeStrategyFirstWins,
		Value: &client.EngineParamBindingValuePayloadV3{
			Literal: lo.ToPtr("production"),
		},
	}

	prior := &alertSourceAttributeBetaModel{}
	model := alertSourceAttributeBetaFromAPI(attribute, prior)

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	assert.False(t, state.Set(ctx, model).HasError(), "setting state from the resource model")
}
