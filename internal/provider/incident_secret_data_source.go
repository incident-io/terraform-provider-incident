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
	_ datasource.DataSource                   = &IncidentSecretDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentSecretDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentSecretDataSource{}
)

func NewIncidentSecretDataSource() datasource.DataSource {
	return &IncidentSecretDataSource{}
}

type IncidentSecretDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentSecretDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (d *IncidentSecretDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Secrets V2"),
			"Use this data source to look up an existing secret, either by `id` or by `name`. This is how "+
				"you reference a secret somebody created in the dashboard without managing it as an "+
				"`incident_secret` resource. Set exactly one of the two lookup attributes; setting both, or "+
				"neither, is rejected at plan time.\n\nA secret's value is never returned by the API, so there "+
				"is no value attribute here: `last_four_chars` and `version` are all a read can tell you about it."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SecretV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SecretV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SecretV2", "description"),
				Computed:            true,
			},
			"owning_team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("SecretV2", "owning_team_ids"),
				Computed:            true,
				ElementType:         types.StringType,
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("SecretV2", "version"),
				Computed:            true,
			},
			"last_four_chars": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("SecretV2", "last_four_chars"),
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				// The API schema has no description for either timestamp, so these are the
				// provider's own words rather than apischema.Docstring.
				MarkdownDescription: "When this secret was created.",
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "When this secret was last changed, which includes being rotated as well as having its metadata edited.",
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional
// and Computed so either can be used, which means setting both would otherwise silently
// ignore one of them.
func (d *IncidentSecretDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.SecretDataSourceModel
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

func (d *IncidentSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.SecretDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var secret *client.SecretV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.SecretsV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read secret, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read secret, unexpected response: %s", result.Status()))
			return
		}

		secret = &result.JSON200.Secret

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read secret by name, got error: %s", err))
			return
		}

		secret = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	model := models.SecretDataSourceModel{}.FromAPI(*secret)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// secretListPageSize is the largest page the list endpoint accepts.
const secretListPageSize = 250

// findByName looks for an exact name match in the secret list, and requires exactly one.
// The endpoint has no name filter, so every page has to be fetched to know a name doesn't
// appear on a later one. A name is unique amongst an organisation's live secrets and the
// list returns only those, so the more-than-one case should be unreachable; it's reported
// rather than resolved arbitrarily in case that ever stops being true.
func (d *IncidentSecretDataSource) findByName(ctx context.Context, name string) (*client.SecretV2, error) {
	var (
		after   *string
		matches []client.SecretV2
	)

	for {
		result, err := d.client.SecretsV2ListWithResponse(ctx, &client.SecretsV2ListParams{
			PageSize: lo.ToPtr(int64(secretListPageSize)),
			After:    after,
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing secrets: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.Secrets, func(secret client.SecretV2, _ int) bool {
			return secret.Name == name
		})...)

		// The endpoint returns an after cursor only while another page exists, so an absent
		// one ends the walk rather than looping on the last page forever.
		if result.JSON200.PaginationMeta == nil || result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no secret found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d secrets named %q; look it up by id instead", len(matches), name)
	}
}
