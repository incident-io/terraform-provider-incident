package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentIncidentTemplateDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentIncidentTemplateDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentIncidentTemplateDataSource{}
)

func NewIncidentIncidentTemplateDataSource() datasource.DataSource {
	return &IncidentIncidentTemplateDataSource{}
}

type IncidentIncidentTemplateDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentIncidentTemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_template"
}

func (d *IncidentIncidentTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Incident Templates V1"),
			"Use this data source to look up an existing incident template, either by `id` or by `name`. "+
				"This is useful for referencing a template the dashboard owns — for example pointing an "+
				"`incident_alert_route` at it via `incident_config.incident_template` — without managing "+
				"the template as an `incident_incident_template` resource. Set exactly one of the two lookup "+
				"attributes; setting both, or neither, is rejected at plan time."),
		Attributes: incidentTemplateDataSourceAttributes(),
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are
// Optional and Computed so either can be used, which means setting both would
// otherwise silently ignore one of them.
func (d *IncidentIncidentTemplateDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.IncidentTemplateV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the
	// same apply, or either attribute behind an unresolved conditional — is
	// non-null, so judging it here would reject a config that's actually fine.
	if data.ID.IsUnknown() || data.Name.IsUnknown() {
		return
	}

	switch {
	case !data.ID.IsNull() && !data.Name.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case data.ID.IsNull() && data.Name.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
	}
}

func (d *IncidentIncidentTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.IncidentTemplateV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var template *client.IncidentTemplateV1
	switch {
	case !data.ID.IsNull():
		result, err := d.client.IncidentTemplatesV1ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident template, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident template, unexpected response: %s", result.Status()))
			return
		}

		template = &result.JSON200.IncidentTemplate

	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident template by name, got error: %s", err))
			return
		}

		template = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	// There's no prior state to reconcile against in a data source, so pass nil:
	// we want whatever the API has, including optional bindings the config never set.
	model := models.IncidentTemplateV1Model{}.FromAPI(*template, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// incidentTemplateListPageSize is the largest page the list endpoint accepts.
const incidentTemplateListPageSize = 50

// findByName pages the incident template list looking for an exact name match.
// The endpoint has no name filter, so every page has to be fetched until the name
// turns up or the pages run out.
//
// A template's name is unique within an organisation, so the first match is the
// only one.
func (d *IncidentIncidentTemplateDataSource) findByName(ctx context.Context, name string) (*client.IncidentTemplateV1, error) {
	var after *string

	for {
		result, err := d.client.IncidentTemplatesV1ListWithResponse(ctx, &client.IncidentTemplatesV1ListParams{
			PageSize: lo.ToPtr(int64(incidentTemplateListPageSize)),
			After:    after,
		})
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", result.Body)
		}
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing incident templates: %s", result.Status())
		}

		if match, found := lo.Find(result.JSON200.IncidentTemplates, func(template client.IncidentTemplateV1) bool {
			return template.Name == name
		}); found {
			return &match, nil
		}

		// The endpoint returns an after cursor only while another page exists, so an
		// absent one ends the walk rather than looping on the last page forever.
		if result.JSON200.PaginationMeta == nil || result.JSON200.PaginationMeta.After == nil {
			return nil, fmt.Errorf("no incident template found with name %q", name)
		}
		after = result.JSON200.PaginationMeta.After
	}
}

// incidentTemplateDataSourceAttributes is the attribute set for an incident
// template. id and name are Optional as well as Computed so either can be used
// as the lookup key.
func incidentTemplateDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("IncidentTemplateV1", "id"),
			Optional:            true,
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("IncidentTemplateV1", "name"),
			Optional:            true,
			Computed:            true,
		},
		"expressions": models.ExpressionsDataSourceAttribute(),
		"template": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateV1", "template"),
			Attributes:          incidentTemplateConfigDataSourceAttributes(),
		},
	}
}

func incidentTemplateConfigDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"custom_fields": schema.SetNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "custom_fields"),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"custom_field_id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("IncidentTemplateCustomFieldBindingV1", "custom_field_id"),
					},
					"binding": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: apischema.Docstring("IncidentTemplateCustomFieldBindingV1", "binding"),
						Attributes:          models.ParamBindingDataSourceAttributes(),
					},
					"merge_strategy": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: EnumValuesDescription("IncidentTemplateCustomFieldBindingV1", "merge_strategy"),
					},
				},
			},
		},
		"incident_mode": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "incident_mode"),
			Attributes:          models.ParamBindingDataSourceAttributes(),
		},
		"incident_type": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "incident_type"),
			Attributes:          models.ParamBindingDataSourceAttributes(),
		},
		"name": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "name"),
			Attributes:          models.AutoGeneratedParamBindingDataSourceAttributes(),
		},
		"severity": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "severity"),
			Attributes: map[string]schema.Attribute{
				"binding": schema.SingleNestedAttribute{
					Computed:            true,
					MarkdownDescription: apischema.Docstring("IncidentTemplateSeverityBindingV1", "binding"),
					Attributes:          models.ParamBindingDataSourceAttributes(),
				},
				"merge_strategy": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: EnumValuesDescription("IncidentTemplateSeverityBindingV1", "merge_strategy"),
				},
			},
		},
		"start_in_triage": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "start_in_triage"),
			Attributes:          models.ParamBindingDataSourceAttributes(),
		},
		"summary": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "summary"),
			Attributes:          models.AutoGeneratedParamBindingDataSourceAttributes(),
		},
		"workspace": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentTemplateConfigV1", "workspace"),
			Attributes:          models.ParamBindingDataSourceAttributes(),
		},
	}
}
