package models

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// A template edited in the dashboard can store its fields out of rank order, and with gaps
// between ranks. The list has to come back in rank order, because that's the order on the
// post, and a write has to number it from one in list order.
func TestAnnouncementTemplateModelOrdersByRank(t *testing.T) {
	template := client.AnnouncementTemplateV2{
		Id:   "01FCNDV6P870EA6S7TK1DSYDG0",
		Name: "Major incidents",
		Fields: []client.AnnouncementTemplateFieldV2{
			{FieldType: "announcement_post_fields_status", Rank: 30},
			{FieldType: "announcement_post_fields_severity", Rank: 10, Emoji: lo.ToPtr("fire")},
			{FieldType: "announcement_post_fields_rich_text", Rank: 20, RichText: &client.AnnouncementTemplateRichTextV2{
				Type:     "markdown",
				Contents: "Join {{incident.reference}}",
			}},
		},
		Actions: []client.AnnouncementTemplateActionV2{
			{ActionType: "announcement_post_actions_subscribe", Rank: 5},
			{ActionType: "announcement_post_actions_join_call", Rank: 1},
		},
		OwningTeamIds: []string{},
	}

	model := AnnouncementTemplateModel{}.FromAPI(template)

	assert.Equal(t, []string{
		"announcement_post_fields_severity",
		"announcement_post_fields_rich_text",
		"announcement_post_fields_status",
	}, lo.Map(model.Fields, func(field AnnouncementTemplateFieldModel, _ int) string { return field.FieldType.ValueString() }))
	assert.Equal(t, "fire", model.Fields[0].Emoji.ValueString())
	assert.Nil(t, model.Fields[0].RichText)
	assert.Equal(t, "Join {{incident.reference}}", model.Fields[1].RichText.Contents.ValueString())
	assert.Equal(t, "announcement_post_actions_join_call", model.Actions[0].ActionType.ValueString())

	payload := model.ToUpdatePayload()

	assert.Equal(t, []int64{1, 2, 3}, lo.Map(payload.Fields, func(field client.AnnouncementTemplateFieldPayloadV2, _ int) int64 { return field.Rank }))
	assert.Equal(t, client.AnnouncementTemplateFieldPayloadV2FieldType("announcement_post_fields_severity"), payload.Fields[0].FieldType)
	assert.Equal(t, "markdown", string(payload.Fields[1].RichText.Type))
	assert.Equal(t, []int64{1, 2}, lo.Map(payload.Actions, func(action client.AnnouncementTemplateActionPayloadV2, _ int) int64 { return action.Rank }))
	// Owning teams are always sent, so an empty set clears them rather than being ignored.
	assert.NotNil(t, payload.OwningTeamIds)
}
