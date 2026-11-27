package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// AccessGroupModel is the Terraform model for an access group: a named group of users
// that access grants give privileges to. Membership is not part of it, because it's
// managed in the dashboard or by SCIM.
type AccessGroupModel struct {
	ID                types.String `tfsdk:"id"`
	UnlockInDashboard types.Bool   `tfsdk:"unlock_in_dashboard"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	Slug              types.String `tfsdk:"slug"`
	Type              types.String `tfsdk:"type"`
}

// FromAPI converts an API access group into the Terraform model. UnlockInDashboard is
// configuration rather than something the API reports, so the caller carries it over.
func (AccessGroupModel) FromAPI(group client.AccessGroupV2, unlockInDashboard types.Bool) AccessGroupModel {
	return AccessGroupModel{
		ID:                types.StringValue(group.Id),
		UnlockInDashboard: unlockInDashboard,
		Name:              types.StringValue(group.Name),
		Description:       types.StringPointerValue(group.Description),
		Slug:              types.StringValue(group.Slug),
		Type:              types.StringValue(string(group.Type)),
	}
}

// ToCreatePayload converts the model into a create payload. An unset slug is omitted, so
// incident.io generates one from the name.
func (m AccessGroupModel) ToCreatePayload() client.AccessGroupsCreatePayloadV2 {
	payload := client.AccessGroupsCreatePayloadV2{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueStringPointer(),
	}
	if !m.Slug.IsNull() && !m.Slug.IsUnknown() {
		payload.Slug = lo.ToPtr(m.Slug.ValueString())
	}

	return payload
}

// ToUpdatePayload converts the model into an update payload. The API saves the name and
// description together and reads an omitted description as "no description", so a
// config that drops the description clears it.
func (m AccessGroupModel) ToUpdatePayload() client.AccessGroupsUpdatePayloadV2 {
	return client.AccessGroupsUpdatePayloadV2{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueStringPointer(),
	}
}

// AccessGroupDataSourceModel is the model for looking up an existing access group.
type AccessGroupDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Slug        types.String `tfsdk:"slug"`
	Type        types.String `tfsdk:"type"`
}

// FromAPI converts an API access group into the data source model.
func (AccessGroupDataSourceModel) FromAPI(group client.AccessGroupV2) AccessGroupDataSourceModel {
	return AccessGroupDataSourceModel{
		ID:          types.StringValue(group.Id),
		Name:        types.StringValue(group.Name),
		Description: types.StringPointerValue(group.Description),
		Slug:        types.StringValue(group.Slug),
		Type:        types.StringValue(string(group.Type)),
	}
}

// AccessGrantModel is the Terraform model for an access grant: the privileges a principal
// (today, always an access group) holds on a resource (today, always the whole
// organization).
type AccessGrantModel struct {
	ID                types.String               `tfsdk:"id"`
	UnlockInDashboard types.Bool                 `tfsdk:"unlock_in_dashboard"`
	Principal         *AccessGrantPrincipalModel `tfsdk:"principal"`
	Resource          *AccessGrantResourceModel  `tfsdk:"resource"`
	Privileges        types.Set                  `tfsdk:"privileges"`
}

type AccessGrantPrincipalModel struct {
	Type types.String `tfsdk:"type"`
	ID   types.String `tfsdk:"id"`
}

type AccessGrantResourceModel struct {
	Type types.String `tfsdk:"type"`
}

// FromAPI converts an API access grant into the Terraform model. The API returns
// privileges exactly as they were sent, including an alias sent in place of a privilege's
// current name, so they read back as configured.
func (AccessGrantModel) FromAPI(grant client.AccessGrantV2, unlockInDashboard types.Bool) AccessGrantModel {
	return AccessGrantModel{
		ID:                types.StringValue(grant.Id),
		UnlockInDashboard: unlockInDashboard,
		Principal: &AccessGrantPrincipalModel{
			Type: types.StringValue(string(grant.Principal.Type)),
			ID:   types.StringValue(grant.Principal.Id),
		},
		Resource: &AccessGrantResourceModel{
			Type: types.StringValue(string(grant.Resource.Type)),
		},
		Privileges: accessStringSet(grant.Privileges),
	}
}

// ToCreatePayload converts the model into a create payload.
func (m AccessGrantModel) ToCreatePayload() client.AccessGrantsCreatePayloadV2 {
	return client.AccessGrantsCreatePayloadV2{
		Principal: client.AccessGrantPrincipalV2{
			Type: client.AccessGrantPrincipalV2Type(m.Principal.Type.ValueString()),
			Id:   m.Principal.ID.ValueString(),
		},
		Resource: client.AccessGrantResourceV2{
			Type: client.AccessGrantResourceV2Type(m.Resource.Type.ValueString()),
		},
		Privileges: m.PrivilegesSlice(),
	}
}

// ToUpdatePayload converts the model into an update payload. Only the privileges can
// change: a different principal or resource is a different grant.
func (m AccessGrantModel) ToUpdatePayload() client.AccessGrantsUpdatePayloadV2 {
	return client.AccessGrantsUpdatePayloadV2{
		Privileges: m.PrivilegesSlice(),
	}
}

// PrivilegesSlice reads the privileges out of the set as a plain slice, which is never
// nil.
func (m AccessGrantModel) PrivilegesSlice() []string {
	privileges := []string{}
	if m.Privileges.IsNull() || m.Privileges.IsUnknown() {
		return privileges
	}

	for _, element := range m.Privileges.Elements() {
		if privilege, ok := element.(types.String); ok {
			privileges = append(privileges, privilege.ValueString())
		}
	}

	return privileges
}

// AccessPrivilegesDataSourceModel is the model for listing the privileges an access grant
// can give.
type AccessPrivilegesDataSourceModel struct {
	IncludeDeprecated types.Bool             `tfsdk:"include_deprecated"`
	Privileges        []AccessPrivilegeModel `tfsdk:"privileges"`
}

type AccessPrivilegeModel struct {
	Name          types.String `tfsdk:"name"`
	Label         types.String `tfsdk:"label"`
	Description   types.String `tfsdk:"description"`
	Parents       types.List   `tfsdk:"parents"`
	Children      types.List   `tfsdk:"children"`
	Aliases       types.List   `tfsdk:"aliases"`
	Deprecated    types.Bool   `tfsdk:"deprecated"`
	ReplacedBy    types.List   `tfsdk:"replaced_by"`
	GrantsNothing types.Bool   `tfsdk:"grants_nothing"`
}

// FromAPI converts an API privilege into the data source model.
func (AccessPrivilegeModel) FromAPI(privilege client.AccessPrivilegeV2) AccessPrivilegeModel {
	return AccessPrivilegeModel{
		Name:          types.StringValue(privilege.Name),
		Label:         types.StringValue(privilege.Label),
		Description:   types.StringPointerValue(privilege.Description),
		Parents:       accessStringList(privilege.Parents),
		Children:      accessStringList(privilege.Children),
		Aliases:       accessStringList(privilege.Aliases),
		Deprecated:    types.BoolValue(privilege.Deprecated),
		ReplacedBy:    accessStringList(privilege.ReplacedBy),
		GrantsNothing: types.BoolValue(privilege.GrantsNothing),
	}
}

func accessStringValues(values []string) []attr.Value {
	return lo.Map(values, func(value string, _ int) attr.Value {
		return types.StringValue(value)
	})
}

func accessStringSet(values []string) types.Set {
	return types.SetValueMust(types.StringType, accessStringValues(values))
}

func accessStringList(values []string) types.List {
	return types.ListValueMust(types.StringType, accessStringValues(values))
}
