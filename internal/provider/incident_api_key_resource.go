package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                   = &IncidentAPIKeyResource{}
	_ resource.ResourceWithImportState    = &IncidentAPIKeyResource{}
	_ resource.ResourceWithValidateConfig = &IncidentAPIKeyResource{}
)

type IncidentAPIKeyResource struct {
	client *client.ClientWithResponses
}

type IncidentAPIKeyResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	RoleNames      types.Set    `tfsdk:"role_names"`
	TeamIDs        types.Set    `tfsdk:"team_ids"`
	TeamRoleNames  types.Set    `tfsdk:"team_role_names"`
	Token          types.String `tfsdk:"token"`
}

func NewIncidentAPIKeyResource() resource.Resource {
	return &IncidentAPIKeyResource{}
}

func (r *IncidentAPIKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *IncidentAPIKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("API Keys V1"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("APIKeyV1", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("APIKeyV1", "name"),
			},
			"role_names": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Account-level roles to assign to this API key. These roles apply across the entire account. Use an empty set if no account-level roles are needed.",
			},
			"team_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("APIKeyV1", "team_ids"),
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"team_role_names": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Roles to grant for the teams specified in `team_ids`. Must be set when `team_ids` is set, and vice versa.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The bearer token for this API key. Only available when the key is first created — store it securely.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *IncidentAPIKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client.Client
}

func (r *IncidentAPIKeyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data IncidentAPIKeyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamIDsSet := !data.TeamIDs.IsNull() && !data.TeamIDs.IsUnknown() && len(data.TeamIDs.Elements()) > 0
	teamRolesSet := !data.TeamRoleNames.IsNull() && !data.TeamRoleNames.IsUnknown() && len(data.TeamRoleNames.Elements()) > 0

	if teamIDsSet != teamRolesSet {
		resp.Diagnostics.AddAttributeError(
			path.Root("team_ids"),
			"Invalid team access configuration",
			"Both team_ids and team_role_names must be set together, or both must be empty.",
		)
	}
}

func (r *IncidentAPIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data IncidentAPIKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleNames := []client.APIKeysCreatePayloadV1RoleNames{}
	if !data.RoleNames.IsNull() && !data.RoleNames.IsUnknown() {
		var names []string
		resp.Diagnostics.Append(data.RoleNames.ElementsAs(ctx, &names, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		roleNames = lo.Map(names, func(n string, _ int) client.APIKeysCreatePayloadV1RoleNames {
			return client.APIKeysCreatePayloadV1RoleNames(n)
		})
	}

	teamIDs := []string{}
	if !data.TeamIDs.IsNull() && !data.TeamIDs.IsUnknown() {
		resp.Diagnostics.Append(data.TeamIDs.ElementsAs(ctx, &teamIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	teamRoleNames := []client.APIKeysCreatePayloadV1TeamRoleNames{}
	if !data.TeamRoleNames.IsNull() && !data.TeamRoleNames.IsUnknown() {
		var names []string
		resp.Diagnostics.Append(data.TeamRoleNames.ElementsAs(ctx, &names, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		teamRoleNames = lo.Map(names, func(n string, _ int) client.APIKeysCreatePayloadV1TeamRoleNames {
			return client.APIKeysCreatePayloadV1TeamRoleNames(n)
		})
	}

	result, err := r.client.APIKeysV1CreateWithResponse(ctx, client.APIKeysV1CreateJSONRequestBody{
		Name:          data.Name.ValueString(),
		RoleNames:     roleNames,
		TeamIds:       teamIDs,
		TeamRoleNames: teamRoleNames,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create API key, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON201.ApiKey)
	data.Token = types.StringValue(result.JSON201.Token)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAPIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data IncidentAPIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.APIKeysV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API key, got error: %s", err))
		return
	}

	token := data.Token // preserve — only returned on create
	data = r.buildModel(result.JSON200.ApiKey)
	data.Token = token
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAPIKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data IncidentAPIKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve token from state — it's not returned on update
	var state IncidentAPIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleNames := []client.APIKeysUpdatePayloadV1RoleNames{}
	if !data.RoleNames.IsNull() && !data.RoleNames.IsUnknown() {
		var names []string
		resp.Diagnostics.Append(data.RoleNames.ElementsAs(ctx, &names, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		roleNames = lo.Map(names, func(n string, _ int) client.APIKeysUpdatePayloadV1RoleNames {
			return client.APIKeysUpdatePayloadV1RoleNames(n)
		})
	}

	teamIDs := []string{}
	if !data.TeamIDs.IsNull() && !data.TeamIDs.IsUnknown() {
		resp.Diagnostics.Append(data.TeamIDs.ElementsAs(ctx, &teamIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	teamRoleNames := []client.APIKeysUpdatePayloadV1TeamRoleNames{}
	if !data.TeamRoleNames.IsNull() && !data.TeamRoleNames.IsUnknown() {
		var names []string
		resp.Diagnostics.Append(data.TeamRoleNames.ElementsAs(ctx, &names, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		teamRoleNames = lo.Map(names, func(n string, _ int) client.APIKeysUpdatePayloadV1TeamRoleNames {
			return client.APIKeysUpdatePayloadV1TeamRoleNames(n)
		})
	}

	result, err := r.client.APIKeysV1UpdateWithResponse(ctx, data.ID.ValueString(), client.APIKeysV1UpdateJSONRequestBody{
		Name:          data.Name.ValueString(),
		RoleNames:     roleNames,
		TeamIds:       teamIDs,
		TeamRoleNames: teamRoleNames,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update API key, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.ApiKey)
	data.Token = state.Token
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAPIKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data IncidentAPIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.APIKeysV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete API key, got error: %s", err))
		return
	}
}

func (r *IncidentAPIKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentAPIKeyResource) buildModel(key client.APIKeyV1) IncidentAPIKeyResourceModel {
	roleNames := lo.Map(key.Roles, func(role client.APIKeyRoleV1, _ int) attr.Value {
		return types.StringValue(string(role.Name))
	})
	roleNamesSet := types.SetValueMust(types.StringType, roleNames)

	teamIDsSet := types.SetNull(types.StringType)
	if len(key.TeamIds) > 0 {
		elements := lo.Map(key.TeamIds, func(id string, _ int) attr.Value {
			return types.StringValue(id)
		})
		teamIDsSet = types.SetValueMust(types.StringType, elements)
	} else {
		teamIDsSet = types.SetValueMust(types.StringType, []attr.Value{})
	}

	teamRoleNames := lo.Map(key.TeamRoles, func(role client.APIKeyTeamRoleV1, _ int) attr.Value {
		return types.StringValue(string(role.Name))
	})
	teamRoleNamesSet := types.SetValueMust(types.StringType, teamRoleNames)

	return IncidentAPIKeyResourceModel{
		ID:            types.StringValue(key.Id),
		Name:          types.StringValue(key.Name),
		RoleNames:     roleNamesSet,
		TeamIDs:       teamIDsSet,
		TeamRoleNames: teamRoleNamesSet,
		Token:         types.StringNull(), // populated by caller when available
	}
}
