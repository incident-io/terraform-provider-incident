package models

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/richtexttypes"
)

// The data source mirror of TemplatedTextAttribute. A data source's schema comes from a
// different package to a resource's, so the two can't share one definition even though
// they describe the same thing: what's shared is the model they decode into, and
// TestAlertSourceDataSourceSchemaMatchesModel is what holds the pair together.

// TemplatedTextDataSourceAttribute is a rich text field as a data source reads it. Unlike
// the resource's, every part is Computed: a data source describes what the API holds
// rather than what an author asked for.
func TemplatedTextDataSourceAttribute(description, featureSet string) schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:    true,
		Description: description,
		Attributes:  TemplatedTextValueDataSourceAttributes(featureSet),
	}
}

func TemplatedTextValueDataSourceAttributes(featureSet string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"literal": schema.StringAttribute{
			CustomType: richtexttypes.TemplatedTextType{},
			Computed:   true,
			Description: fmt.Sprintf(
				"Fixed content, which may interpolate the scope with `{{ variable }}`. Filters "+
					"`truncate: N` and `omit_if_unset` are supported. For content needing formatting "+
					"a template can't express, pass a document from "+
					"`data.incident_rich_text` with `feature_set = %q`.",
				featureSet,
			),
		},
		"reference": schema.StringAttribute{
			Computed: true,
			Description: "A reference into the scope whose value becomes the content, such as " +
				"`payload.summary`.",
		},
	}
}
