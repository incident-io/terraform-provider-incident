package models

import (
	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// SecretModel is the Terraform model for a secret.
//
// A secret's value is write-only in the API: it's accepted on create and on rotate, and
// is never returned by a read. The model mirrors that. ValueWO is a write-only attribute,
// so it holds a value only in the config and is always null in the plan and the state,
// which means the provider cannot compare it against anything to decide whether it
// changed. ValueWOVersion is an ordinary attribute and so is stored: a change to it is
// what Terraform can see, and therefore what asks for a rotation.
type SecretModel struct {
	ID             types.String      `tfsdk:"id"`
	Name           types.String      `tfsdk:"name"`
	Description    types.String      `tfsdk:"description"`
	OwningTeamIDs  types.Set         `tfsdk:"owning_team_ids"`
	ValueWO        types.String      `tfsdk:"value_wo"`
	ValueWOVersion types.Int64       `tfsdk:"value_wo_version"`
	Version        types.Int64       `tfsdk:"version"`
	LastFourChars  types.String      `tfsdk:"last_four_chars"`
	CreatedAt      timetypes.RFC3339 `tfsdk:"created_at"`
	UpdatedAt      timetypes.RFC3339 `tfsdk:"updated_at"`
}

// FromAPI converts an API secret into the Terraform model.
//
// valueWOVersion comes from the config or the prior state rather than the API, which
// knows nothing about it: it's how the practitioner asks for a rotation, not something
// incident.io stores. ValueWO is left null because a write-only attribute must never be
// written to state.
func (SecretModel) FromAPI(secret client.SecretV2, valueWOVersion types.Int64) SecretModel {
	return SecretModel{
		ID:             types.StringValue(secret.Id),
		Name:           types.StringValue(secret.Name),
		Description:    types.StringPointerValue(secret.Description),
		OwningTeamIDs:  secretOwningTeamIDs(secret.OwningTeamIds),
		ValueWO:        types.StringNull(),
		ValueWOVersion: valueWOVersion,
		Version:        types.Int64Value(secret.Version),
		LastFourChars:  types.StringPointerValue(secret.LastFourChars),
		CreatedAt:      timetypes.NewRFC3339TimeValue(secret.CreatedAt),
		UpdatedAt:      timetypes.NewRFC3339TimeValue(secret.UpdatedAt),
	}
}

// ToCreatePayload converts the Terraform model to an API create payload. The value is
// passed separately because it comes from the config, which is the only place a
// write-only attribute exists.
func (m SecretModel) ToCreatePayload(value string) client.SecretsCreatePayloadV2 {
	payload := client.SecretsCreatePayloadV2{
		Name:          m.Name.ValueString(),
		Value:         value,
		OwningTeamIds: lo.ToPtr(m.OwningTeamIDsSlice()),
	}

	// Nothing to clear on a brand new secret, so an absent description is simply omitted.
	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		payload.Description = lo.ToPtr(m.Description.ValueString())
	}

	return payload
}

// ToUpdatePayload converts the Terraform model to an API update payload, which covers a
// secret's metadata only: the value is changed by rotating.
func (m SecretModel) ToUpdatePayload() client.SecretsUpdatePayloadV2 {
	return client.SecretsUpdatePayloadV2{
		Name: m.Name.ValueString(),
		// The API reads an omitted description as "leave unchanged" and an omitted
		// owning_team_ids the same way, so both are always sent: a config that drops a
		// description or a team is asking for it to go away, and omitting the field would
		// silently keep it and then fail the post-apply consistency check.
		Description:   lo.ToPtr(m.Description.ValueString()),
		OwningTeamIds: lo.ToPtr(m.OwningTeamIDsSlice()),
	}
}

// OwningTeamIDsSlice reads the owning team IDs out of the set as a plain slice, which is
// never nil: a nil slice would serialise as an omitted field, which the API reads as
// "leave unchanged" rather than "no owning teams".
func (m SecretModel) OwningTeamIDsSlice() []string {
	if m.OwningTeamIDs.IsNull() || m.OwningTeamIDs.IsUnknown() {
		return []string{}
	}

	ids := make([]string, 0, len(m.OwningTeamIDs.Elements()))
	for _, element := range m.OwningTeamIDs.Elements() {
		if id, ok := element.(types.String); ok {
			ids = append(ids, id.ValueString())
		}
	}

	return ids
}

// SecretDataSourceModel is the Terraform model for looking up an existing secret. It has
// no value attribute at all: a secret's value can never be read back, so there is nothing
// a data source could return.
type SecretDataSourceModel struct {
	ID            types.String      `tfsdk:"id"`
	Name          types.String      `tfsdk:"name"`
	Description   types.String      `tfsdk:"description"`
	OwningTeamIDs types.Set         `tfsdk:"owning_team_ids"`
	Version       types.Int64       `tfsdk:"version"`
	LastFourChars types.String      `tfsdk:"last_four_chars"`
	CreatedAt     timetypes.RFC3339 `tfsdk:"created_at"`
	UpdatedAt     timetypes.RFC3339 `tfsdk:"updated_at"`
}

// FromAPI converts an API secret into the data source model.
func (SecretDataSourceModel) FromAPI(secret client.SecretV2) SecretDataSourceModel {
	return SecretDataSourceModel{
		ID:            types.StringValue(secret.Id),
		Name:          types.StringValue(secret.Name),
		Description:   types.StringPointerValue(secret.Description),
		OwningTeamIDs: secretOwningTeamIDs(secret.OwningTeamIds),
		Version:       types.Int64Value(secret.Version),
		LastFourChars: types.StringPointerValue(secret.LastFourChars),
		CreatedAt:     timetypes.NewRFC3339TimeValue(secret.CreatedAt),
		UpdatedAt:     timetypes.NewRFC3339TimeValue(secret.UpdatedAt),
	}
}

// secretOwningTeamIDs builds the owning team IDs set. The API always returns the field,
// as an empty array when a secret is owned by the whole organisation, which is the same
// thing the schema defaults an unset attribute to.
func secretOwningTeamIDs(ids []string) types.Set {
	elements := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		elements = append(elements, types.StringValue(id))
	}

	return types.SetValueMust(types.StringType, elements)
}
