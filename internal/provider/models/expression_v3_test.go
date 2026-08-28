package models

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	testNamespace            = AlertAttributeExpressions("01ABCDEF0123456789ABCDEFGH")
	alertSourceTestNamespace = ExpressionNamespace{}
)

// TestExpressionNamespaceSeparatesResources is the property the namespace exists for: two
// resources on one alert source both writing `severity` must store two expressions.
func TestExpressionNamespaceSeparatesResources(t *testing.T) {
	severity := []NamedExpression{{
		Name:       types.StringValue("severity"),
		StartFrom:  types.StringValue("payload"),
		Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
	}}

	stored := func(ns ExpressionNamespace) client.ExpressionPayloadV3 {
		payloads, _, err := ExpressionsToPayload(ns, nil, severity)
		if err != nil {
			t.Fatalf("ExpressionsToPayload: %v", err)
		}
		if len(payloads) != 1 {
			t.Fatalf("wrote %d expressions, want 1", len(payloads))
		}

		return payloads[0]
	}

	first := stored(AlertAttributeExpressions("01AAA"))
	second := stored(AlertAttributeExpressions("01BBB"))

	if first.Reference == second.Reference {
		t.Errorf("two attributes stored the same reference %q", first.Reference)
	}

	// The label is what the dashboard shows, so it is the name as written.
	for _, payload := range []client.ExpressionPayloadV3{first, second} {
		if payload.Label != "severity" {
			t.Errorf("labelled %q, want the name the config wrote", payload.Label)
		}
	}

	if reference := stored(alertSourceTestNamespace).Reference; reference != "severity" {
		t.Errorf("a resource with no namespace stored %q, want the name as written", reference)
	}
}

// TestExpressionNamespaceReservesBoundName covers the collision the namespace introduces: a
// named_expression called "_bound" reaches the unnamed block's reference.
func TestExpressionNamespaceReservesBoundName(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ns         ExpressionNamespace
		expression string
		reserved   bool
	}{
		{
			name:       "a namespaced resource, by a name that collides once prefixed",
			ns:         testNamespace,
			expression: "_bound",
			reserved:   true,
		},
		{
			// No unnamed block of its own, but it shares the source with attributes that have one.
			name:       "a resource with no namespace, by the whole suffix",
			ns:         alertSourceTestNamespace,
			expression: "severity" + BoundExpressionSuffix,
			reserved:   true,
		},
		{
			// An empty id is what an unresolved one reads as at plan time. Its prefix ends in the
			// separator, so a check against the stored name would read "bound" as reserved and
			// reject a config that is legal the moment the id resolves.
			name:       "not a name only an unresolved id would collide with",
			ns:         AlertAttributeExpressions(""),
			expression: "bound",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diags := diag.Diagnostics{}
			ValidateExpressions(tc.ns, nil, path.Empty(), []NamedExpression{{
				Name:       types.StringValue(tc.expression),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			}}, path.Root("named_expression"), &diags)

			if diags.HasError() != tc.reserved {
				t.Errorf("a named_expression called %q: reserved = %v, want %v (%+v)",
					tc.expression, diags.HasError(), tc.reserved, diags)
			}
		})
	}
}

