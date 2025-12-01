package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

type AlertSourceResourceModel struct {
	ID                types.String                       `tfsdk:"id"`
	Name              types.String                       `tfsdk:"name"`
	SourceType        types.String                       `tfsdk:"source_type"`
	SecretToken       types.String                       `tfsdk:"secret_token"`
	Template          *AlertTemplateModel                `tfsdk:"template"`
	JiraOptions       *AlertSourceJiraOptionsModel       `tfsdk:"jira_options"`
	EmailAddress      types.String                       `tfsdk:"email_address"`
	HTTPCustomOptions *AlertSourceHTTPCustomOptionsModel `tfsdk:"http_custom_options"`
}

func (AlertSourceResourceModel) FromAPI(source client.AlertSourceV2) AlertSourceResourceModel {
	var emailAddress *string
	if source.EmailOptions != nil {
		emailAddress = &source.EmailOptions.EmailAddress
	}

	var visibleToTeams *IncidentEngineParamBinding
	if source.Template.VisibleToTeams != nil {
		v := IncidentEngineParamBinding{}.FromAPI(*source.Template.VisibleToTeams)
		visibleToTeams = &v
	}

	return AlertSourceResourceModel{
		ID:          types.StringValue(source.Id),
		Name:        types.StringValue(source.Name),
		SourceType:  types.StringValue(string(source.SourceType)),
		SecretToken: types.StringPointerValue(source.SecretToken),
		Template: &AlertTemplateModel{
			Title:          IncidentEngineParamBindingValue{}.FromAPI(source.Template.Title),
			Description:    IncidentEngineParamBindingValue{}.FromAPI(source.Template.Description),
			Attributes:     AlertTemplateAttributesModel{}.FromAPI(source.Template.Attributes),
			Expressions:    IncidentEngineExpressions{}.FromAPI(source.Template.Expressions),
			IsPrivate:      types.BoolValue(source.Template.IsPrivate),
			VisibleToTeams: visibleToTeams,
		},
		JiraOptions:       AlertSourceJiraOptionsModel{}.FromAPI(source.JiraOptions),
		EmailAddress:      types.StringPointerValue(emailAddress),
		HTTPCustomOptions: AlertSourceHTTPCustomOptionsModel{}.FromAPI(source.HttpCustomOptions),
	}
}

type AlertTemplateModel struct {
	Expressions    IncidentEngineExpressions       `tfsdk:"expressions"`
	Title          IncidentEngineParamBindingValue `tfsdk:"title"`
	Description    IncidentEngineParamBindingValue `tfsdk:"description"`
	Attributes     AlertTemplateAttributesModel    `tfsdk:"attributes"`
	IsPrivate      types.Bool                      `tfsdk:"is_private"`
	VisibleToTeams *IncidentEngineParamBinding     `tfsdk:"visible_to_teams"`
}

func (template AlertTemplateModel) ToPayload() client.AlertTemplatePayloadV2 {
	var visibleToTeams *client.EngineParamBindingPayloadV2
	if template.VisibleToTeams != nil {
		visibleToTeams = lo.ToPtr(template.VisibleToTeams.ToPayload())
	}

	return client.AlertTemplatePayloadV2{
		Expressions:    template.Expressions.ToPayload(),
		Title:          template.Title.ToPayload(),
		Description:    template.Description.ToPayload(),
		Attributes:     template.Attributes.ToPayload(),
		IsPrivate:      lo.ToPtr(template.IsPrivate.ValueBool()),
		VisibleToTeams: visibleToTeams,
	}
}

type AlertTemplateAttributeModel struct {
	AlertAttributeID types.String               `tfsdk:"alert_attribute_id"`
	Binding          IncidentEngineParamBinding `tfsdk:"binding"`
}

type AlertTemplateAttributesModel []AlertTemplateAttributeModel

func (AlertTemplateAttributesModel) FromAPI(data []client.AlertTemplateAttributeV2) AlertTemplateAttributesModel {
	out := AlertTemplateAttributesModel{}

	for _, attr := range data {
		out = append(out, AlertTemplateAttributeModel{
			AlertAttributeID: types.StringValue(attr.AlertAttributeId),
			Binding:          IncidentEngineParamBinding{}.FromAPI(attr.Binding),
		})
	}

	return out
}

func (attributes AlertTemplateAttributesModel) ToPayload() []client.AlertTemplateAttributePayloadV2 {
	out := []client.AlertTemplateAttributePayloadV2{}

	for _, attr := range attributes {
		out = append(out, client.AlertTemplateAttributePayloadV2{
			AlertAttributeId: attr.AlertAttributeID.ValueString(),
			Binding:          attr.Binding.ToPayload(),
		})
	}

	return out
}

type AlertSourceJiraOptionsModel struct {
	ProjectIDs []types.String `tfsdk:"project_ids"`
}

func (AlertSourceJiraOptionsModel) FromAPI(opts *client.AlertSourceJiraOptionsV2) *AlertSourceJiraOptionsModel {
	if opts == nil {
		return nil
	}

	projectIDs := []types.String{}
	for _, projectID := range opts.ProjectIds {
		projectIDs = append(projectIDs, types.StringValue(projectID))
	}

	return &AlertSourceJiraOptionsModel{
		ProjectIDs: projectIDs,
	}
}

func (opts *AlertSourceJiraOptionsModel) ToPayload() *client.AlertSourceJiraOptionsV2 {
	if opts == nil {
		return nil
	}

	projectIDs := []string{}
	for _, projectID := range opts.ProjectIDs {
		projectIDs = append(projectIDs, projectID.ValueString())
	}

	return &client.AlertSourceJiraOptionsV2{
		ProjectIds: projectIDs,
	}
}

type AlertSourceHTTPCustomOptionsModel struct {
	TransformExpression  types.String `tfsdk:"transform_expression"`
	DeduplicationKeyPath types.String `tfsdk:"deduplication_key_path"`
}

func (AlertSourceHTTPCustomOptionsModel) FromAPI(opts *client.AlertSourceHTTPCustomOptionsV2) *AlertSourceHTTPCustomOptionsModel {
	if opts == nil {
		return nil
	}

	return &AlertSourceHTTPCustomOptionsModel{
		TransformExpression:  types.StringValue(opts.TransformExpression),
		DeduplicationKeyPath: types.StringValue(opts.DeduplicationKeyPath),
	}
}

func (opts *AlertSourceHTTPCustomOptionsModel) ToPayload() *client.AlertSourceHTTPCustomOptionsV2 {
	if opts == nil {
		return nil
	}

	return &client.AlertSourceHTTPCustomOptionsV2{
		TransformExpression:  opts.TransformExpression.ValueString(),
		DeduplicationKeyPath: opts.DeduplicationKeyPath.ValueString(),
	}
}
