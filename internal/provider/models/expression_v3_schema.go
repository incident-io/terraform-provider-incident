package models

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The exclusive groups live in ValidateExpressions, not in schema validators: most span a
// block and its sibling attribute, which ConflictsWith cannot describe, and exclusivity
// split across two mechanisms is how the two drift apart.

// ExpressionBlock has to be a SingleNestedBlock so it can join a resource's exclusive
// value group: it is nullable, where an unset ListNestedBlock is empty rather than null and
// so never reads as absent.
func ExpressionBlock() schema.Block {
	return schema.SingleNestedBlock{
		Description: "The expression this field is bound to. Declaring it binds its result.",
		Attributes:  expressionAttributes(false),
		Blocks:      expressionBlocks(),
	}
}

func NamedExpressionBlock() schema.Block {
	return schema.ListNestedBlock{
		Description: "An expression this resource owns, addressed by name.",
		NestedObject: schema.NestedBlockObject{
			Attributes: expressionAttributes(true),
			Blocks:     expressionBlocks(),
		},
	}
}

func expressionAttributes(named bool) map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{
		// Required here would make `expression { }` mandatory on every resource: the
		// framework enforces a Required child even when the block itself is absent, and
		// there is no way to say "required, but only if present". ValidateExpressions
		// enforces it instead.
		"start_from": schema.StringAttribute{
			Optional:    true,
			Description: `Where the expression starts: "payload", "alert", "." for a branches-only expression, or a scope path. Required when the block is present.`,
		},
	}

	if named {
		attributes["name"] = schema.StringAttribute{
			Required:    true,
			Description: "A name for this expression, unique within this resource, referenced by expression_ref.",
		}
		attributes["label"] = schema.StringAttribute{
			Optional: true,
			Description: "What the dashboard shows for this expression. Defaults to the name. Set it " +
				"when importing a source whose expressions were labelled in the dashboard, so those " +
				"labels survive.",
		}
	}

	return attributes
}

func expressionBlocks() map[string]schema.Block {
	return map[string]schema.Block{
		"operation": schema.ListNestedBlock{
			Description: "An ordered pipeline. Each operation feeds the next.",
			NestedObject: schema.NestedBlockObject{
				Attributes: operationOptionAttributes(),
				Blocks: map[string]schema.Block{
					// The one option that must be a block, to carry if / else_if.
					"branches": branchesBlock(),
				},
			},
		},
		"fallback": fallbackBlock(),
	}
}

func operationOptionAttributes() map[string]schema.Attribute {
	empty := func(description string) schema.Attribute {
		return schema.SingleNestedAttribute{
			Optional:    true,
			Description: description,
			Attributes:  map[string]schema.Attribute{},
		}
	}

	return map[string]schema.Attribute{
		"parse": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Evaluates a function against the current value.",
			Attributes: map[string]schema.Attribute{
				"function": schema.StringAttribute{
					Required:    true,
					Description: "JavaScript evaluated against the current value, bound to `$`. 5 KiB limit.",
				},
				"as": schema.StringAttribute{
					Required:    true,
					Description: "The type this returns. Take it from the resource that defines the type rather than writing it out, e.g. `incident_catalog_type.service.attribute_type`.",
				},
				"array": schema.BoolAttribute{
					Optional:    true,
					Description: "Whether this returns several values rather than one.",
				},
			},
		},
		"navigate": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Follows an attribute of the current value.",
			Attributes: map[string]schema.Attribute{
				"to": schema.StringAttribute{
					Required:    true,
					Description: "The catalog attribute to follow.",
				},
			},
		},
		"cast": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Converts the current value to another type.",
			Attributes: map[string]schema.Attribute{
				"as": schema.StringAttribute{
					Required:    true,
					Description: "The type to convert to. Take it from the resource that defines the type rather than writing it out.",
				},
			},
		},
		"concatenate": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Adds the values behind another reference to the current value, keeping each value once. There is no delimiter, despite the name.",
			Attributes: map[string]schema.Attribute{
				"with": schema.StringAttribute{
					Required:    true,
					Description: "The reference whose values are added.",
				},
			},
		},
		"filter": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "Keeps the values matching these conditions. Inside a filter the value under test is bound as `input`.",
			Attributes:  ConditionsAttributes(),
		},

		"first":  empty("Takes the first value."),
		"count":  empty("Counts the values."),
		"sum":    empty("Adds the values together."),
		"min":    empty("Takes the smallest value."),
		"max":    empty("Takes the largest value."),
		"random": empty("Takes one value at random."),
	}
}

