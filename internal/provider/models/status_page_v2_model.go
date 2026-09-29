package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// StatusPageModel is the Terraform model for a status page. Status pages are created in
// the dashboard rather than through the API, so there is no resource: the data source is
// the only thing that reads one.
type StatusPageModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	PublicURL   types.String `tfsdk:"public_url"`
}

// FromAPI converts an API status page into the Terraform model. The description and
// public URL are optional in the API, so an absent one is null rather than empty.
func (StatusPageModel) FromAPI(page client.StatusPageV2) StatusPageModel {
	return StatusPageModel{
		ID:          types.StringValue(page.Id),
		Name:        types.StringValue(page.Name),
		Description: types.StringPointerValue(page.Description),
		PublicURL:   types.StringPointerValue(page.PublicUrl),
	}
}
