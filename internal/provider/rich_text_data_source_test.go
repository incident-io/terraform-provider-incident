package provider

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// TestRichTextDataSourceSchema guards that the data source and the generated client
// came from the same schema: building the schema resolves every apischema lookup
// against the embedded one, and panics if a definition or property is missing.
func TestRichTextDataSourceSchema(t *testing.T) {
	ctx := context.Background()
	d := NewRichTextDataSource()

	var metaResp datasource.MetadataResponse
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_rich_text" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, attr := range []string{"markdown", "feature_set", "json", "dropped_content"} {
		if _, ok := schemaResp.Schema.Attributes[attr]; !ok {
			t.Errorf("schema missing expected attribute %q", attr)
		}
	}

	// Empty means the lookup found the property but not the docstring we rely on.
	for _, attr := range []string{"markdown", "feature_set", "dropped_content"} {
		if schemaResp.Schema.Attributes[attr].GetMarkdownDescription() == "" {
			t.Errorf("attribute %q has no description from the API schema", attr)
		}
	}
}

// TestRichTextDataSourceRead reads against a real API rather than a fake, which would only
// assert our own beliefs about the route and payload and pass happily if the contract moved.
// Skipped without a key, so CI leans on scripts/acceptance-test.sh for live coverage.
//
//	INCIDENT_ENDPOINT=http://localhost:8080/api/public go test ./internal/provider/ -run RichText
func TestRichTextDataSourceRead(t *testing.T) {
	api := richTextTestClient(t)

	t.Run("marks and variables survive the round trip", func(t *testing.T) {
		model, resp := readRichText(t, api, "**Alert**: {{description | truncate: 100}}", "rich")
		if resp.Diagnostics.HasError() {
			t.Fatalf("read produced errors: %+v", resp.Diagnostics)
		}

		// The exact literal a rich text field stores, keys sorted at every level.
		want := `{"content":[{"content":[{"marks":[{"type":"bold"}],"text":"Alert","type":"text"},` +
			`{"text":": ","type":"text"},{"attrs":{"name":"description","truncateTo":100},` +
			`"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`
		if got := model.JSON.ValueString(); got != want {
			t.Errorf("json =\n  %s\nwant\n  %s", got, want)
		}

		// An empty list, not null, so config can call length() on it unguarded.
		if model.DroppedContent.IsNull() {
			t.Error("dropped_content is null, want an empty list")
		}
		if n := len(model.DroppedContent.Elements()); n != 0 {
			t.Errorf("dropped_content has %d elements, want 0", n)
		}
	})

	// The API downgrades content instead of failing, so a silent 200 is how formatting would
	// vanish unnoticed.
	t.Run("content the feature set can't hold is reported", func(t *testing.T) {
		model, resp := readRichText(t, api, "**Alert**: something went wrong", "plain_single_line")
		if resp.Diagnostics.HasError() {
			t.Fatalf("read produced errors: %+v", resp.Diagnostics)
		}

		if got := droppedContent(model); len(got) != 1 || !strings.Contains(got[0], "bold") {
			t.Errorf("dropped_content = %v, want [bold]", got)
		}

		warnings := resp.Diagnostics.Warnings()
		if len(warnings) != 1 {
			t.Fatalf("got %d warnings, want 1: %+v", len(warnings), warnings)
		}
		for _, want := range []string{"plain_single_line", "bold"} {
			if detail := warnings[0].Detail(); !strings.Contains(detail, want) {
				t.Errorf("warning detail %q does not mention %q", detail, want)
			}
		}
	})

	// We keep no copy of the feature sets, so the API's message is the only thing telling
	// someone what they can use.
	t.Run("an unrecognised feature set surfaces the API's error", func(t *testing.T) {
		_, resp := readRichText(t, api, "hello", "nonsense")
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error diagnostic")
		}

		err := resp.Diagnostics.Errors()[0]
		joined := err.Summary() + " | " + err.Detail()
		for _, want := range []string{"Unable to parse markdown", "feature_set"} {
			if !strings.Contains(joined, want) {
				t.Errorf("diagnostic %q does not mention %q", joined, want)
			}
		}
	})
}

// richTextTestClient points at whichever API the environment names, skipping without a key.
func richTextTestClient(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	apiKey := os.Getenv("INCIDENT_API_KEY")
	if apiKey == "" {
		t.Skip("No INCIDENT_API_KEY environment variable set, skipping")
	}

	endpoint := os.Getenv("INCIDENT_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://api.incident.io"
	}

	api, err := client.New(t.Context(), apiKey, endpoint, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

// readRichText runs Read with markdown and feature_set set in config, as Terraform would.
func readRichText(t *testing.T, api *client.ClientWithResponses, markdown, featureSet string) (richTextDataSourceModel, *datasource.ReadResponse) {
	t.Helper()

	ctx := context.Background()
	d := &richTextDataSource{dataSourceConfigurer: withClientDataSource(api)}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	objType := schemaResp.Schema.Type().TerraformType(ctx)

	config := tftypes.NewValue(objType, map[string]tftypes.Value{
		"markdown":    tftypes.NewValue(tftypes.String, markdown),
		"feature_set": tftypes.NewValue(tftypes.String, featureSet),
		// Computed, so unset in config.
		"json":            tftypes.NewValue(tftypes.String, nil),
		"dropped_content": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})

	resp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config},
	}, resp)

	var model richTextDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &model)...)
	}

	return model, resp
}

func droppedContent(model richTextDataSourceModel) []string {
	out := []string{}
	for _, element := range model.DroppedContent.Elements() {
		out = append(out, element.String())
	}

	return out
}
