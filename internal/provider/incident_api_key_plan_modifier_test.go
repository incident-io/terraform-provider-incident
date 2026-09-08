package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestAPIKeyRotationPlanned covers which token_version changes ask for a rotation. Update
// and the plan modifier both consult this, and they have to agree: if the plan promises the
// stored token and the apply rotates anyway, Terraform reports the provider as inconsistent.
func TestAPIKeyRotationPlanned(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		planned types.Int64
		state   types.Int64
		want    bool
	}{
		{
			name:    "version incremented",
			planned: types.Int64Value(2),
			state:   types.Int64Value(1),
			want:    true,
		},
		{
			// Any change asks for a rotation, not just an increase: the number is a version,
			// and reusing an old one still means "this is not the token you had".
			name:    "version decreased",
			planned: types.Int64Value(1),
			state:   types.Int64Value(2),
			want:    true,
		},
		{
			name:    "version unchanged",
			planned: types.Int64Value(2),
			state:   types.Int64Value(2),
			want:    false,
		},
		{
			// Adopting a version for a key that had none - an imported key, or one whose
			// config is taking over its rotation - is a rotation, and the only way to get a
			// token for a key Terraform imported.
			name:    "version newly set",
			planned: types.Int64Value(1),
			state:   types.Int64Null(),
			want:    true,
		},
		{
			// Dropping the version hands the token back rather than asking for a new one.
			name:    "version removed",
			planned: types.Int64Null(),
			state:   types.Int64Value(2),
			want:    false,
		},
		{
			name:    "no version either side",
			planned: types.Int64Null(),
			state:   types.Int64Null(),
			want:    false,
		},
		{
			// A version computed elsewhere might turn out to be a rotation, so it's treated
			// as one: promising the old token and then rotating is the failure to avoid.
			name:    "version unknown",
			planned: types.Int64Unknown(),
			state:   types.Int64Value(1),
			want:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := apiKeyRotationPlanned(tc.planned, tc.state); got != tc.want {
				t.Fatalf("apiKeyRotationPlanned(%v, %v) = %v, want %v", tc.planned, tc.state, got, tc.want)
			}
		})
	}
}

// TestAPIKeyRotationAwarePlanModifier covers the modifier that decides whether the token
// holds still across a plan. It stands in for UseStateForUnknown, which would pin the old
// token on the very plan that replaces it.
func TestAPIKeyRotationAwarePlanModifier(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		planVersion  tftypes.Value
		stateVersion tftypes.Value
		// nullState builds the request as a create, which has no prior state at all.
		nullState bool
		// planValueKnown builds the request with the value already decided, which is not
		// the modifier's to overwrite.
		planValueKnown bool
		wantUnknown    bool
	}{
		{
			// Nothing about the token changes, so the plan can promise the stored one and
			// spare the practitioner a "known after apply" on every unrelated edit.
			name:         "no rotation keeps the stored token",
			planVersion:  tftypes.NewValue(tftypes.Number, 1),
			stateVersion: tftypes.NewValue(tftypes.Number, 1),
			wantUnknown:  false,
		},
		{
			name:         "rotation leaves the token unknown",
			planVersion:  tftypes.NewValue(tftypes.Number, 2),
			stateVersion: tftypes.NewValue(tftypes.Number, 1),
			wantUnknown:  true,
		},
		{
			name:         "adopting a version leaves the token unknown",
			planVersion:  tftypes.NewValue(tftypes.Number, 1),
			stateVersion: tftypes.NewValue(tftypes.Number, nil),
			wantUnknown:  true,
		},
		{
			// Dropping the version is not a rotation, so the token stays as it was.
			name:         "dropping the version keeps the stored token",
			planVersion:  tftypes.NewValue(tftypes.Number, nil),
			stateVersion: tftypes.NewValue(tftypes.Number, 2),
			wantUnknown:  false,
		},
		{
			// A create has nothing to carry forward, and the token is genuinely not known
			// until the API issues it.
			name:        "create leaves the token unknown",
			nullState:   true,
			wantUnknown: true,
		},
		{
			// The framework already knows the value, so the modifier must leave it alone
			// even though this plan rotates.
			name:           "a known planned value is left alone",
			planVersion:    tftypes.NewValue(tftypes.Number, 2),
			stateVersion:   tftypes.NewValue(tftypes.Number, 1),
			planValueKnown: true,
			wantUnknown:    false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				keySchema     = apiKeySchema(t)
				storedToken   = "inc_stored"
				planOverrides = map[string]tftypes.Value{}
			)

			if tc.planVersion.Type() != nil {
				planOverrides["token_version"] = tc.planVersion
			}

			objType, planRaw := apiKeyObject(t, planOverrides)

			stateRaw := tftypes.NewValue(objType, nil)
			if !tc.nullState {
				stateOverrides := map[string]tftypes.Value{
					"token": tftypes.NewValue(tftypes.String, storedToken),
				}
				if tc.stateVersion.Type() != nil {
					stateOverrides["token_version"] = tc.stateVersion
				}

				_, stateRaw = apiKeyObject(t, stateOverrides)
			}

			planValue := types.StringUnknown()
			if tc.planValueKnown {
				planValue = types.StringValue("inc_decided")
			}

			resp := planmodifier.StringResponse{PlanValue: planValue}
			apiKeyRotationAware{}.PlanModifyString(
				context.Background(),
				planmodifier.StringRequest{
					Plan:       tfsdk.Plan{Schema: keySchema, Raw: planRaw},
					State:      tfsdk.State{Schema: keySchema, Raw: stateRaw},
					PlanValue:  planValue,
					StateValue: types.StringValue(storedToken),
				},
				&resp,
			)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %+v", resp.Diagnostics)
			}

			if tc.wantUnknown {
				if !resp.PlanValue.IsUnknown() {
					t.Fatalf("expected the token to stay unknown, got %q", resp.PlanValue)
				}

				return
			}

			if resp.PlanValue.IsUnknown() {
				t.Fatal("expected the token to be known, got unknown")
			}

			want := storedToken
			if tc.planValueKnown {
				want = "inc_decided"
			}
			if resp.PlanValue.ValueString() != want {
				t.Fatalf("expected the planned token to be %q, got %q", want, resp.PlanValue.ValueString())
			}
		})
	}
}