// TestExpressionRoundTrip is the property the grammar rests on: a config must read back as
// itself, or plan shows a change on a config nobody touched. The sugar spellings are where
// that goes wrong, several collapsing to the same payload.
//
// Each case is written in the canonical spelling — the one the read side picks. A wider form
// mapping to the same payload gets rewritten on first apply, and these cases pin down which
// form wins.
func TestExpressionRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bound *Expression
		named []NamedExpression
	}{
		{
			name: "a parse pipeline",
			bound: &Expression{
				StartFrom: types.StringValue("payload"),
				Operations: []Operation{
					{Parse: &Parse{
						Function: types.StringValue("$.service"),
						As:       types.StringValue(`CatalogEntry["01ABC"]`),
						// Absent, not false: they mean the same thing, so only one survives.
						Array: types.BoolNull(),
					}},
				},
			},
		},
		{
			name: "every cardinality operation",
			bound: &Expression{
				StartFrom: types.StringValue("payload.services"),
				Operations: []Operation{
					{Navigate: &Navigate{To: types.StringValue("owner")}},
					{Concatenate: &Concatenate{With: types.StringValue("payload.extra_teams")}},
					{First: &EmptyOpts{}},
					{Count: &EmptyOpts{}},
					{Sum: &EmptyOpts{}},
					{Min: &EmptyOpts{}},
					{Max: &EmptyOpts{}},
					{Random: &EmptyOpts{}},
					{Cast: &Cast{As: types.StringValue("Text")}},
				},
			},
		},
		{
			name: "a filter with the conditions sugar",
			bound: &Expression{
				StartFrom: types.StringValue("payload.teams"),
				Operations: []Operation{
					{Filter: &Conditions{
						Conditions: []Condition{
							{
								Subject:   types.StringValue("input.tier"),
								Operation: types.StringValue("one_of"),
								Params: []Binding{
									{Values: []types.String{types.StringValue("1"), types.StringValue("2")}},
								},
							},
						},
					}},
				},
			},
		},
		{
			name: "a filter with real condition groups",
			bound: &Expression{
				StartFrom: types.StringValue("payload.teams"),
				Operations: []Operation{
					{Filter: &Conditions{
						ConditionGroups: []ConditionGroup{
							{Conditions: []Condition{{
								Subject:   types.StringValue("input.tier"),
								Operation: types.StringValue("is_set"),
							}}},
							{Conditions: []Condition{{
								Subject:   types.StringValue("input.name"),
								Operation: types.StringValue("is_set"),
							}}},
						},
					}},
				},
			},
		},
		{
			name: "branches with if and else_if",
			bound: &Expression{
				StartFrom: types.StringValue("."),
				Operations: []Operation{
					{Branches: &Branches{
						As:    types.StringValue("Text"),
						Array: types.BoolNull(),
						If: &Branch{
							Conditions: []Condition{{
								Subject:   types.StringValue("payload.severity"),
								Operation: types.StringValue("is_set"),
							}},
							Result: &Binding{ValueLiteral: types.StringValue("major")},
						},
						ElseIf: []Branch{{
							Conditions: []Condition{{
								Subject:   types.StringValue("payload.tier"),
								Operation: types.StringValue("is_set"),
							}},
							Result: &Binding{ValueReference: types.StringValue("payload.tier")},
						}},
					}},
				},
			},
		},
		{
			name: "an array-valued branches",
			bound: &Expression{
				StartFrom: types.StringValue("."),
				Operations: []Operation{
					{Branches: &Branches{
						As:    types.StringValue("Text"),
						Array: types.BoolValue(true),
						If: &Branch{
							Conditions: []Condition{{
								Subject:   types.StringValue("payload.env"),
								Operation: types.StringValue("is_set"),
							}},
							Result: &Binding{ArrayValue: []BindingValue{
								{Literal: types.StringValue("core")},
								{Reference: types.StringValue("payload.team")},
							}},
						},
					}},
				},
			},
		},
		{
			name: "a flat fallback",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback:   &Fallback{Result: &Binding{ValueLiteral: types.StringValue("unknown")}},
			},
		},
		{
			name: "a fallback deferring to a sibling",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback:   &Fallback{ExpressionRef: types.StringValue("guess")},
			},
			named: []NamedExpression{{
				Name:       types.StringValue("guess"),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			}},
		},
		{
			// Has a second expression behind it, so most able to read back as
			// something nobody wrote.
			name: "the fallback if/else_if/else shorthand",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Parse: &Parse{Function: types.StringValue("$.env"), As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					If: &Branch{
						Conditions: []Condition{{
							Subject:   types.StringValue("payload.env"),
							Operation: types.StringValue("is_set"),
						}},
						Result: &Binding{ValueLiteral: types.StringValue("staging")},
					},
					ElseIf: []Branch{{
						Conditions: []Condition{{
							Subject:   types.StringValue("payload.region"),
							Operation: types.StringValue("is_set"),
						}},
						Result: &Binding{ValueLiteral: types.StringValue("eu")},
					}},
					Else: &Else{Result: &Binding{ValueLiteral: types.StringValue("production")}},
				},
			},
		},
		{
			name: "the shorthand with no trailing else",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Parse: &Parse{Function: types.StringValue("$.env"), As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					If: &Branch{
						Conditions: []Condition{{
							Subject:   types.StringValue("payload.env"),
							Operation: types.StringValue("is_set"),
						}},
						Result: &Binding{ValueLiteral: types.StringValue("staging")},
					},
				},
			},
		},
		{
			name: "a binding pointing at a named expression",
			bound: &Expression{
				StartFrom: types.StringValue("."),
				Operations: []Operation{
					{Branches: &Branches{
						As: types.StringValue("Text"),
						If: &Branch{
							Conditions: []Condition{{
								Subject:   types.StringValue("payload.env"),
								Operation: types.StringValue("is_set"),
								Params:    []Binding{{ExpressionRef: types.StringValue("guess")}},
							}},
							Result: &Binding{ExpressionRef: types.StringValue("guess")},
						},
					}},
				},
			},
			named: []NamedExpression{{
				Name:       types.StringValue("guess"),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			}},
		},
		{
			name: "named expressions keep the order they were written in",
			named: []NamedExpression{
				{
					Name:       types.StringValue("zebra"),
					StartFrom:  types.StringValue("payload"),
					Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				},
				{
					Name:       types.StringValue("aardvark"),
					StartFrom:  types.StringValue("payload"),
					Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				},
			},
		},
		{
			// How the alert source itself uses this layer.
			name: "named expressions with no bound expression",
			named: []NamedExpression{{
				Name:       types.StringValue("guess"),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payloads, boundBinding, err := ExpressionsToPayload(testNamespace, tc.bound, tc.named)
			if err != nil {
				t.Fatalf("ExpressionsToPayload: %v", err)
			}

			// Declaring the block is what binds it; the config wrote no binding.
			if tc.bound == nil && boundBinding != nil {
				t.Error("got a bound binding for a resource with no expression block")
			}
			if tc.bound != nil {
				if boundBinding == nil || boundBinding.Reference == nil {
					t.Fatal("expected an expression block to bind its result")
				}
				name := ExpressionNameFromReference(*boundBinding.Reference)
				if !slices.ContainsFunc(payloads, func(payload client.ExpressionPayloadV3) bool {
					return payload.Reference == name
				}) {
					t.Errorf("bound to %q, which none of the expressions written is called", *boundBinding.Reference)
				}
			}

			// A prior carrying only the names fixes the order the way a real one does, while
			// still failing the spelling reconcile — a name-only model can't round trip to the
			// payload. So this exercises the mapping, and every case is written in the spelling
			// a read returns, which is what makes the round trip meaningful.
			priorOrder := make([]NamedExpression, 0, len(tc.named))
			for _, expression := range tc.named {
				priorOrder = append(priorOrder, NamedExpression{Name: expression.Name})
			}

			bound, named := ExpressionsFromPayload(payloads, testNamespace, nil, priorOrder)

			if !reflect.DeepEqual(tc.bound, bound) {
				t.Errorf("expression did not round trip\n got: %#v\nwant: %#v", bound, tc.bound)
			}
			if !reflect.DeepEqual(tc.named, named) {
				t.Errorf("named expressions did not round trip\n got: %#v\nwant: %#v", named, tc.named)
			}
		})
	}
}

