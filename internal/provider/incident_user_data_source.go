package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/samber/lo"
)

var (
	_ datasource.DataSource              = &IncidentUserDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentUserDataSource{}
)

func NewIncidentUserDataSource() datasource.DataSource {
	return &IncidentUserDataSource{}
}

type IncidentUserDataSource struct {
	client *client.ClientWithResponses
}

type IncidentUserDataSourceModel struct {
	BaseRole    *RBACRole    `tfsdk:"base_role" json:"base_role"`
	CustomRoles []*RBACRole  `tfsdk:"custom_roles" json:"custom_roles"`
	Email       types.String `tfsdk:"email" json:"email"`
	ID          types.String `tfsdk:"id" json:"id"`
	Name        types.String `tfsdk:"name" json:"name"`
	Role        types.String `tfsdk:"role" json:"role"`
	SlackUserID types.String `tfsdk:"slack_user_id" json:"slack_user_id"`
}

type IncidentUserRequest struct {
	ID types.String `tfsdk:"id"`
}

type RBACRole struct {
	Description types.String `tfsdk:"description"`
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Slug        types.String `tfsdk:"slug"`
}

func (i *IncidentUserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source User",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client
}

func (i *IncidentUserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (i *IncidentUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentUserDataSourceModel
	req.Config.Get(ctx, &data)

	var user *client.UserWithRolesV2
	if !data.ID.IsNull() {
		resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
		if resp.Diagnostics.HasError() {
			return
		}
		result, err := i.client.UsersV2ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf(string(result.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		user = &result.JSON200.User
	} else if !data.Email.IsNull() {
		result, err := i.client.UsersV2ListWithResponse(ctx, &client.UsersV2ListParams{
			Email: data.Email.ValueStringPointer(),
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf(string(result.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		if len(result.JSON200.Users) == 0 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "User not found"))
			return
		}
		user = &result.JSON200.Users[0]
	} else if !data.SlackUserId.IsNull() {
		result, err := i.client.UsersV2ListWithResponse(ctx, &client.UsersV2ListParams{
			SlackUserId: data.SlackUserId.ValueStringPointer(),
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf(string(result.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		if len(result.JSON200.Users) == 0 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "User not found"))
			return
		}
		user = &result.JSON200.Users[0]
	} else {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "No ID, Email or SlackUserId provided"))
		return
	}

	modelResp := i.buildModel(*user)
	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

func (r *IncidentUserDataSource) buildModel(userType client.UserWithRolesV2) *IncidentUserDataSourceModel {
	var roleDesc string
	if userType.BaseRole.Description != nil {
		roleDesc = *userType.BaseRole.Description
	}
	model := &IncidentUserDataSourceModel{
		BaseRole: &RBACRole{
			Description: types.StringValue(roleDesc),
			Id:          types.StringValue(userType.BaseRole.Id),
			Name:        types.StringValue(userType.BaseRole.Name),
			Slug:        types.StringValue(userType.BaseRole.Slug),
		},
		CustomRoles: lo.Map(userType.CustomRoles, func(role client.RBACRoleV2, _ int) *RBACRole {
			return &RBACRole{
				Description: types.StringPointerValue(role.Description),
				Id:          types.StringValue(role.Id),
				Name:        types.StringValue(role.Name),
				Slug:        types.StringValue(role.Slug),
			}
		}),
		Email:       types.StringPointerValue(userType.Email),
		ID:          types.StringValue(userType.Id),
		Name:        types.StringValue(userType.Name),
		Role:        types.StringValue(string(userType.Role)),
		SlackUserId: types.StringPointerValue(userType.SlackUserId),
	}

	return model
}

func (i *IncidentUserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	rbacRoleAttribute := map[string]schema.Attribute{
		"description": schema.StringAttribute{
			Computed: true,
		},
		"id": schema.StringAttribute{
			Computed: true,
		},
		"name": schema.StringAttribute{
			Computed: true,
		},
		"slug": schema.StringAttribute{
			Computed: true,
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Users V2"),
		Attributes: map[string]schema.Attribute{
			"base_role": schema.SingleNestedAttribute{
				Computed:   true,
				Attributes: rbacRoleAttribute,
			},
			"custom_roles": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: rbacRoleAttribute,
				},
			},
			"email": schema.StringAttribute{
				Optional: true,
			},
			"id": schema.StringAttribute{
				Optional: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"role": schema.StringAttribute{
				Computed: true,
			},
			"slack_user_id": schema.StringAttribute{
				Optional: true,
			},
		},
	}
}
