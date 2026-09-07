package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestIncidentIPAllowlistFromAPI(t *testing.T) {
	updatedAt := time.Date(2021, 8, 17, 13, 28, 57, 0, time.UTC)
	model := incidentIPAllowlistFromAPI(client.IPAllowlistV1{
		Allowlist: []client.IPAllowlistItemV1{
			{Label: lo.ToPtr("London HQ"), Value: "192.0.2.0"},
			{Value: "198.51.100.0/24"},
		},
		Enabled:   true,
		UpdatedAt: &updatedAt,
		Version:   3,
	})

	require.True(t, model.Enabled.ValueBool())
	require.Equal(t, int64(3), model.Version.ValueInt64())
	gotUpdatedAt, diags := model.UpdatedAt.ValueRFC3339Time()
	require.False(t, diags.HasError(), "updated_at: %s", diags)
	require.True(t, updatedAt.Equal(gotUpdatedAt))
	require.Len(t, model.Allowlist, 2)
	require.Equal(t, "London HQ", model.Allowlist[0].Label.ValueString())
	require.Equal(t, "192.0.2.0", model.Allowlist[0].Value.ValueString())
	require.True(t, model.Allowlist[1].Label.IsNull())
	require.Equal(t, "198.51.100.0/24", model.Allowlist[1].Value.ValueString())
}

func TestIncidentIPAllowlistDataSourceRead(t *testing.T) {
	updatedAt := time.Date(2021, 8, 17, 13, 28, 57, 0, time.UTC)
	api := startFakeIPAllowlistAPI(t, client.IPAllowlistV1{
		Allowlist: []client.IPAllowlistItemV1{
			{Label: lo.ToPtr("Office"), Value: "203.0.113.0/24"},
		},
		Enabled:   false,
		UpdatedAt: &updatedAt,
		Version:   1,
	})

	model, resp := readIncidentIPAllowlist(t, api)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	require.False(t, model.Enabled.ValueBool())
	require.Equal(t, int64(1), model.Version.ValueInt64())
	require.Len(t, model.Allowlist, 1)
	require.Equal(t, "Office", model.Allowlist[0].Label.ValueString())
	require.Equal(t, "203.0.113.0/24", model.Allowlist[0].Value.ValueString())
}

func TestIncidentIPAllowlistDataSourceReadError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ip_allowlists", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"type":"resource_forbidden","status":403}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	_, resp := readIncidentIPAllowlist(t, api)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Unable to read IP allowlist")
}

func TestAccIncidentIPAllowlistDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "incident_ip_allowlist" "current" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.incident_ip_allowlist.current", "enabled"),
					resource.TestCheckResourceAttrSet("data.incident_ip_allowlist.current", "version"),
				),
			},
		},
	})
}

func startFakeIPAllowlistAPI(t *testing.T, allowlist client.IPAllowlistV1) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ip_allowlists", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, client.IPAllowlistsShowIPAllowlistResultV1{IpAllowlist: allowlist})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

func readIncidentIPAllowlist(t *testing.T, api *client.ClientWithResponses) (IncidentIPAllowlistDataSourceModel, *datasource.ReadResponse) {
	t.Helper()

	ctx := t.Context()
	d := &IncidentIPAllowlistDataSource{dataSourceConfigurer: withClientDataSource(api)}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building the schema: %v", schemaResp.Diagnostics.Errors())
	}

	emptyConfig := emptyIPAllowlistConfig(t, ctx, schemaResp)
	resp := &datasource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    emptyConfig,
		},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    emptyConfig,
		},
	}, resp)

	var model IncidentIPAllowlistDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &model)...)
	}

	return model, resp
}

func emptyIPAllowlistConfig(t *testing.T, ctx context.Context, schemaResp datasource.SchemaResponse) tftypes.Value {
	t.Helper()

	tfType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, ok, "the schema's Terraform type is not an object")

	attrs := map[string]tftypes.Value{}
	for name, attrType := range tfType.AttributeTypes {
		attrs[name] = tftypes.NewValue(attrType, nil)
	}

	return tftypes.NewValue(tfType, attrs)
}