// TestImportOrdersExpressionsStably reads a resource with no config order to preserve. The
// order must be deterministic, or the first plan after an import churns, and a synthesized
// fallback must still fold back into its parent.
func TestImportOrdersExpressionsStably(t *testing.T) {
	// zebra sorts before its own synthesized expression, which is what lets the parent fold
	// it away first.
	payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, []NamedExpression{
		{
			Name:       types.StringValue("zebra"),
			StartFrom:  types.StringValue("payload"),
			Operations: []Operation{{Parse: &Parse{Function: types.StringValue("$.env"), As: types.StringValue("Text")}}},
			Fallback: &Fallback{
				If: &Branch{
					Conditions: []Condition{{Subject: types.StringValue("payload.env"), Operation: types.StringValue("is_set")}},
					Result:     &Binding{ValueLiteral: types.StringValue("staging")},
				},
			},
		},
		{
			Name:       types.StringValue("aardvark"),
			StartFrom:  types.StringValue("payload"),
			Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
		},
	})
	if err != nil {
		t.Fatalf("ExpressionsToPayload: %v", err)
	}

	// No prior at all is what an import looks like.
	bound, named := ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, nil)

	if bound != nil {
		t.Error("got a bound expression from a payload set that has none")
	}

	gotNames := make([]string, 0, len(named))
	for _, expression := range named {
		gotNames = append(gotNames, expression.Name.ValueString())
	}
	if want := []string{"aardvark", "zebra"}; !reflect.DeepEqual(gotNames, want) {
		t.Errorf("imported %v, want %v sorted for stability", gotNames, want)
	}

	// Folded into zebra's fallback, not returned alongside it.
	zebra := named[1]
	if zebra.Fallback == nil || zebra.Fallback.If == nil {
		t.Fatalf("expected zebra's fallback to fold back into the shorthand, got %#v", zebra.Fallback)
	}
	if !zebra.Fallback.ExpressionRef.IsNull() {
		t.Error("a folded fallback should not also carry the expression_ref it was built from")
	}
}

// TestPriorSpellingSurvivesRead covers the spellings a read can't infer from the payload. Each
// case is written in a form that collapses onto the same payload as its sugar, so a read given
// only the payload returns the sugar — which fails the apply as an inconsistent result, after
// the write has landed. Given the prior, the config's own spelling has to come back untouched.
func TestPriorSpellingSurvivesRead(t *testing.T) {
	condition := Condition{
		Subject:   types.StringValue("payload.level"),
		Operation: types.StringValue("is_set"),
	}

	for _, tc := range []struct {
		name  string
		named []NamedExpression
	}{
		{
			// Sugar: value_literal.
			name: "long form value on a branch result",
			named: []NamedExpression{{
				Name:      types.StringValue("severity"),
				StartFrom: types.StringValue("payload"),
				Operations: []Operation{{
					Branches: &Branches{
						As: types.StringValue("Text"),
						If: &Branch{
							Conditions: []Condition{condition},
							Result:     &Binding{Value: &BindingValue{Literal: types.StringValue("high")}},
						},
					},
				}},
			}},
		},
		{
			// Sugar: values.
			name: "all-literal array_value on a condition param",
			named: []NamedExpression{{
				Name:      types.StringValue("owning_team"),
				StartFrom: types.StringValue("payload"),
				Operations: []Operation{{
					Filter: &Conditions{
						Conditions: []Condition{{
							Subject:   types.StringValue("payload.team"),
							Operation: types.StringValue("one_of"),
							Params: []Binding{{
								ArrayValue: []BindingValue{
									{Literal: types.StringValue("platform")},
									{Literal: types.StringValue("payments")},
								},
							}},
						}},
					},
				}},
			}},
		},
		{
			// Sugar: conditions. A single group is indistinguishable from the flat form.
			name: "single condition_groups on a filter",
			named: []NamedExpression{{
				Name:      types.StringValue("errors"),
				StartFrom: types.StringValue("payload"),
				Operations: []Operation{{
					Filter: &Conditions{
						ConditionGroups: []ConditionGroup{{Conditions: []Condition{condition}}},
					},
				}},
			}},
		},
		{
			// The fallback shorthand synthesizes a second expression, so the reconcile has to
			// match that one too before it can keep the prior.
			name: "long form value inside a fallback shorthand",
			named: []NamedExpression{{
				Name:       types.StringValue("service"),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Parse: &Parse{Function: types.StringValue("$.svc"), As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					If: &Branch{
						Conditions: []Condition{condition},
						Result:     &Binding{Value: &BindingValue{Literal: types.StringValue("unknown")}},
					},
				},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, tc.named)
			if err != nil {
				t.Fatalf("ExpressionsToPayload: %v", err)
			}

			// Names only, so the reconcile can't match and the read has to map fresh. This is
			// the behaviour that breaks an apply, pinned here so the reconcile below is known to
			// be doing the work.
			priorOrder := []NamedExpression{{Name: tc.named[0].Name}}
			_, rewritten := ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, priorOrder)
			if reflect.DeepEqual(tc.named, rewritten) {
				t.Fatal("expected the read to rewrite this spelling without a prior; the case no longer covers anything")
			}

			_, kept := ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, tc.named)
			if !reflect.DeepEqual(tc.named, kept) {
				t.Errorf("prior spelling not kept\n got: %#v\nwant: %#v", kept, tc.named)
			}
		})
	}
}

// TestPriorSpellingYieldsToRealChanges is the other half: keeping the prior must not swallow a
// change made outside Terraform, or a drifted resource reads back as unchanged and the next
// apply silently reverts it.
func TestPriorSpellingYieldsToRealChanges(t *testing.T) {
	prior := []NamedExpression{{
		Name:      types.StringValue("severity"),
		StartFrom: types.StringValue("payload"),
		Operations: []Operation{{
			Branches: &Branches{
				As: types.StringValue("Text"),
				If: &Branch{
					Conditions: []Condition{{
						Subject:   types.StringValue("payload.level"),
						Operation: types.StringValue("is_set"),
					}},
					Result: &Binding{Value: &BindingValue{Literal: types.StringValue("high")}},
				},
			},
		}},
	}}

	// What the server holds after someone edited the result in the dashboard.
	changed := []NamedExpression{{
		Name:      prior[0].Name,
		StartFrom: prior[0].StartFrom,
		Operations: []Operation{{
			Branches: &Branches{
				As: types.StringValue("Text"),
				If: &Branch{
					Conditions:      prior[0].Operations[0].Branches.If.Conditions,
					ConditionGroups: nil,
					Result:          &Binding{ValueLiteral: types.StringValue("critical")},
				},
			},
		}},
	}}

	payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, changed)
	if err != nil {
		t.Fatalf("ExpressionsToPayload: %v", err)
	}

	_, got := ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, prior)
	if !reflect.DeepEqual(changed, got) {
		t.Errorf("read did not surface the server's value\n got: %#v\nwant: %#v", got, changed)
	}
}

