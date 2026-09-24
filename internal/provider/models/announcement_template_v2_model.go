package models

import (
	"cmp"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// AnnouncementTemplateModel is the Terraform model for an announcement template, shared by
// the resource and the data source.
//
// Fields and actions are lists whose order is the order they appear on the post. The API
// orders them by an explicit rank instead, which the provider derives from the position in
// the list rather than asking for it: a rank the config had to keep in step with the list
// would be one more thing to get wrong.
type AnnouncementTemplateModel struct {
	ID            types.String                      `tfsdk:"id"`
	Name          types.String                      `tfsdk:"name"`
	IsDefault     types.Bool                        `tfsdk:"is_default"`
	Fields        []AnnouncementTemplateFieldModel  `tfsdk:"fields"`
	Actions       []AnnouncementTemplateActionModel `tfsdk:"actions"`
	OwningTeamIDs types.Set                         `tfsdk:"owning_team_ids"`
}

type AnnouncementTemplateFieldModel struct {
	FieldType           types.String                       `tfsdk:"field_type"`
	Emoji               types.String                       `tfsdk:"emoji"`
	CustomFieldID       types.String                       `tfsdk:"custom_field_id"`
	IncidentRoleID      types.String                       `tfsdk:"incident_role_id"`
	IncidentTimestampID types.String                       `tfsdk:"incident_timestamp_id"`
	RichText            *AnnouncementTemplateRichTextModel `tfsdk:"rich_text"`
}

// AnnouncementTemplateRichTextModel is a rich text field's content. It mirrors the API's
// union, keyed on type, so a new way of writing rich text is a new type rather than a new
// attribute.
type AnnouncementTemplateRichTextModel struct {
	Type     types.String `tfsdk:"type"`
	Contents types.String `tfsdk:"contents"`
}

type AnnouncementTemplateActionModel struct {
	ActionType types.String `tfsdk:"action_type"`
	Emoji      types.String `tfsdk:"emoji"`
}

// FromAPI converts an API announcement template into the Terraform model, ordering fields
// and actions by rank.
func (AnnouncementTemplateModel) FromAPI(template client.AnnouncementTemplateV2) AnnouncementTemplateModel {
	fields := slices.Clone(template.Fields)
	slices.SortStableFunc(fields, func(a, b client.AnnouncementTemplateFieldV2) int { return cmp.Compare(a.Rank, b.Rank) })

	actions := slices.Clone(template.Actions)
	slices.SortStableFunc(actions, func(a, b client.AnnouncementTemplateActionV2) int { return cmp.Compare(a.Rank, b.Rank) })

	return AnnouncementTemplateModel{
		ID:        types.StringValue(template.Id),
		Name:      types.StringValue(template.Name),
		IsDefault: types.BoolValue(template.IsDefault),
		Fields: lo.Map(fields, func(field client.AnnouncementTemplateFieldV2, _ int) AnnouncementTemplateFieldModel {
			model := AnnouncementTemplateFieldModel{
				FieldType:           types.StringValue(string(field.FieldType)),
				Emoji:               types.StringPointerValue(field.Emoji),
				CustomFieldID:       types.StringPointerValue(field.CustomFieldId),
				IncidentRoleID:      types.StringPointerValue(field.IncidentRoleId),
				IncidentTimestampID: types.StringPointerValue(field.IncidentTimestampId),
			}
			if field.RichText != nil {
				model.RichText = &AnnouncementTemplateRichTextModel{
					Type:     types.StringValue(string(field.RichText.Type)),
					Contents: types.StringValue(field.RichText.Contents),
				}
			}

			return model
		}),
		Actions: lo.Map(actions, func(action client.AnnouncementTemplateActionV2, _ int) AnnouncementTemplateActionModel {
			return AnnouncementTemplateActionModel{
				ActionType: types.StringValue(string(action.ActionType)),
				Emoji:      types.StringPointerValue(action.Emoji),
			}
		}),
		OwningTeamIDs: announcementStringSet(template.OwningTeamIds),
	}
}

// ToCreatePayload converts the Terraform model to an API create payload.
func (m AnnouncementTemplateModel) ToCreatePayload() client.AnnouncementTemplatesCreatePayloadV2 {
	return client.AnnouncementTemplatesCreatePayloadV2{
		Name:          m.Name.ValueString(),
		Fields:        lo.ToPtr(m.fieldsPayload()),
		Actions:       lo.ToPtr(m.actionsPayload()),
		OwningTeamIds: lo.ToPtr(announcementSetStrings(m.OwningTeamIDs)),
	}
}

// ToUpdatePayload converts the Terraform model to an API update payload. The API replaces
// fields and actions wholesale, so the payload carries the full set.
func (m AnnouncementTemplateModel) ToUpdatePayload() client.AnnouncementTemplatesUpdatePayloadV2 {
	return client.AnnouncementTemplatesUpdatePayloadV2{
		Name:          m.Name.ValueString(),
		Fields:        m.fieldsPayload(),
		Actions:       m.actionsPayload(),
		OwningTeamIds: lo.ToPtr(announcementSetStrings(m.OwningTeamIDs)),
	}
}

func (m AnnouncementTemplateModel) fieldsPayload() []client.AnnouncementTemplateFieldPayloadV2 {
	return lo.Map(m.Fields, func(field AnnouncementTemplateFieldModel, i int) client.AnnouncementTemplateFieldPayloadV2 {
		payload := client.AnnouncementTemplateFieldPayloadV2{
			FieldType:           client.AnnouncementTemplateFieldPayloadV2FieldType(field.FieldType.ValueString()),
			Rank:                int64(i + 1),
			Emoji:               knownStringPtr(field.Emoji),
			CustomFieldId:       knownStringPtr(field.CustomFieldID),
			IncidentRoleId:      knownStringPtr(field.IncidentRoleID),
			IncidentTimestampId: knownStringPtr(field.IncidentTimestampID),
		}
		if field.RichText != nil {
			payload.RichText = &client.AnnouncementTemplateRichTextV2{
				Type:     client.AnnouncementTemplateRichTextV2Type(field.RichText.Type.ValueString()),
				Contents: field.RichText.Contents.ValueString(),
			}
		}

		return payload
	})
}

func (m AnnouncementTemplateModel) actionsPayload() []client.AnnouncementTemplateActionPayloadV2 {
	return lo.Map(m.Actions, func(action AnnouncementTemplateActionModel, i int) client.AnnouncementTemplateActionPayloadV2 {
		return client.AnnouncementTemplateActionPayloadV2{
			ActionType: client.AnnouncementTemplateActionPayloadV2ActionType(action.ActionType.ValueString()),
			Rank:       int64(i + 1),
			Emoji:      knownStringPtr(action.Emoji),
		}
	})
}
