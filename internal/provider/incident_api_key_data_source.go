package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentAPIKeyDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentAPIKeyDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentAPIKeyDataSource{}
)

func NewIncidentAPIKeyDataSource() datasource.DataSource {
	return &IncidentAPIKeyDataSource{}
}

type IncidentAPIKeyDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentAPIKeyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (d *IncidentAPIKeyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("API Keys V1"),
			"Use this data source to look up an existing API key, either by `id` or by `name`. This is how "+
				"you read the roles held by a key somebody created in the dashboard without managing it as an "+
				"`incident_api_key` resource. Set exactly one of the two lookup attributes; setting both, or "+
				"neither, is rejected at plan time.\n\n"+
				"A key's token is returned only when incident.io issues one, so there is no token attribute "+
				"here: a lookup can tell you what a key may do and when it was last used, but never how to "+
				"authenticate as it."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "name"),
				Optional:            true,
				Computed:            true,
			},
			"comments": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "comments"),
				Computed:            true,
			},
			"role_names": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "roles"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "team_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"team_role_names": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "team_roles"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "created_at"),
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
			"token_last_issued_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "token_last_issued_at"),
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
			"last_used_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("APIKeyV1", "last_used_at"),
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional
// and Computed so either can be used, which means setting both would otherwise silently
// ignore one of them.
func (d *IncidentAPIKeyDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.APIKeyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the same apply,
	// or either attribute behind an unresolved conditional — is non-null, so judging it here
	// would reject a config that's actually fine.
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

func (d *IncidentAPIKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.APIKeyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var key *client.APIKeyV1
	switch {
	case !data.ID.IsNull():
		result, err := d.client.APIKeysV1ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API key, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API key, unexpected response: %s", result.Status()))
			return
		}

		key = &result.JSON200.ApiKey

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API key by name, got error: %s", err))
			return
		}

		key = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	model := models.APIKeyDataSourceModel{}.FromAPI(*key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// apiKeyListPageSize is the largest page the list endpoint accepts.
const apiKeyListPageSize = 250

// findByName looks for an exact name match in the API key list, and requires exactly one.
// The endpoint has no name filter, so every page has to be fetched to know a name doesn't
// appear on a later one.
//
// Unlike a secret's, an API key's name is not unique - the dashboard will happily issue two
// keys called "CI" - so the ambiguous case is genuinely reachable and is reported rather
// than resolved arbitrarily. Looking a key up by id is the way through it.
func (d *IncidentAPIKeyDataSource) findByName(ctx context.Context, name string) (*client.APIKeyV1, error) {
	var (
		after   *string
		matches []client.APIKeyV1
	)

	for {
		result, err := d.client.APIKeysV1ListWithResponse(ctx, &client.APIKeysV1ListParams{
			PageSize: lo.ToPtr(int64(apiKeyListPageSize)),
			After:    after,
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing API keys: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.ApiKeys, func(key client.APIKeyV1, _ int) bool {
			return key.Name == name
		})...)

		// The endpoint returns an after cursor only while another page exists, so an absent
		// one ends the walk rather than looping on the last page forever.
		if result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no API key found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d API keys named %q; look it up by id instead", len(matches), name)
	}
}