func TestReconcileBinding(t *testing.T) {
	longForm := &Binding{Value: &BindingValue{Literal: types.StringValue("high")}}
	sugar := &Binding{ValueLiteral: types.StringValue("high")}

	payload, err := BindingToPayload(longForm)
	if err != nil {
		t.Fatalf("BindingToPayload: %v", err)
	}

	for _, tc := range []struct {
		name    string
		prior   *Binding
		fromAPI *client.EngineParamBindingPayloadV3
		want    *Binding
	}{
		{"keeps the long form the config wrote", longForm, payload, longForm},
		{"keeps the sugar the config wrote", sugar, payload, sugar},
		{"maps fresh with no prior", nil, payload, sugar},
		{
			// The prior no longer means this, so the server's value has to win.
			name:    "yields to a changed value",
			prior:   &Binding{ValueLiteral: types.StringValue("low")},
			fromAPI: payload,
			want:    sugar,
		},
		{
			// Absent has one spelling, so the prior is irrelevant.
			name:    "reads absent as absent",
			prior:   longForm,
			fromAPI: nil,
			want:    nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReconcileBinding(tc.prior, tc.fromAPI); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestKnownExpressionNames(t *testing.T) {
	known := KnownExpressionNames([]NamedExpression{
		{Name: types.StringValue("guess")},
		{Name: types.StringValue("severity")},
	})

	for _, name := range []string{"guess", "severity"} {
		if !known[name] {
			t.Errorf("expected %q to be known", name)
		}
	}
	if known["nope"] {
		t.Error("expected an unwritten name not to be known")
	}
}

// TestFallbackShorthandPayload pins what the round trip can't see: emitting a catch-all
// branch instead of an else branch would still fold back to the same blocks, and they are
// different rows to the server.
func TestFallbackShorthandPayload(t *testing.T) {
	payloads, _, err := ExpressionsToPayload(testNamespace, &Expression{
		StartFrom:  types.StringValue("payload"),
		Operations: []Operation{{Parse: &Parse{Function: types.StringValue("$.x"), As: types.StringValue("Text"), Array: types.BoolValue(true)}}},
		Fallback: &Fallback{
			If: &Branch{
				Conditions: []Condition{{Subject: types.StringValue("payload.env"), Operation: types.StringValue("is_set")}},
				Result:     &Binding{ValueLiteral: types.StringValue("staging")},
			},
			Else: &Else{Result: &Binding{ValueLiteral: types.StringValue("production")}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("ExpressionsToPayload: %v", err)
	}

	if len(payloads) != 2 {
		t.Fatalf("got %d expressions, want the parent plus its synthesized fallback", len(payloads))
	}
	parent, synthesized := payloads[0], payloads[1]

	if want := testNamespace.stored(boundExpressionLocalName); parent.Reference != want {
		t.Errorf("parent reference is %q, want %q", parent.Reference, want)
	}
	if want := parent.Reference + FallbackSuffix; synthesized.Reference != want {
		t.Errorf("synthesized reference is %q, want %q", synthesized.Reference, want)
	}

	// The parent defers rather than carrying the branches itself.
	if parent.ElseBranch == nil || parent.ElseBranch.Result.Value == nil || parent.ElseBranch.Result.Value.Reference == nil {
		t.Fatal("expected the parent to point its else branch at the synthesized expression")
	}
	if got, want := *parent.ElseBranch.Result.Value.Reference, ExpressionReference(synthesized.Reference); got != want {
		t.Errorf("parent else branch points at %q, want %q", got, want)
	}

	// A branches-only expression starts from nothing.
	if synthesized.RootReference != "." {
		t.Errorf("synthesized root reference is %q, want %q", synthesized.RootReference, ".")
	}
	if len(synthesized.Operations) != 1 || synthesized.Operations[0].Branches == nil {
		t.Fatal("expected the synthesized expression to hold exactly one branches operation")
	}
	branches := synthesized.Operations[0].Branches

	// Read off the parent: the shorthand has nowhere to write one.
	if branches.Returns.Type != "Text" || !branches.Returns.Array {
		t.Errorf("synthesized branches return %+v, want the parent's Text array", branches.Returns)
	}

	// The trailing else is an else branch, never a branch with no conditions.
	if len(branches.Branches) != 1 {
		t.Fatalf("got %d branches, want only the one the config wrote", len(branches.Branches))
	}
	if len(branches.Branches[0].ConditionGroups) == 0 {
		t.Error("expected the written branch to carry its conditions")
	}
	if synthesized.ElseBranch == nil || synthesized.ElseBranch.Result.Value == nil {
		t.Fatal("expected the trailing else to become the synthesized expression's else branch")
	}
	if got := lo.FromPtr(synthesized.ElseBranch.Result.Value.Literal); got != "production" {
		t.Errorf("else branch holds %q, want %q", got, "production")
	}
}

// TestFallbackShorthandNeedsAKnowableType is the one case the shorthand cannot serve: after
// navigate the type comes from the catalog, and only the server can resolve that.
func TestFallbackShorthandNeedsAKnowableType(t *testing.T) {
	_, _, err := ExpressionsToPayload(testNamespace, &Expression{
		StartFrom:  types.StringValue("payload.service"),
		Operations: []Operation{{Navigate: &Navigate{To: types.StringValue("owner")}}},
		Fallback: &Fallback{
			If: &Branch{
				Conditions: []Condition{{Subject: types.StringValue("payload.env"), Operation: types.StringValue("is_set")}},
				Result:     &Binding{ValueLiteral: types.StringValue("staging")},
			},
		},
	}, nil)

	if err == nil {
		t.Fatal("expected an error when the return type cannot be known locally")
	}
}

// TestInferResultType pins when the provider can name what a pipeline returns. It needs both
// halves — the type and the array flag — because the branches it synthesizes declare both,
// and an operation that clears one does not necessarily clear the other.
func TestInferResultType(t *testing.T) {
	for _, tc := range []struct {
		name       string
		operations []Operation
		wantType   string
		wantArray  bool
		wantOK     bool
	}{
		{
			name:       "parse asserts both",
			operations: []Operation{{Parse: &Parse{As: types.StringValue("Text"), Array: types.BoolValue(true)}}},
			wantType:   "Text",
			wantArray:  true,
			wantOK:     true,
		},
		{
			name:       "navigate alone knows neither",
			operations: []Operation{{Navigate: &Navigate{To: types.StringValue("owner")}}},
			wantOK:     false,
		},
		{
			// The root's own cardinality is not ours to know: start_from = "payload.teams"
			// is an array and start_from = "payload" is not, and both reach here identically.
			// cast names the type and leaves cardinality following its input, so it cannot
			// settle this on its own.
			name:       "cast alone does not pin the root's cardinality",
			operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			wantOK:     false,
		},
		{
			name:       "filter alone pins neither",
			operations: []Operation{{Filter: &Conditions{}}},
			wantOK:     false,
		},
		{
			// first pins cardinality, leaving cast only the type to supply.
			name: "cast and first together pin both from the root",
			operations: []Operation{
				{Cast: &Cast{As: types.StringValue("Text")}},
				{First: &EmptyOpts{}},
			},
			wantType: "Text",
			wantOK:   true,
		},
		{
			// parse declares cardinality even when array is omitted, which means scalar —
			// so the canonical attribute pipeline stays knowable straight off the root.
			name:       "parse pins both straight off the root",
			operations: []Operation{{Parse: &Parse{As: types.StringValue("Text")}}},
			wantType:   "Text",
			wantOK:     true,
		},
		{
			// An unknown type mid-pipeline is not the final answer: parse asserts both
			// halves again, so the result is knowable after all.
			name: "parse recovers from navigate",
			operations: []Operation{
				{Navigate: &Navigate{To: types.StringValue("owner")}},
				{Parse: &Parse{As: types.StringValue("Text")}},
			},
			wantType: "Text",
			wantOK:   true,
		},
		{
			// cast asserts the type but leaves cardinality following its input, which
			// navigate already made unknown.
			name: "cast alone does not recover from navigate",
			operations: []Operation{
				{Navigate: &Navigate{To: types.StringValue("owner")}},
				{Cast: &Cast{As: types.StringValue("Text")}},
			},
			wantOK: false,
		},
		{
			// first pins cardinality, so cast has only the type left to supply.
			name: "first and cast together recover from navigate",
			operations: []Operation{
				{Navigate: &Navigate{To: types.StringValue("owner")}},
				{First: &EmptyOpts{}},
				{Cast: &Cast{As: types.StringValue("Text")}},
			},
			wantType: "Text",
			wantOK:   true,
		},
		{
			// count returns one number, so the cardinality is known and only the engine's
			// name for the type is not.
			name:       "count knows the cardinality but not the type",
			operations: []Operation{{Count: &EmptyOpts{}}},
			wantOK:     false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotArray, ok := inferResultType(tc.operations)

			if ok != tc.wantOK {
				t.Fatalf("ok is %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if gotType != tc.wantType || gotArray != tc.wantArray {
				t.Errorf("got (%q, %v), want (%q, %v)", gotType, gotArray, tc.wantType, tc.wantArray)
			}
		})
	}
}

// TestBindingToPayload covers the sugar collapsing onto the two forms the API has.
func TestBindingToPayload(t *testing.T) {
	for _, tc := range []struct {
		name    string
		binding *Binding
		want    *client.EngineParamBindingPayloadV3
		wantErr bool
	}{
		{
			name:    "nothing set reads as absent",
			binding: &Binding{},
			want:    nil,
		},
		{
			name:    "value_literal",
			binding: &Binding{ValueLiteral: types.StringValue("major")},
			want: &client.EngineParamBindingPayloadV3{
				Value: &client.EngineParamBindingValuePayloadV3{Literal: lo.ToPtr("major")},
			},
		},
		{
			name:    "value_reference",
			binding: &Binding{ValueReference: types.StringValue("payload.team")},
			want: &client.EngineParamBindingPayloadV3{
				Value: &client.EngineParamBindingValuePayloadV3{Reference: lo.ToPtr("payload.team")},
			},
		},
		{
			name:    "expression_ref becomes an ordinary scope reference",
			binding: &Binding{ExpressionRef: types.StringValue("guess")},
			want: &client.EngineParamBindingPayloadV3{
				Value: &client.EngineParamBindingValuePayloadV3{Reference: lo.ToPtr(`expressions["guess"]`)},
			},
		},
		{
			name:    "values is an all-literal array",
			binding: &Binding{Values: []types.String{types.StringValue("a"), types.StringValue("b")}},
			want: &client.EngineParamBindingPayloadV3{
				ArrayValue: &[]client.EngineParamBindingValuePayloadV3{
					{Literal: lo.ToPtr("a")},
					{Literal: lo.ToPtr("b")},
				},
			},
		},
		{
			name:    "the long form of one value",
			binding: &Binding{Value: &BindingValue{Reference: types.StringValue("payload.team")}},
			want: &client.EngineParamBindingPayloadV3{
				Value: &client.EngineParamBindingValuePayloadV3{Reference: lo.ToPtr("payload.team")},
			},
		},
		{
			name: "a mixed array needs the long form",
			binding: &Binding{ArrayValue: []BindingValue{
				{Literal: types.StringValue("core")},
				{Reference: types.StringValue("payload.team")},
			}},
			want: &client.EngineParamBindingPayloadV3{
				ArrayValue: &[]client.EngineParamBindingValuePayloadV3{
					{Literal: lo.ToPtr("core")},
					{Reference: lo.ToPtr("payload.team")},
				},
			},
		},
		{
			name: "two forms at once is refused",
			binding: &Binding{
				ValueLiteral:   types.StringValue("major"),
				ValueReference: types.StringValue("payload.team"),
			},
			wantErr: true,
		},
		{
			name:    "a value that is neither literal nor reference is refused",
			binding: &Binding{Value: &BindingValue{}},
			wantErr: true,
		},
		{
			name:    "a value that is both is refused",
			binding: &Binding{Value: &BindingValue{Literal: types.StringValue("a"), Reference: types.StringValue("b")}},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BindingToPayload(tc.binding)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("BindingToPayload: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestOperationPayloadTypes checks each option lands on the type the server keys on. A
// mis-mapping is silent — the payload stays well formed and does the wrong thing.
func TestOperationPayloadTypes(t *testing.T) {
	for want, operation := range map[string]Operation{
		"parse":       {Parse: &Parse{Function: types.StringValue("$.x"), As: types.StringValue("Text")}},
		"navigate":    {Navigate: &Navigate{To: types.StringValue("owner")}},
		"cast":        {Cast: &Cast{As: types.StringValue("Text")}},
		"concatenate": {Concatenate: &Concatenate{With: types.StringValue("payload.x")}},
		"filter":      {Filter: &Conditions{}},
		"first":       {First: &EmptyOpts{}},
		"count":       {Count: &EmptyOpts{}},
		"sum":         {Sum: &EmptyOpts{}},
		"min":         {Min: &EmptyOpts{}},
		"max":         {Max: &EmptyOpts{}},
		"random":      {Random: &EmptyOpts{}},
		"branches": {Branches: &Branches{
			As: types.StringValue("Text"),
			If: &Branch{Result: &Binding{ValueLiteral: types.StringValue("a")}},
		}},
	} {
		t.Run(want, func(t *testing.T) {
			payload, err := operationPayload(operation)
			if err != nil {
				t.Fatalf("operationPayload: %v", err)
			}
			if got := string(payload.OperationType); got != want {
				t.Errorf("operation type is %q, want %q", got, want)
			}
		})
	}

	t.Run("an empty operation is refused", func(t *testing.T) {
		if _, err := operationPayload(Operation{}); err == nil {
			t.Fatal("expected an error for an operation with no option set")
		}
	})
}

// TestValidateExpressions covers the checks standing in for schema Required and
// ConflictsWith. They hold only as long as this test does.
func TestValidateExpressions(t *testing.T) {
	validExpression := func() *Expression {
		return &Expression{
			StartFrom:  types.StringValue("payload"),
			Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
		}
	}

	for _, tc := range []struct {
		name    string
		bound   *Expression
		named   []NamedExpression
		wantErr bool
	}{
		{
			name:  "a valid expression",
			bound: validExpression(),
		},
		{
			name:    "an expression with no start_from",
			bound:   &Expression{Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}}},
			wantErr: true,
		},
		{
			name:    "an expression with no operations",
			bound:   &Expression{StartFrom: types.StringValue("payload")},
			wantErr: true,
		},
		{
			name: "an operation setting two options",
			bound: &Expression{
				StartFrom: types.StringValue("payload"),
				Operations: []Operation{{
					Cast:  &Cast{As: types.StringValue("Text")},
					First: &EmptyOpts{},
				}},
			},
			wantErr: true,
		},
		{
			name: "branches with no as",
			bound: &Expression{
				StartFrom: types.StringValue("."),
				Operations: []Operation{{Branches: &Branches{
					If: &Branch{Result: &Binding{ValueLiteral: types.StringValue("a")}},
				}}},
			},
			wantErr: true,
		},
		{
			name: "branches with no branches",
			bound: &Expression{
				StartFrom:  types.StringValue("."),
				Operations: []Operation{{Branches: &Branches{As: types.StringValue("Text")}}},
			},
			wantErr: true,
		},
		{
			name: "an else_if with no if",
			bound: &Expression{
				StartFrom: types.StringValue("."),
				Operations: []Operation{{Branches: &Branches{
					As:     types.StringValue("Text"),
					ElseIf: []Branch{{Result: &Binding{ValueLiteral: types.StringValue("a")}}},
				}}},
			},
			wantErr: true,
		},
		{
			name: "a branch with no result",
			bound: &Expression{
				StartFrom: types.StringValue("."),
				Operations: []Operation{{Branches: &Branches{
					As: types.StringValue("Text"),
					If: &Branch{},
				}}},
			},
			wantErr: true,
		},
		{
			name: "both conditions and condition_groups",
			bound: &Expression{
				StartFrom: types.StringValue("payload"),
				Operations: []Operation{{Filter: &Conditions{
					Conditions:      []Condition{{Subject: types.StringValue("input.x"), Operation: types.StringValue("is_set")}},
					ConditionGroups: []ConditionGroup{{Conditions: []Condition{{Subject: types.StringValue("input.y"), Operation: types.StringValue("is_set")}}}},
				}}},
			},
			wantErr: true,
		},
		{
			name: "a fallback setting both result and expression_ref",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					Result:        &Binding{ValueLiteral: types.StringValue("a")},
					ExpressionRef: types.StringValue("guess"),
				},
			},
			named: []NamedExpression{{
				Name:       types.StringValue("guess"),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			}},
			wantErr: true,
		},
		{
			name: "a fallback mixing result with the shorthand",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					Result: &Binding{ValueLiteral: types.StringValue("a")},
					If:     &Branch{Result: &Binding{ValueLiteral: types.StringValue("b")}},
				},
			},
			wantErr: true,
		},
		{
			name: "an else with no result",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					If:   &Branch{Result: &Binding{ValueLiteral: types.StringValue("b")}},
					Else: &Else{},
				},
			},
			wantErr: true,
		},
		{
			name: "an expression_ref naming nothing",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback:   &Fallback{ExpressionRef: types.StringValue("nope")},
			},
			wantErr: true,
		},
		{
			name: "an expression_ref inside a binding naming nothing",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback:   &Fallback{Result: &Binding{ExpressionRef: types.StringValue("nope")}},
			},
			wantErr: true,
		},
		{
			name:  "an expression_ref may point forwards at a sibling written below it",
			bound: validExpression(),
			named: []NamedExpression{
				{
					Name:       types.StringValue("first"),
					StartFrom:  types.StringValue("payload"),
					Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
					Fallback:   &Fallback{ExpressionRef: types.StringValue("second")},
				},
				{
					Name:       types.StringValue("second"),
					StartFrom:  types.StringValue("payload"),
					Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				},
			},
		},
		{
			name:  "a named expression taking the reserved bound name",
			bound: validExpression(),
			named: []NamedExpression{{
				Name:       types.StringValue("_bound"),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			}},
			wantErr: true,
		},
		{
			name:  "a named expression taking the reserved fallback suffix",
			bound: validExpression(),
			named: []NamedExpression{{
				Name:       types.StringValue("guess" + FallbackSuffix),
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
			}},
			wantErr: true,
		},
		{
			name:  "two named expressions with the same name",
			bound: validExpression(),
			named: []NamedExpression{
				{
					Name:       types.StringValue("guess"),
					StartFrom:  types.StringValue("payload"),
					Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				},
				{
					Name:       types.StringValue("guess"),
					StartFrom:  types.StringValue("payload"),
					Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				},
			},
			wantErr: true,
		},
		{
			// The shorthand's branches take their type from the parent, so a parent whose
			// type is not knowable locally has none to give them.
			name: "a branching fallback whose parent type cannot be inferred",
			bound: &Expression{
				StartFrom:  types.StringValue("payload.service"),
				Operations: []Operation{{Navigate: &Navigate{To: types.StringValue("owner")}}},
				Fallback: &Fallback{
					If: &Branch{
						Conditions: []Condition{{Subject: types.StringValue("payload.env"), Operation: types.StringValue("is_set")}},
						Result:     &Binding{ValueLiteral: types.StringValue("staging")},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "an else with no if to default",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					Else: &Else{Result: &Binding{ValueLiteral: types.StringValue("a")}},
				},
			},
			wantErr: true,
		},
		{
			name: "a value that is neither literal nor reference",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback:   &Fallback{Result: &Binding{Value: &BindingValue{}}},
			},
			wantErr: true,
		},
		{
			name: "an array_value element that is both literal and reference",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback: &Fallback{Result: &Binding{ArrayValue: []BindingValue{
					{Literal: types.StringValue("core"), Reference: types.StringValue("payload.team")},
				}}},
			},
			wantErr: true,
		},
		{
			name: "a binding setting two forms at once",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback: &Fallback{Result: &Binding{
					ValueLiteral: types.StringValue("a"),
					Values:       []types.String{types.StringValue("b")},
				}},
			},
			wantErr: true,
		},
		{
			// `result = {}` is a present binding holding nothing. The missing-result checks
			// see a non-nil binding and pass, and the mapping then treats zero forms as
			// absent — so without this the branch fails at apply.
			name: "a branch result with no value set",
			bound: &Expression{
				StartFrom: types.StringValue("."),
				Operations: []Operation{{Branches: &Branches{
					As: types.StringValue("Text"),
					If: &Branch{Result: &Binding{}},
				}}},
			},
			wantErr: true,
		},
		{
			// Worse than an apply error: the mapping drops an empty else branch, so the
			// next plan tries to add it again, forever.
			name: "a fallback result with no value set",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback:   &Fallback{Result: &Binding{}},
			},
			wantErr: true,
		},
		{
			name: "a fallback else result with no value set",
			bound: &Expression{
				StartFrom:  types.StringValue("payload"),
				Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
				Fallback: &Fallback{
					If:   &Branch{Result: &Binding{ValueLiteral: types.StringValue("a")}},
					Else: &Else{Result: &Binding{}},
				},
			},
			wantErr: true,
		},
		{
			name: "a condition parameter with no value",
			bound: &Expression{
				StartFrom: types.StringValue("payload"),
				Operations: []Operation{{Filter: &Conditions{
					Conditions: []Condition{{
						Subject:   types.StringValue("input.x"),
						Operation: types.StringValue("one_of"),
						Params:    []Binding{{}},
					}},
				}}},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			ValidateExpressions(
				testNamespace,
				tc.bound, path.Root("expression"),
				tc.named, path.Root("named_expression"),
				&diags)

			if tc.wantErr && !diags.HasError() {
				t.Fatal("expected an error")
			}
			if !tc.wantErr && diags.HasError() {
				t.Fatalf("expected no error, got: %+v", diags)
			}

			// Whatever validation accepts, the mapping must map, or the config plans clean
			// and fails at apply.
			if !tc.wantErr {
				if _, _, err := ExpressionsToPayload(testNamespace, tc.bound, tc.named); err != nil {
					t.Fatalf("a config that validated failed to map: %v", err)
				}
			}
		})
	}
}

