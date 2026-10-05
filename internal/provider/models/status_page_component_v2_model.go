package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// StatusPageComponentModel is the Terraform model for a status page component.
//
// Which pages show a component is decided by each page's structure, which the provider
// doesn't manage, so the model is just the component's own name and description.
type StatusPageComponentModel struct {
	ID                types.String `tfsdk:"id"`
	UnlockInDashboard types.Bool   `tfsdk:"unlock_in_dashboard"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
}

// FromAPI converts an API component into the Terraform model.
//
// unlockInDashboard comes from the plan or the prior state: the provider treats the claim
// as configuration rather than reading it back from management_meta.
func (StatusPageComponentModel) FromAPI(component client.StatusPageComponentV2, unlockInDashboard types.Bool) StatusPageComponentModel {
	return StatusPageComponentModel{
		ID:                types.StringValue(component.Id),
		UnlockInDashboard: unlockInDashboard,
		Name:              types.StringValue(component.Name),
		Description:       types.StringPointerValue(component.Description),
	}
}

// ToCreatePayload converts the Terraform model to an API create payload.
func (m StatusPageComponentModel) ToCreatePayload() client.StatusPageComponentsCreatePayloadV2 {
	return client.StatusPageComponentsCreatePayloadV2{
		Name:        m.Name.ValueString(),
		Description: m.descriptionPayload(),
	}
}

// ToUpdatePayload converts the Terraform model to an API update payload. The API reads an
// omitted description as "clear it", so a dropped attribute needs nothing more sent.
func (m StatusPageComponentModel) ToUpdatePayload() client.StatusPageComponentsUpdatePayloadV2 {
	return client.StatusPageComponentsUpdatePayloadV2{
		Name:        m.Name.ValueString(),
		Description: m.descriptionPayload(),
	}
}

func (m StatusPageComponentModel) descriptionPayload() *string {
	if m.Description.IsNull() || m.Description.IsUnknown() {
		return nil
	}

	return lo.ToPtr(m.Description.ValueString())
}

// StatusPageComponentDataSourceModel is the Terraform model for looking up an existing
// component. It has no unlock_in_dashboard: a lookup claims nothing.
type StatusPageComponentDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// FromAPI converts an API component into the data source model.
func (StatusPageComponentDataSourceModel) FromAPI(component client.StatusPageComponentV2) StatusPageComponentDataSourceModel {
	return StatusPageComponentDataSourceModel{
		ID:          types.StringValue(component.Id),
		Name:        types.StringValue(component.Name),
		Description: types.StringPointerValue(component.Description),
	}
}