func branchesBlock() schema.Block {
	return schema.SingleNestedBlock{
		Description: `A lookup table, evaluated in order until one matches. Must be the only operation in its expression, with start_from = ".".`,
		Attributes: map[string]schema.Attribute{
			// Optional for the same reason as start_from above.
			"as": schema.StringAttribute{
				Optional:    true,
				Description: "The type every branch result returns. Required when the block is present.",
			},
			"array": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether each branch returns several values rather than one.",
			},
		},
		Blocks: map[string]schema.Block{
			"if":      branchBlock("The first branch to try."),
			"else_if": branchListBlock("Tried in order, after if."),
		},
	}
}

func branchBlock(description string) schema.Block {
	return schema.SingleNestedBlock{
		Description: description,
		Attributes:  branchAttributes(),
	}
}

func branchListBlock(description string) schema.Block {
	return schema.ListNestedBlock{
		Description:  description,
		NestedObject: schema.NestedBlockObject{Attributes: branchAttributes()},
	}
}

func branchAttributes() map[string]schema.Attribute {
	attributes := ConditionsAttributes()
	attributes["result"] = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "The value this branch produces. Required when the branch is present.",
		Attributes:  BindingAttributes(),
	}

	return attributes
}

func ConditionsAttributes() map[string]schema.Attribute {
	condition := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"subject": schema.StringAttribute{
				Required:    true,
				Description: "The reference this condition tests.",
			},
			"operation": schema.StringAttribute{
				Required:    true,
				Description: "How the subject is tested. The available operations depend on the subject's type.",
			},
			"params": schema.ListNestedAttribute{
				Optional:     true,
				Description:  "Positional parameters for the operation.",
				NestedObject: schema.NestedAttributeObject{Attributes: BindingAttributes()},
			},
		},
	}

	return map[string]schema.Attribute{
		"conditions": schema.ListNestedAttribute{
			Optional:     true,
			Description:  "All of these must hold. Sugar for a single condition group.",
			NestedObject: condition,
		},
		"condition_groups": schema.ListNestedAttribute{
			Optional:    true,
			Description: "Groups are OR'd; conditions within a group are AND'd.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"conditions": schema.ListNestedAttribute{
						Required:     true,
						Description:  "All of these must hold for the group to hold.",
						NestedObject: condition,
					},
				},
			},
		},
	}
}

// BindingAttribute is a whole field that takes a value, for the resources that bind one
// directly rather than through an expression block — an alert source's priority.
func BindingAttribute(description string) schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: description,
		Attributes:  BindingAttributes(),
	}
}

func BindingAttributes() map[string]schema.Attribute {
	value := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"literal": schema.StringAttribute{
				Optional:    true,
				Description: "A fixed value. A catalog entry ID is a literal, not a reference.",
			},
			"reference": schema.StringAttribute{
				Optional:    true,
				Description: "A reference into the scope, such as `payload.team`.",
			},
		},
	}

	return map[string]schema.Attribute{
		"value_literal": schema.StringAttribute{
			Optional:    true,
			Description: "A fixed value. A catalog entry ID is a literal, not a reference.",
		},
		"value_reference": schema.StringAttribute{
			Optional:    true,
			Description: "A reference into the scope, such as `payload.team`.",
		},
		"expression_ref": schema.StringAttribute{
			Optional:    true,
			Description: "The name of a named_expression in this resource, whose result becomes the value.",
		},
		"values": schema.ListAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Several fixed values. For a mix of fixed values and references, use array_value.",
		},
		"value": schema.SingleNestedAttribute{
			Optional:    true,
			Description: "One value, spelled out. `value_literal` and `value_reference` are shorthand for this.",
			Attributes:  value.Attributes,
		},
		"array_value": schema.ListNestedAttribute{
			Optional:     true,
			Description:  "Several values, spelled out. Needed when they mix fixed values and references.",
			NestedObject: value,
		},
	}
}

func fallbackBlock() schema.Block {
	return schema.SingleNestedBlock{
		Description: "What this expression produces when nothing else matched.",
		Attributes: map[string]schema.Attribute{
			"result": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "A flat, unconditional value.",
				Attributes:  BindingAttributes(),
			},
			"expression_ref": schema.StringAttribute{
				Optional:    true,
				Description: "The name of a named_expression in this resource.",
			},
		},
		Blocks: map[string]schema.Block{
			// Shorthand for a private expression whose only operation is a branches
			// block. `else` exists here and not on branches because that expression is
			// implicit, so its own fallback has nowhere else to live.
			"if":      branchBlock("Shorthand for a branching fallback."),
			"else_if": branchListBlock("Tried in order, after if."),
			"else": schema.SingleNestedBlock{
				Description: "The unconditional default for the shorthand above.",
				Attributes: map[string]schema.Attribute{
					"result": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "The value to fall back to. Required when the block is present.",
						Attributes:  BindingAttributes(),
					},
				},
			},
		},
	}
}
