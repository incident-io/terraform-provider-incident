package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentWorkflowResource{}
	_ resource.ResourceWithImportState = &IncidentWorkflowResource{}
)

type IncidentWorkflowResource struct {
	client *client.ClientWithResponses
}

type IncidentWorkflowResourceModel struct {
	ID                            types.String                     `tfsdk:"id"`
	Name                          types.String                     `tfsdk:"name"`
	Trigger                       IncidentWorkflowTriggerSlimModel `tfsdk:"trigger"`
	Folder                        types.String                     `tfsdk:"folder"`
	Version                       types.Int64                      `tfsdk:"version"`
	DelayForSeconds               types.Int64                      `tfsdk:"delay_for_seconds"`
	ConditionsApplyOverDelay      types.Bool                       `tfsdk:"conditions_apply_over_delay"`
	IncludePrivateIncidents       types.Bool                       `tfsdk:"include_private_incidents"`
	IncludeTestIncidents          types.Bool                       `tfsdk:"include_test_incidents"`
	IncludeRetrospectiveIncidents types.Bool                       `tfsdk:"include_retrospective_incidents"`
	RunsOnIncidents               types.Bool                       `tfsdk:"runs_on_incidents"`
	RunsFrom                      types.String                     `tfsdk:"runs_from"`
	TerraformRepoURL              types.String                     `tfsdk:"terraform_repo_url"`
	OnceFor                       []IncidentEngineReferenceModel   `tfsdk:"once_for"`
	IsDraft                       types.Bool                       `tfsdk:"is_draft"`

	CreatedAt  types.String `tfsdk:"created_at"`
	UpdatedAt  types.String `tfsdk:"updated_at"`
	DisabledAt types.String `tfsdk:"disabled_at"`
}

type IncidentWorkflowTriggerSlimModel struct {
	Name       types.String `tfsdk:"name"`
	Icon       types.String `tfsdk:"icon"`
	Label      types.String `tfsdk:"label"`
	GroupLabel types.String `tfsdk:"group_label"`
}

type IncidentEngineReferenceModel struct {
	Key        types.String `tfsdk:"key"`
	Label      types.String `tfsdk:"label"`
	NodeLabel  types.String `tfsdk:"node_label"`
	Type       types.String `tfsdk:"type"`
	HideFilter types.Bool   `tfsdk:"hide_filter"`
	Array      types.Bool   `tfsdk:"array"`
	Parent     types.String `tfsdk:"parent"`
	Icon       types.String `tfsdk:"icon"`
}

// Create implements resource.Resource.
func (i *IncidentWorkflowResource) Create(context.Context, resource.CreateRequest, *resource.CreateResponse) {
	panic("unimplemented")
}

// Delete implements resource.Resource.
func (i *IncidentWorkflowResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
	panic("unimplemented")
}

// Metadata implements resource.Resource.
func (i *IncidentWorkflowResource) Metadata(context.Context, resource.MetadataRequest, *resource.MetadataResponse) {
	panic("unimplemented")
}

// Read implements resource.Resource.
func (i *IncidentWorkflowResource) Read(context.Context, resource.ReadRequest, *resource.ReadResponse) {
	panic("unimplemented")
}

// Schema implements resource.Resource.
func (i *IncidentWorkflowResource) Schema(context.Context, resource.SchemaRequest, *resource.SchemaResponse) {
	panic("unimplemented")
}

// Update implements resource.Resource.
func (i *IncidentWorkflowResource) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
	panic("unimplemented")
}

// ImportState implements resource.ResourceWithImportState.
func (i *IncidentWorkflowResource) ImportState(context.Context, resource.ImportStateRequest, *resource.ImportStateResponse) {
	panic("unimplemented")
}
