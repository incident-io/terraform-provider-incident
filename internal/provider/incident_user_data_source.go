package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

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
	IsActive    types.Bool   `tfsdk:"is_active" json:"is_active"`
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
		// Include inactive users so a scheduled user who has since been
		// deactivated (offboarded) still resolves — otherwise the apply breaks.
		result, err := i.client.UsersV2ListWithResponse(ctx, &client.UsersV2ListParams{
			Email:           data.Email.ValueStringPointer(),
			IncludeInactive: lo.ToPtr(true),
		})
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		user, err = selectUser(result.JSON200.Users)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
	} else if !data.SlackUserID.IsNull() {
		result, err := i.client.UsersV2ListWithResponse(ctx, &client.UsersV2ListParams{
			SlackUserId:     data.SlackUserID.ValueStringPointer(),
			IncludeInactive: lo.ToPtr(true),
		})
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
		user, err = selectUser(result.JSON200.Users)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
			return
		}
	} else {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", "No ID, Email or SlackUserId provided"))
		return
	}

	// Warn (but don't fail) when a config references an inactive user. We still
	// resolve them so an apply doesn't break the moment someone is offboarded,
	// but nudge authors to move on-call responsibilities to an active user. An
	// inactive user is either deactivated (the common case, offboarding) or not
	// yet active — the public API only exposes is_active, so the message covers
	// both rather than asserting deactivation.
	if !user.IsActive {
		lookup := path.Root("email")
		switch {
		case !data.ID.IsNull():
			lookup = path.Root("id")
		case !data.SlackUserID.IsNull():
			lookup = path.Root("slack_user_id")
		}
		resp.Diagnostics.AddAttributeWarning(
			lookup,
			"User is not active",
			fmt.Sprintf(
				"User %q (%s) is not active — they've either been deactivated "+
					"(e.g. offboarded) or are not yet active. Referencing inactive users "+
					"(e.g. in schedules or escalation paths) is discouraged — move these "+
					"responsibilities to an active user.",
				user.Name, user.Id,
			),
		)
	}

	modelResp := i.buildModel(*user)
	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

// selectUser picks the single user a lookup (by email or Slack user ID) meant.
//
// We list with IncludeInactive so a scheduled user who has since been offboarded
// still resolves, which means a lookup can legitimately match more than one user:
// orgs commonly have an active user and a deactivated duplicate on the same email
// (someone who signed in via Slack before SSO, then again via SSO, with the first
// account deactivated). In that case the active user is unambiguously the right
// answer, so narrow to active users before giving up.
func selectUser(users []client.UserWithRolesV2) (*client.UserWithRolesV2, error) {
	if len(users) == 0 {
		return nil, errors.New("user not found")
	}
	if len(users) == 1 {
		return &users[0], nil
	}

	active := lo.Filter(users, func(user client.UserWithRolesV2, _ int) bool {
		return user.IsActive
	})
	if len(active) == 1 {
		return &active[0], nil
	}

	// Either every match is inactive, so there's nothing to disambiguate on, or
	// several are active and we can't tell which one was meant.
	return nil, errors.New("multiple users found")
}

func (i *IncidentUserDataSource) buildModel(userType client.UserWithRolesV2) *IncidentUserDataSourceModel {
	model := &IncidentUserDataSourceModel{
		Email:       types.StringPointerValue(userType.Email),
		ID:          types.StringValue(userType.Id),
		Name:        types.StringValue(userType.Name),
		SlackUserID: types.StringPointerValue(userType.SlackUserId),
		IsActive:    types.BoolValue(userType.IsActive),
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
			"is_active": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the user is active. False if the user has been deactivated (e.g. offboarded) or is not yet active.",
			},
		},
	}
}
