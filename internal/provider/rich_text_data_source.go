package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ datasource.DataSource              = &richTextDataSource{}
	_ datasource.DataSourceWithConfigure = &richTextDataSource{}
)

func NewRichTextDataSource() datasource.DataSource {
	return &richTextDataSource{}
}

type richTextDataSource struct {
	client *client.ClientWithResponses
}

type richTextDataSourceModel struct {
	Markdown       types.String `tfsdk:"markdown"`
	FeatureSet     types.String `tfsdk:"feature_set"`
	JSON           types.String `tfsdk:"json"`
	DroppedContent types.List   `tfsdk:"dropped_content"`
}

func (d *richTextDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rich_text"
}

func (d *richTextDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Build a rich text document from markdown, for fields that store a document rather " +
			"than a string. Use it where formatting — bold, links, lists — can't be expressed by writing the " +
			"template inline. One-way: markdown in, document out.",
		Attributes: map[string]schema.Attribute{
			"markdown": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("RichTextParseMarkdownPayloadV1", "markdown"),
			},
			// Unvalidated: the API owns this list, and a copy here would drift.
			"feature_set": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: EnumValuesDescription("RichTextParseMarkdownPayloadV1", "feature_set"),
			},
			"json": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The document, encoded as JSON. Assign it to the rich text field you built " +
					"it for, such as an alert source template's `title` or `description` literal.",
			},
			"dropped_content": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("RichTextParseMarkdownResultV1", "dropped_content"),
			},
		},
	}
}

func (d *richTextDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *IncidentProviderData, got %T. This is a provider bug.", req.ProviderData),
		)
		return
	}
	d.client = data.Client
}

func (d *richTextDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data richTextDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.RichTextV1ParseMarkdownWithResponse(ctx, client.RichTextV1ParseMarkdownJSONRequestBody{
		Markdown:   data.Markdown.ValueString(),
		FeatureSet: client.RichTextParseMarkdownPayloadV1FeatureSet(data.FeatureSet.ValueString()),
	})
	if err != nil {
		// Bad markdown and an unknown feature set both 4xx, which client.go has already turned
		// into an HTTPError carrying the API's message.
		resp.Diagnostics.AddError("Unable to parse markdown", err.Error())
		return
	}
	// A 2xx we couldn't decode, rather than nil-panicking on it.
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to parse markdown", string(result.Body))
		return
	}

	// A rich text field's literal holds the tree alone, not the whole document. Encoding it
	// from the map the client decoded into sorts the keys, so the same document is always the
	// same bytes — otherwise the API's key order would show up as a diff.
	documentJSON, err := json.Marshal(result.JSON200.Document.TextNode)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode parsed document", err.Error())
		return
	}
	data.JSON = types.StringValue(string(documentJSON))

	// Never null, so config can call length() on it without a conditional.
	dropped := result.JSON200.DroppedContent
	if dropped == nil {
		dropped = []string{}
	}
	droppedList, diags := types.ListValueFrom(ctx, types.StringType, dropped)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.DroppedContent = droppedList

	// The API drops content rather than failing, so without this it goes silently.
	if len(dropped) > 0 {
		resp.Diagnostics.AddAttributeWarning(
			path.Root("markdown"),
			"Content dropped from rich text document",
			fmt.Sprintf("The %q feature set does not permit: %s. That content has been dropped from the document.",
				data.FeatureSet.ValueString(), strings.Join(dropped, ", ")),
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