// TestValidatedConfigsAlwaysMapCleanly: whatever validation accepts, the mapping must map.
// Otherwise a config passes plan and fails at apply.
func TestValidatedConfigsAlwaysMapCleanly(t *testing.T) {
	bound := &Expression{
		StartFrom: types.StringValue("."),
		Operations: []Operation{{Branches: &Branches{
			As: types.StringValue("Text"),
			If: &Branch{
				Conditions: []Condition{{
					Subject:   types.StringValue("payload.env"),
					Operation: types.StringValue("one_of"),
					Params:    []Binding{{Values: []types.String{types.StringValue("staging")}}},
				}},
				Result: &Binding{ExpressionRef: types.StringValue("guess")},
			},
		}}},
		Fallback: &Fallback{
			If: &Branch{
				Conditions: []Condition{{Subject: types.StringValue("payload.region"), Operation: types.StringValue("is_set")}},
				Result:     &Binding{ValueLiteral: types.StringValue("eu")},
			},
			Else: &Else{Result: &Binding{ValueLiteral: types.StringValue("unknown")}},
		},
	}
	named := []NamedExpression{{
		Name:       types.StringValue("guess"),
		StartFrom:  types.StringValue("payload"),
		Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
	}}

	var diags diag.Diagnostics
	ValidateExpressions(testNamespace, bound, path.Root("expression"), named, path.Root("named_expression"), &diags)
	if diags.HasError() {
		t.Fatalf("expected the config to validate, got: %+v", diags)
	}

	if _, _, err := ExpressionsToPayload(testNamespace, bound, named); err != nil {
		t.Fatalf("a config that validated failed to map: %v", err)
	}
}

