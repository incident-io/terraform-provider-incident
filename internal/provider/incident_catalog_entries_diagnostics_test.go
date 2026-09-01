package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildUpdateAttributes skips a null managed_attributes, but an unknown one — which is what
// Terraform hands us when the value comes from a resource that hasn't been created yet —
// reaches ElementsAs, which cannot convert unknown into []string. That used to panic, which
// crashes the plugin and shows the user a Go stack trace instead of a diagnostic.
func TestBuildUpdateAttributesRejectsUnknownWithoutPanicking(t *testing.T) {
	model := &IncidentCatalogEntriesResourceModel{
		ManagedAttributes: types.SetUnknown(types.StringType),
	}

	got, err := model.buildUpdateAttributes(context.Background())

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "unable to read managed_attributes")
}

func TestBuildUpdateAttributes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value types.Set
		want  *[]string
	}{
		{
			name:  "null is left to the API",
			value: types.SetNull(types.StringType),
			want:  nil,
		},
		{
			name:  "empty means manage nothing",
			value: types.SetValueMust(types.StringType, nil),
			want:  &[]string{},
		},
		{
			name: "values are passed through",
			value: types.SetValueMust(types.StringType, []attr.Value{
				types.StringValue("attr_one"),
				types.StringValue("attr_two"),
			}),
			want: &[]string{"attr_one", "attr_two"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &IncidentCatalogEntriesResourceModel{ManagedAttributes: tc.value}

			got, err := model.buildUpdateAttributes(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
