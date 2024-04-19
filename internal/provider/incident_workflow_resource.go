package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentWorkflowResource{}
	_ resource.ResourceWithImportState = &IncidentWorkflowResource{}
)

type IncidentWorkflowResource struct {
	_ *client.ClientWithResponses
}

func NewIncidentWorkflowResource() resource.Resource {
	return &IncidentWorkflowResource{}
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

func (i *IncidentWorkflowResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (i *IncidentWorkflowResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Workflows V2"),
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{},
			"name": schema.StringAttribute{},
			"trigger": schema.SingleNestedAttribute{
				Required: true,
				Attributes: map[string]schema.Attribute{
					"name":        schema.StringAttribute{},
					"icon":        schema.StringAttribute{},
					"label":       schema.StringAttribute{},
					"group_label": schema.StringAttribute{},
				},
			},
			"folder":                          schema.StringAttribute{},
			"version":                         schema.Int64Attribute{},
			"delay_for_seconds":               schema.Int64Attribute{},
			"conditions_apply_over_delay":     schema.BoolAttribute{},
			"include_private_incidents":       schema.BoolAttribute{},
			"include_test_incidents":          schema.BoolAttribute{},
			"include_retrospective_incidents": schema.BoolAttribute{},
			"runs_on_incidents":               schema.BoolAttribute{},
			"runs_from":                       schema.StringAttribute{},
			"terraform_repo_url":              schema.StringAttribute{},
			"once_for": schema.SetNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key":         schema.StringAttribute{},
						"label":       schema.StringAttribute{},
						"node_label":  schema.StringAttribute{},
						"type":        schema.StringAttribute{},
						"hide_filter": schema.BoolAttribute{},
						"array":       schema.BoolAttribute{},
						"parent":      schema.StringAttribute{},
						"icon":        schema.StringAttribute{},
					},
				},
			},
			"is_draft":    schema.BoolAttribute{},
			"created_at":  schema.StringAttribute{},
			"updated_at":  schema.StringAttribute{},
			"disabled_at": schema.StringAttribute{},
		},
	}
}

func (i *IncidentWorkflowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	panic("unimplemented")
}

func (i *IncidentWorkflowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	panic("unimplemented")
}

func (i *IncidentWorkflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	panic("unimplemented")
}

func (i *IncidentWorkflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	panic("unimplemented")
}

func (i *IncidentWorkflowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	panic("unimplemented")
}