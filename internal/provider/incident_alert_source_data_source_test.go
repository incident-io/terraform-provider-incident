package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// The data source's schema and the resource's model are written separately - a data
// source's schema comes from a different package, so they cannot be one definition - and
// this is what holds them together: a source read from the API, projected by the
// resource's own alertSourceFromAPI, has to fit the schema the data source declares.
//
// A field added to the model or the resource without being added here fails to set rather
// than reading back empty, which is the failure mode this catches.
func TestAlertSourceDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&IncidentAlertSourceDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	source := client.AlertSourceV3{
		Id:             "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:           "Cron heartbeat",
		SourceType:     client.AlertSourceV3SourceTypeHeartbeat,
		SecretToken:    lo.ToPtr("incident_io_alert_source_token_01FCNDV6P870EA6S7TK1DSYDG0"),
		AlertEventsUrl: lo.ToPtr("https://api.incident.io/v2/alert_events/http/01FCNDV6P870EA6S7TK1DSYDG0"),
		IsPrivate:      false,
		OwningTeamIds:  lo.ToPtr([]string{"01G0J1EXE7AXZ2C93K61WBPYEH"}),
		Version:        4,
		Title: &client.EngineParamBindingPayloadV3{
			Value: &client.EngineParamBindingValuePayloadV3{Literal: lo.ToPtr("Cron failed")},
		},
		Description: &client.EngineParamBindingPayloadV3{
			Value: &client.EngineParamBindingValuePayloadV3{Literal: lo.ToPtr("The nightly job did not report")},
		},
		HeartbeatOptions: &client.AlertSourceHeartbeatOptionsV3{
			IntervalSeconds:    60,
			FailureThreshold:   1,
			GracePeriodSeconds: 0,
			PingUrl:            "https://api.incident.io/v2/heartbeat/01GW2G3V0S59R238FAHPDS1R66/ping",
		},
		AutoResolveTimeoutMinutes: lo.ToPtr(int64(60)),
		AutoResolveIncidentAlerts: lo.ToPtr(true),
		Disabled:                  lo.ToPtr(false),
	}

	var diags diag.Diagnostics
	model := alertSourceFromAPI(source, &alertSourceModel{}, &diags)
	require.False(t, diags.HasError(), "projecting the API source: %s", diags)

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	assert.False(t, state.Set(ctx, model).HasError(), "setting state from the API model")
}

// A lookup naming neither field, or both, identifies no single source. Terraform would
// otherwise read whichever the provider happened to prefer.
func TestAlertSourceDataSourceValidateConfig(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&IncidentAlertSourceDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError())

	objectType, ok := resp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, ok)

	config := func(id, name *string) tfsdk.Config {
		values := map[string]tftypes.Value{}
		for attribute, attributeType := range objectType.AttributeTypes {
			values[attribute] = tftypes.NewValue(attributeType, nil)
		}
		if id != nil {
			values["id"] = tftypes.NewValue(tftypes.String, *id)
		}
		if name != nil {
			values["name"] = tftypes.NewValue(tftypes.String, *name)
		}

		return tfsdk.Config{Schema: resp.Schema, Raw: tftypes.NewValue(objectType, values)}
	}

	for name, tc := range map[string]struct {
		id, sourceName *string
		wantError      string
	}{
		"neither":   {wantError: "Missing lookup"},
		"both":      {id: lo.ToPtr("01ABC"), sourceName: lo.ToPtr("Cron"), wantError: "Ambiguous lookup"},
		"id only":   {id: lo.ToPtr("01ABC")},
		"name only": {sourceName: lo.ToPtr("Cron")},
	} {
		t.Run(name, func(t *testing.T) {
			var validateResp datasource.ValidateConfigResponse
			(&IncidentAlertSourceDataSource{}).ValidateConfig(ctx,
				datasource.ValidateConfigRequest{Config: config(tc.id, tc.sourceName)}, &validateResp)

			if tc.wantError == "" {
				assert.False(t, validateResp.Diagnostics.HasError(), "%s", validateResp.Diagnostics)
				return
			}

			require.True(t, validateResp.Diagnostics.HasError(), "expected %s", tc.wantError)
			assert.Equal(t, tc.wantError, validateResp.Diagnostics.Errors()[0].Summary())
		})
	}
}

func TestAccIncidentAlertSourceDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAlertSourceDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// By ID.
					resource.TestCheckResourceAttrPair(
						"data.incident_alert_source.by_id", "id",
						"incident_alert_source.test", "id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_alert_source.by_id", "name",
						"incident_alert_source.test", "name"),
					resource.TestCheckResourceAttr(
						"data.incident_alert_source.by_id", "source_type", "http"),
					resource.TestCheckResourceAttrSet(
						"data.incident_alert_source.by_id", "secret_token"),
					// The point of the v3 data source: the title reads back at the same
					// attribute path the resource writes it at, rather than under template.
					resource.TestCheckResourceAttrPair(
						"data.incident_alert_source.by_id", "title.literal",
						"incident_alert_source.test", "title.literal"),

					// By name, which is what replaced the plural data source.
					resource.TestCheckResourceAttrPair(
						"data.incident_alert_source.by_name", "id",
						"incident_alert_source.test", "id"),
				),
			},
		},
	})
}

func testAccIncidentAlertSourceDataSourceConfig() string {
	return testRunTemplate("incident_alert_source_data_source", testAccIncidentAlertSourceDataSourceTemplate, nil)
}

const testAccIncidentAlertSourceDataSourceTemplate = `
resource "incident_alert_source" "test" {
  name        = {{ stableSuffix "Test HTTP Alert Source" | quote }}
  source_type = "http"
  title = {
    literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Title\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
  }
  description = {
    literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Description\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
  }
}

data "incident_alert_source" "by_id" {
  id = incident_alert_source.test.id
}

data "incident_alert_source" "by_name" {
  name = incident_alert_source.test.name
}
`
