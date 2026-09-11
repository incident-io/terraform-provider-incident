package models

import (
	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// APIKeyModel is the Terraform model for an API key.
//
// An API key's token runs the opposite way to a secret's value: a secret takes a value in
// and never gives it back, whereas a key hands a token out and will never repeat itself.
// The token is returned when the key is created and each time it is rotated, and by
// nothing else - not a read, not an update - so state is the only place it survives, which
// is why it is Computed and Sensitive rather than write-only. A write-only attribute
// carries a value towards the API, and there is no equivalent for one coming back.
//
// TokenVersion and RotationGracePeriodMinutes are the practitioner's own, stored but never
// sent anywhere except a rotation: incident.io knows nothing about either. A change to
// TokenVersion is what asks for a rotation, mirroring value_wo_version on a secret.
type APIKeyModel struct {
	ID                         types.String      `tfsdk:"id"`
	Name                       types.String      `tfsdk:"name"`
	Comments                   types.String      `tfsdk:"comments"`
	RoleNames                  types.Set         `tfsdk:"role_names"`
	TeamIDs                    types.Set         `tfsdk:"team_ids"`
	TeamRoleNames              types.Set         `tfsdk:"team_role_names"`
	Token                      types.String      `tfsdk:"token"`
	TokenVersion               types.Int64       `tfsdk:"token_version"`
	RotationGracePeriodMinutes types.Int64       `tfsdk:"rotation_grace_period_minutes"`
	CreatedAt                  timetypes.RFC3339 `tfsdk:"created_at"`
	TokenLastIssuedAt          timetypes.RFC3339 `tfsdk:"token_last_issued_at"`
	LastUsedAt                 timetypes.RFC3339 `tfsdk:"last_used_at"`
}

// APIKeyLocals are the attributes that exist only in Terraform, so FromAPI has to be told
// them rather than read them off the API's response.
//
// Token is here because the API returns it once and a later read cannot recover it: after
// a create or a rotate it is the fresh token, and otherwise it is whatever state already
// held. An imported key has none, and there is no way to obtain one short of rotating.
type APIKeyLocals struct {
	Token                      types.String
	TokenVersion               types.Int64
	RotationGracePeriodMinutes types.Int64
}

// FromAPI converts an API key into the Terraform model.
func (APIKeyModel) FromAPI(key client.APIKeyV1, locals APIKeyLocals) APIKeyModel {
	return APIKeyModel{
		ID:                         types.StringValue(key.Id),
		Name:                       types.StringValue(key.Name),
		Comments:                   types.StringPointerValue(key.Comments),
		RoleNames:                  apiKeyRoleNames(key.Roles),
		TeamIDs:                    apiKeyStringSet(key.TeamIds),
		TeamRoleNames:              apiKeyTeamRoleNames(key.TeamRoles),
		Token:                      locals.Token,
		TokenVersion:               locals.TokenVersion,
		RotationGracePeriodMinutes: locals.RotationGracePeriodMinutes,
		CreatedAt:                  timetypes.NewRFC3339TimeValue(key.CreatedAt),
		TokenLastIssuedAt:          timetypes.NewRFC3339TimeValue(key.TokenLastIssuedAt),
		LastUsedAt:                 timetypes.NewRFC3339TimePointerValue(key.LastUsedAt),
	}
}

// ToCreatePayload converts the Terraform model to an API create payload.
func (m APIKeyModel) ToCreatePayload() client.APIKeysCreatePayloadV1 {
	payload := client.APIKeysCreatePayloadV1{
		Name: m.Name.ValueString(),
		// All three collections are required by the API, which reads an empty array as "none
		// of these" - the distinction the schema's empty-set defaults preserve.
		RoleNames: lo.Map(m.RoleNamesSlice(), func(name string, _ int) client.APIKeysCreatePayloadV1RoleNames {
			return client.APIKeysCreatePayloadV1RoleNames(name)
		}),
		TeamIds: m.TeamIDsSlice(),
		TeamRoleNames: lo.Map(m.TeamRoleNamesSlice(), func(name string, _ int) client.APIKeysCreatePayloadV1TeamRoleNames {
			return client.APIKeysCreatePayloadV1TeamRoleNames(name)
		}),
	}

	// Nothing to clear on a brand new key, so absent comments are simply omitted.
	if !m.Comments.IsNull() && !m.Comments.IsUnknown() {
		payload.Comments = lo.ToPtr(m.Comments.ValueString())
	}

	return payload
}

// ToValidatePayload converts the Terraform model to an API validate payload, which takes
// the same shape as the create payload: the endpoint runs the checks Create runs, so a
// config it accepts is one Create accepts.
//
// Unlike the create payload, absent comments are still sent as empty. Nothing is being
// stored, so there is no "leave unchanged" reading to avoid, and it keeps the payload a
// straight description of the planned key.
func (m APIKeyModel) ToValidatePayload() client.APIKeysValidatePayloadV1 {
	return client.APIKeysValidatePayloadV1{
		Name:     m.Name.ValueString(),
		Comments: lo.ToPtr(m.Comments.ValueString()),
		RoleNames: lo.Map(m.RoleNamesSlice(), func(name string, _ int) client.APIKeysValidatePayloadV1RoleNames {
			return client.APIKeysValidatePayloadV1RoleNames(name)
		}),
		TeamIds: m.TeamIDsSlice(),
		TeamRoleNames: lo.Map(m.TeamRoleNamesSlice(), func(name string, _ int) client.APIKeysValidatePayloadV1TeamRoleNames {
			return client.APIKeysValidatePayloadV1TeamRoleNames(name)
		}),
	}
}

// ToUpdatePayload converts the Terraform model to an API update payload, which covers
// everything about a key except its token: that changes only by rotating.
func (m APIKeyModel) ToUpdatePayload() client.APIKeysUpdatePayloadV1 {
	return client.APIKeysUpdatePayloadV1{
		Name: m.Name.ValueString(),
		// Always sent, including empty: the API reads an omitted field as "leave unchanged",
		// so a config that drops a role or a team is asking for it to go away and omitting
		// the field would silently keep it, then fail the post-apply consistency check.
		Comments: lo.ToPtr(m.Comments.ValueString()),
		RoleNames: lo.Map(m.RoleNamesSlice(), func(name string, _ int) client.APIKeysUpdatePayloadV1RoleNames {
			return client.APIKeysUpdatePayloadV1RoleNames(name)
		}),
		TeamIds: m.TeamIDsSlice(),
		TeamRoleNames: lo.Map(m.TeamRoleNamesSlice(), func(name string, _ int) client.APIKeysUpdatePayloadV1TeamRoleNames {
			return client.APIKeysUpdatePayloadV1TeamRoleNames(name)
		}),
	}
}

// RoleNamesSlice reads the account-level role names out of the set.
func (m APIKeyModel) RoleNamesSlice() []string {
	return apiKeyStringSlice(m.RoleNames)
}

// TeamIDsSlice reads the scoped team IDs out of the set.
func (m APIKeyModel) TeamIDsSlice() []string {
	return apiKeyStringSlice(m.TeamIDs)
}

// TeamRoleNamesSlice reads the team-level role names out of the set.
func (m APIKeyModel) TeamRoleNamesSlice() []string {
	return apiKeyStringSlice(m.TeamRoleNames)
}

// APIKeyDataSourceModel is the Terraform model for looking up an existing API key. It has
// no token attribute: the API returns a token only when it issues one, so a lookup can
// never produce the credential itself.
type APIKeyDataSourceModel struct {
	ID                types.String      `tfsdk:"id"`
	Name              types.String      `tfsdk:"name"`
	Comments          types.String      `tfsdk:"comments"`
	RoleNames         types.Set         `tfsdk:"role_names"`
	TeamIDs           types.Set         `tfsdk:"team_ids"`
	TeamRoleNames     types.Set         `tfsdk:"team_role_names"`
	CreatedAt         timetypes.RFC3339 `tfsdk:"created_at"`
	TokenLastIssuedAt timetypes.RFC3339 `tfsdk:"token_last_issued_at"`
	LastUsedAt        timetypes.RFC3339 `tfsdk:"last_used_at"`
}

// FromAPI converts an API key into the data source model.
func (APIKeyDataSourceModel) FromAPI(key client.APIKeyV1) APIKeyDataSourceModel {
	return APIKeyDataSourceModel{
		ID:                types.StringValue(key.Id),
		Name:              types.StringValue(key.Name),
		Comments:          types.StringPointerValue(key.Comments),
		RoleNames:         apiKeyRoleNames(key.Roles),
		TeamIDs:           apiKeyStringSet(key.TeamIds),
		TeamRoleNames:     apiKeyTeamRoleNames(key.TeamRoles),
		CreatedAt:         timetypes.NewRFC3339TimeValue(key.CreatedAt),
		TokenLastIssuedAt: timetypes.NewRFC3339TimeValue(key.TokenLastIssuedAt),
		LastUsedAt:        timetypes.NewRFC3339TimePointerValue(key.LastUsedAt),
	}
}

// apiKeyRoleNames reduces the API's roles to the names a configuration sets. Each role
// arrives with a human readable description as well, which is documentation rather than
// anything a practitioner chooses, so it isn't carried into state.
func apiKeyRoleNames(roles []client.APIKeyRoleV1) types.Set {
	return apiKeyStringSet(lo.Map(roles, func(role client.APIKeyRoleV1, _ int) string {
		return string(role.Name)
	}))
}

// apiKeyTeamRoleNames does the same for the team-level roles.
func apiKeyTeamRoleNames(roles []client.APIKeyTeamRoleV1) types.Set {
	return apiKeyStringSet(lo.Map(roles, func(role client.APIKeyTeamRoleV1, _ int) string {
		return string(role.Name)
	}))
}

// apiKeyStringSet builds a set of strings, which is never null: the API always returns
// these collections, as an empty array when a key has none, and that is the same thing the
// schema defaults an unset attribute to.
//
// A set rather than a list because none of them is ordered: the API returns roles and teams
// in whatever order it likes, and a list would read that back as a change.
func apiKeyStringSet(values []string) types.Set {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}

	return types.SetValueMust(types.StringType, elements)
}

// apiKeyStringSlice reads a set of strings as a plain slice, which is never nil: a nil
// slice would serialise as an omitted field, which the API reads as "leave unchanged"
// rather than "none of these".
func apiKeyStringSlice(set types.Set) []string {
	if set.IsNull() || set.IsUnknown() {
		return []string{}
	}

	values := make([]string, 0, len(set.Elements()))
	for _, element := range set.Elements() {
		if value, ok := element.(types.String); ok {
			values = append(values, value.ValueString())
		}
	}

	return values
}
