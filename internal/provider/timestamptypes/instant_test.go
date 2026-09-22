package timestamptypes

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstantSemanticEquals(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		current, next string
		want          bool
	}{
		{name: "identical", current: "2026-12-25T00:00:00Z", next: "2026-12-25T00:00:00Z", want: true},
		{name: "same moment in an offset", current: "2027-01-01T00:00:00+01:00", next: "2026-12-31T23:00:00Z", want: true},
		{name: "same moment either way round", current: "2026-12-31T23:00:00Z", next: "2027-01-01T00:00:00+01:00", want: true},
		{name: "a different moment", current: "2026-12-25T00:00:00Z", next: "2026-12-26T00:00:00Z", want: false},
		{name: "the offset ignored would be the same", current: "2027-01-01T00:00:00+01:00", next: "2027-01-01T00:00:00Z", want: false},
		{name: "unparseable is never equal", current: "yesterday", next: "2026-12-25T00:00:00Z", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, diags := NewInstantStringValue(tc.current).StringSemanticEquals(context.Background(), NewInstantStringValue(tc.next))
			require.False(t, diags.HasError(), diags)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestInstantSemanticEqualsRejectsOtherTypes(t *testing.T) {
	t.Parallel()

	_, diags := NewInstantStringValue("2026-12-25T00:00:00Z").StringSemanticEquals(context.Background(), basetypes.NewStringValue("2026-12-25T00:00:00Z"))
	assert.True(t, diags.HasError())
}

func TestInstantValidateAttribute(t *testing.T) {
	t.Parallel()

	validate := func(value Instant) bool {
		var resp xattr.ValidateAttributeResponse
		value.ValidateAttribute(context.Background(), xattr.ValidateAttributeRequest{Path: path.Root("start_at")}, &resp)

		return resp.Diagnostics.HasError()
	}

	assert.False(t, validate(NewInstantStringValue("2026-12-25T00:00:00Z")))
	assert.False(t, validate(NewInstantStringValue("2026-12-25T09:30:00.5+05:30")))
	assert.False(t, validate(NewInstantNull()), "null is for the framework to judge")
	assert.False(t, validate(NewInstantUnknown()), "unknown isn't known yet")
	assert.True(t, validate(NewInstantStringValue("25/12/2026")))
	assert.True(t, validate(NewInstantStringValue("2026-12-25")), "a date alone is not an instant")
}

func TestInstantValueTime(t *testing.T) {
	t.Parallel()

	got, ok := NewInstantStringValue("2027-01-01T00:00:00+01:00").ValueTime()
	require.True(t, ok)
	assert.True(t, got.Equal(time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)))

	_, ok = NewInstantNull().ValueTime()
	assert.False(t, ok)

	_, ok = NewInstantStringValue("soon").ValueTime()
	assert.False(t, ok)
}

func TestNewInstantTimeValue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "2026-12-25T00:00:00Z", NewInstantTimeValue(time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)).ValueString())

	paris := time.FixedZone("CET", 3600)
	assert.Equal(t, "2027-01-01T00:00:00+01:00", NewInstantTimeValue(time.Date(2027, 1, 1, 0, 0, 0, 0, paris)).ValueString())
}