// TestSchemaIsValid asks whether the intended HCL shape is expressible at all, rather than
// anything about behaviour — chiefly whether `if` works as a block type name, and whether
// the block/attribute split survives validation. This is what the framework runs against a
// real provider at startup, so a schema passing here is one Terraform will load.
func TestSchemaIsValid(t *testing.T) {
	ctx := context.Background()

	resourceSchema := schema.Schema{
		Attributes: map[string]schema.Attribute{
			"priority": BindingAttribute("A binding used outside any expression."),
		},
		Blocks: map[string]schema.Block{
			"expression":       ExpressionBlock(),
			"named_expression": NamedExpressionBlock(),
		},
	}

	if diags := resourceSchema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("the schema is not a valid implementation: %s", diags)
	}

	// SingleNestedBlock specifically: nullable, so it can join a resource's exclusive value
	// group, where an unset ListNestedBlock would be empty rather than null.
	expression, ok := resourceSchema.Blocks["expression"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected expression to be a SingleNestedBlock")
	}

	operation, ok := expression.Blocks["operation"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("expected operation to be an ordered ListNestedBlock")
	}

	// The one option that must be a block, to carry if / else_if. The rest are attributes,
	// so they can come from a local or a file().
	branches, ok := operation.NestedObject.Blocks["branches"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected branches to be a SingleNestedBlock")
	}
	for _, name := range []string{"if", "else_if"} {
		if _, ok := branches.Blocks[name]; !ok {
			t.Errorf("expected branches to carry an %q block", name)
		}
	}

	// No else at operation level; the default is the expression's fallback.
	if _, ok := branches.Blocks["else"]; ok {
		t.Error("branches should not carry an else — the default belongs in the expression's fallback")
	}

	// fallback does have one: its expression is implicit, so the else branch has nowhere
	// else to live.
	fallback, ok := expression.Blocks["fallback"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected fallback to be a SingleNestedBlock")
	}
	if _, ok := fallback.Blocks["else"]; !ok {
		t.Error("expected fallback to carry an else")
	}

	// Nothing inside the block may be Required, or the block itself can never be omitted.
	for name, attribute := range expression.Attributes {
		if attribute.IsRequired() {
			t.Errorf("%q is Required, which makes the whole expression block mandatory", name)
		}
	}

	// Same body plus a name, so import maps one row to one block.
	named, ok := resourceSchema.Blocks["named_expression"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("expected named_expression to be a ListNestedBlock")
	}
	if _, ok := named.NestedObject.Attributes["name"]; !ok {
		t.Error("expected named_expression to carry a name")
	}
	for _, name := range []string{"operation", "fallback"} {
		if _, ok := named.NestedObject.Blocks[name]; !ok {
			t.Errorf("expected named_expression to have the same %q block as expression", name)
		}
	}
	for name := range expression.Attributes {
		if _, ok := named.NestedObject.Attributes[name]; !ok {
			t.Errorf("named_expression is missing %q, so the two blocks have drifted", name)
		}
	}
}

