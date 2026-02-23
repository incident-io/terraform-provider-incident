package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	OwningTeamIDs     types.Set                          `tfsdk:"owning_team_ids"`
}

func (AlertSourceResourceModel) FromAPI(source client.AlertSourceV2) AlertSourceResourceModel {
	var emailAddress *string
	if source.EmailOptions != nil {
		emailAddress = &source.EmailOptions.EmailAddress
	}

	var visibleToTeams *IncidentEngineParamBinding
	if source.Template.VisibleToTeams != nil {
		visibleToTeams = lo.ToPtr(IncidentEngineParamBinding{}.FromAPI(*source.Template.VisibleToTeams))
	}

	owningTeamIDs := types.SetNull(types.StringType)
	if source.OwningTeamIds != nil {
		teamIDValues := []attr.Value{}
		for _, teamID := range *source.OwningTeamIds {
			teamIDValues = append(teamIDValues, types.StringValue(teamID))
		}

		owningTeamIDs, _ = types.SetValue(types.StringType, teamIDValues)
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
		OwningTeamIDs:     owningTeamIDs,
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
		IsPrivate:      template.IsPrivate.ValueBoolPointer(),
		VisibleToTeams: visibleToTeams,
	}
}

// AlertTemplateAttributeBinding extends IncidentEngineParamBinding with merge_strategy.
type AlertTemplateAttributeBinding struct {
	ArrayValue    []IncidentEngineParamBindingValue `tfsdk:"array_value"`
	Value         *IncidentEngineParamBindingValue  `tfsdk:"value"`
	MergeStrategy types.String                      `tfsdk:"merge_strategy"`
}

func (AlertTemplateAttributeBinding) FromAPI(pb client.AlertTemplateAttributeBindingV2) AlertTemplateAttributeBinding {
	var arrayValue []IncidentEngineParamBindingValue
	if pb.ArrayValue != nil {
		for _, v := range *pb.ArrayValue {
			arrayValue = append(arrayValue, IncidentEngineParamBindingValue{
				Literal:   types.StringPointerValue(v.Literal),
				Reference: types.StringPointerValue(v.Reference),
			})
		}
	}

	var value *IncidentEngineParamBindingValue
	if pb.Value != nil {
		value = lo.ToPtr(IncidentEngineParamBindingValue{}.FromAPI(*pb.Value))
	}

	var mergeStrategy types.String
	if pb.MergeStrategy != nil {
		mergeStrategy = types.StringValue(string(*pb.MergeStrategy))
	} else {
		mergeStrategy = types.StringNull()
	}

	return AlertTemplateAttributeBinding{
		ArrayValue:    arrayValue,
		Value:         value,
		MergeStrategy: mergeStrategy,
	}
}

func (binding AlertTemplateAttributeBinding) ToPayload() client.AlertTemplateAttributeBindingPayloadV2 {
	arrayValue := []client.EngineParamBindingValuePayloadV2{}
	for _, v := range binding.ArrayValue {
		arrayValue = append(arrayValue, v.ToPayload())
	}

	var value *client.EngineParamBindingValuePayloadV2
	if binding.Value != nil {
		value = lo.ToPtr(binding.Value.ToPayload())
	}

	var mergeStrategy *client.AlertTemplateAttributeBindingPayloadV2MergeStrategy
	if !binding.MergeStrategy.IsNull() && !binding.MergeStrategy.IsUnknown() {
		ms := client.AlertTemplateAttributeBindingPayloadV2MergeStrategy(binding.MergeStrategy.ValueString())
		mergeStrategy = &ms
	}

	return client.AlertTemplateAttributeBindingPayloadV2{
		ArrayValue:    &arrayValue,
		Value:         value,
		MergeStrategy: mergeStrategy,
	}
}

type AlertTemplateAttributeModel struct {
	AlertAttributeID types.String                  `tfsdk:"alert_attribute_id"`
	Binding          AlertTemplateAttributeBinding `tfsdk:"binding"`
}

type AlertTemplateAttributesModel []AlertTemplateAttributeModel

func (AlertTemplateAttributesModel) FromAPI(data []client.AlertTemplateAttributeV2) AlertTemplateAttributesModel {
	out := AlertTemplateAttributesModel{}

	for _, attr := range data {
		out = append(out, AlertTemplateAttributeModel{
			AlertAttributeID: types.StringValue(attr.AlertAttributeId),
			Binding:          AlertTemplateAttributeBinding{}.FromAPI(attr.Binding),
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
