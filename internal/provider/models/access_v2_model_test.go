package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func TestAccessGroupModelUpdatePayloadClearsDescription(t *testing.T) {
	// The API reads an omitted description as "no description", so a config without one
	// must send none rather than an empty string.
	model := AccessGroupModel{
		Name:        types.StringValue("Platform"),
		Description: types.StringNull(),
	}

	payload := model.ToUpdatePayload()

	assert.Equal(t, "Platform", payload.Name)
	assert.Nil(t, payload.Description)
}

func TestAccessGroupModelCreatePayloadOmitsUnknownSlug(t *testing.T) {
	model := AccessGroupModel{
		Name: types.StringValue("Platform"),
		Slug: types.StringUnknown(),
	}
	assert.Nil(t, model.ToCreatePayload().Slug)

	model.Slug = types.StringValue("platform-team")
	assert.Equal(t, lo.ToPtr("platform-team"), model.ToCreatePayload().Slug)
}

func TestAccessGrantModelRoundTrip(t *testing.T) {
	grant := client.AccessGrantV2{
		Id:         "01GRANT",
		Principal:  client.AccessGrantPrincipalV2{Type: client.AccessGrantPrincipalV2TypeAccessGroup, Id: "01GROUP"},
		Resource:   client.AccessGrantResourceV2{Type: client.Organization},
		Privileges: []string{"schedules.create", "rbac_roles.manage"},
	}

	model := AccessGrantModel{}.FromAPI(grant, types.BoolNull())

	assert.Equal(t, "01GROUP", model.Principal.ID.ValueString())
	assert.Equal(t, "organization", model.Resource.Type.ValueString())
	// Privileges come back exactly as they were sent, old names included.
	assert.ElementsMatch(t, []string{"schedules.create", "rbac_roles.manage"}, model.PrivilegesSlice())

	payload := model.ToCreatePayload()
	assert.Equal(t, grant.Principal, payload.Principal)
	assert.Equal(t, grant.Resource, payload.Resource)
	assert.ElementsMatch(t, grant.Privileges, payload.Privileges)
}

func TestAccessGrantModelPrivilegesSliceIsNeverNil(t *testing.T) {
	model := AccessGrantModel{Privileges: types.SetNull(types.StringType)}
	assert.NotNil(t, model.PrivilegesSlice())
}