// TestExpressionNameFromReference guards the fold to expression_ref: a false positive would
// rewrite an ordinary scope path into a reference to an expression that doesn't exist.
func TestExpressionNameFromReference(t *testing.T) {
	for reference, want := range map[string]string{
		`expressions["guess"]`:           "guess",
		`expressions["with--fallback"]`:  "with--fallback",
		"payload.team":                   "",
		"expressions":                    "",
		`incident.custom_field["01ABC"]`: "",
		`other_expressions["guess"]`:     "",

		// A path into an expression's result is a real reference the engine honours, and
		// folding it to expression_ref would silently drop the path.
		`expressions["guess"].name`:        "",
		`expressions["guess"].owner.email`: "",
	} {
		t.Run(reference, func(t *testing.T) {
			if got := ExpressionNameFromReference(reference); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestExpressionLabel covers the label a dashboard-built expression carries. Name is the
// reference everything points at, so an imported expression keeps its opaque reference as the
// name — and without a label of its own, the next write would overwrite "Team" with "823b1cac".
func TestExpressionLabel(t *testing.T) {
	named := func(label types.String) []NamedExpression {
		return []NamedExpression{{
			Name:      types.StringValue("823b1cac"),
			Label:     label,
			StartFrom: types.StringValue("payload"),
			Operations: []Operation{
				{Parse: &Parse{
					Function: types.StringValue("$.team"),
					As:       types.StringValue("String"),
				}},
			},
		}}
	}

	t.Run("a label the config set is what gets written", func(t *testing.T) {
		payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, named(types.StringValue("Team")))
		if err != nil {
			t.Fatalf("mapping: %v", err)
		}
		if payloads[0].Label != "Team" {
			t.Errorf("label should be the configured one, got %q", payloads[0].Label)
		}
		if payloads[0].Reference != "823b1cac" {
			t.Errorf("reference should be the name, got %q", payloads[0].Reference)
		}
	})

	t.Run("no label mirrors the name", func(t *testing.T) {
		payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, named(types.StringNull()))
		if err != nil {
			t.Fatalf("mapping: %v", err)
		}
		if payloads[0].Label != "823b1cac" {
			t.Errorf("label should fall back to the name, got %q", payloads[0].Label)
		}
	})

	t.Run("a label matching the name reads back as unset", func(t *testing.T) {
		payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, named(types.StringNull()))
		if err != nil {
			t.Fatalf("mapping: %v", err)
		}

		_, read := ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, named(types.StringNull()))
		if !read[0].Label.IsNull() {
			t.Errorf("a label the config never set should stay unset, got %v", read[0].Label)
		}
	})

	t.Run("a label the config set survives the read", func(t *testing.T) {
		prior := named(types.StringValue("Team"))
		payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, prior)
		if err != nil {
			t.Fatalf("mapping: %v", err)
		}

		_, read := ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, prior)
		if read[0].Label.ValueString() != "Team" {
			t.Errorf("label should round-trip, got %v", read[0].Label)
		}
	})

	// An import has no prior state, so the label is only worth keeping when it says something
	// the reference doesn't.
	t.Run("import keeps a label that isn't the reference", func(t *testing.T) {
		payloads, _, err := ExpressionsToPayload(alertSourceTestNamespace, nil, named(types.StringValue("Team")))
		if err != nil {
			t.Fatalf("mapping: %v", err)
		}

		_, read := ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, nil)
		if read[0].Label.ValueString() != "Team" {
			t.Errorf("an imported label should be captured, got %v", read[0].Label)
		}

		payloads, _, err = ExpressionsToPayload(alertSourceTestNamespace, nil, named(types.StringNull()))
		if err != nil {
			t.Fatalf("mapping: %v", err)
		}

		_, read = ExpressionsFromPayload(payloads, alertSourceTestNamespace, nil, nil)
		if !read[0].Label.IsNull() {
			t.Errorf("an imported label equal to the reference says nothing, got %v", read[0].Label)
		}
	})
}
