package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// AlertSourceResourceModel represents the complete alert source resource structure
type AlertSourceResourceModel struct {
	ID           types.String                 `tfsdk:"id"`
	Name         types.String                 `tfsdk:"name"`
	SourceType   types.String                 `tfsdk:"source_type"`
	SecretToken  types.String                 `tfsdk:"secret_token"`
	Template     *AlertTemplateModel          `tfsdk:"template"`
	JiraOptions  *AlertSourceJiraOptionsModel `tfsdk:"jira_options"`
	EmailAddress types.String                 `tfsdk:"email_address"`
}

func (AlertSourceResourceModel) FromAPI(source client.AlertSourceV2) AlertSourceResourceModel {
	var emailAddress *string
	if source.EmailOptions != nil {
		emailAddress = &source.EmailOptions.EmailAddress
	}

	return AlertSourceResourceModel{
		ID:          types.StringValue(source.Id),
		Name:        types.StringValue(source.Name),
		SourceType:  types.StringValue(string(source.SourceType)),
		SecretToken: types.StringPointerValue(source.SecretToken),
		Template: &AlertTemplateModel{
			Title:        IncidentEngineParamBindingValue{}.FromAPI(source.Template.Title),
			Description:  IncidentEngineParamBindingValue{}.FromAPI(source.Template.Description),
			Attributes:   AlertTemplateAttributesModel{}.FromAPI(source.Template.Attributes),
			Expresssions: IncidentEngineExpressions{}.FromAPI(source.Template.Expressions),
		},
		JiraOptions:  AlertSourceJiraOptionsModel{}.FromAPI(source.JiraOptions),
		EmailAddress: types.StringPointerValue(emailAddress),
	}
}

// AlertTemplateModel represents the template configuration for an alert source
type AlertTemplateModel struct {
	Expresssions IncidentEngineExpressions       `tfsdk:"expressions"`
	Title        IncidentEngineParamBindingValue `tfsdk:"title"`
	Description  IncidentEngineParamBindingValue `tfsdk:"description"`
	Attributes   AlertTemplateAttributesModel    `tfsdk:"attributes"`
}

func (template AlertTemplateModel) ToPayload() client.AlertTemplatePayloadV2 {
	return client.AlertTemplatePayloadV2{
		Expressions: template.Expresssions.ToPayload(),
		Title:       template.Title.ToPayload(),
		Description: template.Description.ToPayload(),
		Attributes:  template.Attributes.ToPayload(),
	}
}

// AlertTemplateAttributeModel represents a custom attribute in the alert template
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

// AlertSourceJiraOptionsModel represents Jira-specific configuration
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
