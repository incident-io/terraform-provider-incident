package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ datasource.DataSource                   = &IncidentPolicyDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentPolicyDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentPolicyDataSource{}
)

func NewIncidentPolicyDataSource() datasource.DataSource {
	return &IncidentPolicyDataSource{}
}

type IncidentPolicyDataSource struct {
	dataSourceConfigurer
}

// IncidentPolicyDataSourceModel carries a policy's identity and the two fields that say
// what it is, rather than mirroring the resource's config blocks. Someone reaching for this
// wants a policy's id — to reference a policy the dashboard owns, or one another module
// created — and mirroring the blocks would mean a second copy of that whole schema to keep
// in step. Fields can be added later; they cannot be taken away.
type IncidentPolicyDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	PolicyType  types.String `tfsdk:"policy_type"`
}

func (d *IncidentPolicyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (d *IncidentPolicyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up an existing policy, either by `id` or by " +
			"`name`. It reports what the policy is, not how it is configured: use the " +
			"`incident_policy` resource for a policy Terraform owns.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PolicyV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PolicyV2", "name"),
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PolicyV2", "description"),
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: apischema.EnumValuesDescription("PolicyV2", "status"),
				Computed:            true,
			},
			"policy_type": schema.StringAttribute{
				MarkdownDescription: apischema.EnumValuesDescription("PolicyV2", "policy_type"),
				Computed:            true,
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are Optional and
// Computed so either can be used, which means setting both would otherwise silently ignore
// one of them.
func (d *IncidentPolicyDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *IncidentPolicyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — either attribute behind an unresolved conditional,
	// say — is non-null, so judging it here would reject a config that's actually fine.
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

func (d *IncidentPolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentPolicyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var policy *client.PolicyV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.PoliciesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read policy, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read policy, unexpected response: %s", result.Status()))
			return
		}

		policy = &result.JSON200.Policy

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read policy by name, got error: %s", err))
			return
		}

		policy = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	modelResp := &IncidentPolicyDataSourceModel{
		ID:          types.StringValue(policy.Id),
		Name:        types.StringValue(policy.Name),
		Description: types.StringPointerValue(policy.Description),
		Status:      types.StringValue(string(policy.Status)),
		PolicyType:  types.StringValue(string(policy.PolicyType)),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

// policyListPageSize is the largest page the list endpoint accepts.
const policyListPageSize = 50

// findByName pages the policy list looking for an exact name match. The endpoint has no name
// filter, so every page has to be fetched until the name turns up or the pages run out.
//
// A policy's name is unique within an organisation, so the first match is the only one.
func (d *IncidentPolicyDataSource) findByName(ctx context.Context, name string) (*client.PolicyV2, error) {
	var after *string

	for {
		result, err := d.client.PoliciesV2ListWithResponse(ctx, &client.PoliciesV2ListParams{
			PageSize: lo.ToPtr(int64(policyListPageSize)),
			After:    after,
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing policies: %s", result.Status())
		}

		if match, found := lo.Find(result.JSON200.Policies, func(policy client.PolicyV2) bool {
			return policy.Name == name
		}); found {
			return &match, nil
		}

		// The endpoint returns an after cursor only while another page exists, so an
		// absent one ends the walk rather than looping on the last page forever.
		after = result.JSON200.PaginationMeta.After
		if after == nil {
			return nil, fmt.Errorf("no policy found with name %q", name)
		}
	}
}
