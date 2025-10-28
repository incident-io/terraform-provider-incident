package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
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
	Email       types.String `tfsdk:"email" json:"email"`
	ID          types.String `tfsdk:"id" json:"id"`
	Name        types.String `tfsdk:"name" json:"name"`
	SlackUserID types.String `tfsdk:"slack_user_id" json:"slack_user_id"`
}

type IncidentUserRequest struct {
	ID types.String `tfsdk:"id"`
}

func (i *IncidentUserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source User",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentUserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (i *IncidentUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentUserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	var user *client.UserWithRolesV2
	if !data.ID.IsNull() {
		if resp.Diagnostics.HasError() {
			return
		}
		result, err := i.client.UsersV2ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		user = &result.JSON200.User
	} else if !data.Email.IsNull() {
		result, err := i.client.UsersV2ListWithResponse(ctx, &client.UsersV2ListParams{
			Email: data.Email.ValueStringPointer(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		if len(result.JSON200.Users) == 0 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "User not found"))
			return
		} else if len(result.JSON200.Users) > 1 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "Multiple users found"))
			return
		}
		user = &result.JSON200.Users[0]
	} else if !data.SlackUserID.IsNull() {
		result, err := i.client.UsersV2ListWithResponse(ctx, &client.UsersV2ListParams{
			SlackUserId: data.SlackUserID.ValueStringPointer(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		if len(result.JSON200.Users) == 0 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "User not found"))
			return
		} else if len(result.JSON200.Users) > 1 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "Multiple users found"))
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

func (i *IncidentUserDataSource) buildModel(userType client.UserWithRolesV2) *IncidentUserDataSourceModel {
	model := &IncidentUserDataSourceModel{
		Email:       types.StringPointerValue(userType.Email),
		ID:          types.StringValue(userType.Id),
		Name:        types.StringValue(userType.Name),
		SlackUserID: types.StringPointerValue(userType.SlackUserId),
	}

	return model
}

func (i *IncidentUserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Users V2"),
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Optional: true,
			},
			"id": schema.StringAttribute{
				Optional: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"slack_user_id": schema.StringAttribute{
				Optional: true,
			},
		},
	}
}
