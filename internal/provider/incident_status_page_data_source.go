package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentStatusPageDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentStatusPageDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentStatusPageDataSource{}
)

func NewIncidentStatusPageDataSource() datasource.DataSource {
	return &IncidentStatusPageDataSource{}
}

type IncidentStatusPageDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentStatusPageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (d *IncidentStatusPageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// The Status Pages V2 tag docstring is about publishing incidents and maintenance
		// windows, which this data source does none of, so the description is the
		// provider's own rather than apischema.TagDocstring.
		MarkdownDescription: "Look up a status page, either by `id` or by `name`. Set exactly one of the two lookup " +
			"attributes; setting both, or neither, is rejected at plan time.\n\n" +
			"Status pages are created in the incident.io dashboard rather than through the API, so there is no " +
			"`incident_status_page` resource. Use this data source to reference one from configuration, such as " +
			"to pass its `id` to a workflow or an alert route. Any valid API key can read status pages; no " +
			"specific scope is needed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("StatusPageV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("StatusPageV2", "name") + ". Names aren't unique, so a lookup fails if more than one status page matches.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("StatusPageV2", "description") + ". Null when the page has none.",
				Computed:            true,
			},
			"public_url": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("StatusPageV2", "public_url") + ". Null when the page has no public URL yet.",
				Computed:            true,
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional
// and Computed so either can be used, which means setting both would otherwise silently
// ignore one of them.
func (d *IncidentStatusPageDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.StatusPageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — either attribute behind an unresolved conditional,
	// or an id taken from another data source — is non-null, so judging it here would
	// reject a config that's actually fine.
	if data.ID.IsUnknown() || data.Name.IsUnknown() {
		return
	}

	switch {
	case !data.ID.IsNull() && !data.Name.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case data.ID.IsNull() && data.Name.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
	}
}

func (d *IncidentStatusPageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.StatusPageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var page *client.StatusPageV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.StatusPagesV2ShowStatusPageWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read status page, got error: %s", err))
			return
		}

		page = &result.JSON200.StatusPage

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read status page by name, got error: %s", err))
			return
		}

		page = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	model := models.StatusPageModel{}.FromAPI(*page)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// statusPageListPageSize is the largest page the list endpoint accepts.
const statusPageListPageSize = 250

// findByName looks for an exact name match in the status page list, and requires exactly
// one. The endpoint has no name filter, so every page has to be fetched to know a name
// doesn't appear on a later one. Nothing stops two pages sharing a name, so more than one
// match is reported rather than resolved arbitrarily.
func (d *IncidentStatusPageDataSource) findByName(ctx context.Context, name string) (*client.StatusPageV2, error) {
	var (
		after   *string
		matches []client.StatusPageV2
	)

	for {
		result, err := d.client.StatusPagesV2ListStatusPagesWithResponse(ctx, &client.StatusPagesV2ListStatusPagesParams{
			PageSize: lo.ToPtr(int64(statusPageListPageSize)),
			After:    after,
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing status pages: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.StatusPages, func(page client.StatusPageV2, _ int) bool {
			return page.Name == name
		})...)

		// The endpoint returns an after cursor only while another page may exist, so an
		// absent one ends the walk rather than looping on the last page forever.
		if result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no status page found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d status pages named %q; look it up by id instead", len(matches), name)
	}
}
