package models

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// BindingDataSourceAttributes is BindingAttributes with every field computed, for data
// sources that reuse the resource's Binding model.
func BindingDataSourceAttributes() map[string]schema.Attribute {
	value := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"literal": schema.StringAttribute{
				Computed:    true,
				Description: "A fixed value. A catalog entry ID is a literal, not a reference.",
			},
			"reference": schema.StringAttribute{
				Computed:    true,
				Description: "A reference into the scope, such as `payload.team`.",
			},
		},
	}

	return map[string]schema.Attribute{
		"value_literal": schema.StringAttribute{
			Computed:    true,
			Description: "A fixed value. A catalog entry ID is a literal, not a reference.",
		},
		"value_reference": schema.StringAttribute{
			Computed:    true,
			Description: "A reference into the scope, such as `payload.team`.",
		},
		"expression_ref": schema.StringAttribute{
			Computed:    true,
			Description: "The name of a named_expression in this resource, whose result becomes the value.",
		},
		"values": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Several fixed values. For a mix of fixed values and references, use array_value.",
		},
		"value": schema.SingleNestedAttribute{
			Computed:    true,
			Description: "One value, spelled out. `value_literal` and `value_reference` are shorthand for this.",
			Attributes:  value.Attributes,
		},
		"array_value": schema.ListNestedAttribute{
			Computed:     true,
			Description:  "Several values, spelled out. Needed when they mix fixed values and references.",
			NestedObject: value,
		},
	}
}

// BindingDataSourceAttribute is one whole field taking a binding, the data source mirror of
// BindingAttribute. BindingDataSourceAttributes spreads the same attributes across a
// resource that is itself a binding, such as an alert source attribute.
func BindingDataSourceAttribute(description string) schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:    true,
		Description: description,
		Attributes:  BindingDataSourceAttributes(),
	}
}

func ExpressionBlockDataSource() schema.Block {
	return schema.SingleNestedBlock{
		Description: "The expression this field is bound to. Declaring it binds its result.",
		Attributes:  expressionDataSourceAttributes(false),
		Blocks:      expressionDataSourceBlocks(),
	}
}

func NamedExpressionBlockDataSource() schema.Block {
	return schema.ListNestedBlock{
		Description: "An expression this resource owns, addressed by name.",
		NestedObject: schema.NestedBlockObject{
			Attributes: expressionDataSourceAttributes(true),
			Blocks:     expressionDataSourceBlocks(),
		},
	}
}

func expressionDataSourceAttributes(named bool) map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{
		"start_from": schema.StringAttribute{
			Computed:    true,
			Description: `Where the expression starts: "payload", "alert", "." for a branches-only expression, or a scope path.`,
		},
	}

	if named {
		attributes["name"] = schema.StringAttribute{
			Computed:    true,
			Description: "A name for this expression, unique within this resource, referenced by expression_ref.",
		}
		attributes["label"] = schema.StringAttribute{
			Computed:    true,
			Description: "What the dashboard shows for this expression.",
		}
	}

	return attributes
}

func expressionDataSourceBlocks() map[string]schema.Block {
	return map[string]schema.Block{
		"operation": schema.ListNestedBlock{
			Description: "An ordered pipeline. Each operation feeds the next.",
			NestedObject: schema.NestedBlockObject{
				Attributes: operationOptionDataSourceAttributes(),
				Blocks: map[string]schema.Block{
					"branches": branchesBlockDataSource(),
				},
			},
		},
		"fallback": fallbackBlockDataSource(),
	}
}

func operationOptionDataSourceAttributes() map[string]schema.Attribute {
	empty := func(description string) schema.Attribute {
		return schema.SingleNestedAttribute{
			Computed:    true,
			Description: description,
			Attributes:  map[string]schema.Attribute{},
		}
	}

	return map[string]schema.Attribute{
		"parse": schema.SingleNestedAttribute{
			Computed:    true,
			Description: "Evaluates a function against the current value.",
			Attributes: map[string]schema.Attribute{
				"function": schema.StringAttribute{
					Computed:    true,
					Description: "JavaScript evaluated against the current value, bound to `$`.",
				},
				"as": schema.StringAttribute{
					Computed:    true,
					Description: "The type this returns.",
				},
				"array": schema.BoolAttribute{
					Computed:    true,
					Description: "Whether this returns several values rather than one.",
				},
			},
		},
		"navigate": schema.SingleNestedAttribute{
			Computed:    true,
			Description: "Follows an attribute of the current value.",
			Attributes: map[string]schema.Attribute{
				"to": schema.StringAttribute{
					Computed:    true,
					Description: "The catalog attribute to follow.",
				},
			},
		},
		"cast": schema.SingleNestedAttribute{
			Computed:    true,
			Description: "Converts the current value to another type.",
			Attributes: map[string]schema.Attribute{
				"as": schema.StringAttribute{
					Computed:    true,
					Description: "The type to convert to.",
				},
			},
		},
		"concatenate": schema.SingleNestedAttribute{
			Computed:    true,
			Description: "Adds the values behind another reference to the current value, keeping each value once.",
			Attributes: map[string]schema.Attribute{
				"with": schema.StringAttribute{
					Computed:    true,
					Description: "The reference whose values are added.",
				},
			},
		},
		"filter": schema.SingleNestedAttribute{
			Computed:    true,
			Description: "Keeps the values matching these conditions.",
			Attributes:  conditionsDataSourceAttributes(),
		},
		"first":  empty("Takes the first value."),
		"count":  empty("Counts the values."),
		"sum":    empty("Adds the values together."),
		"min":    empty("Takes the smallest value."),
		"max":    empty("Takes the largest value."),
		"random": empty("Takes one value at random."),
	}
}

func branchesBlockDataSource() schema.Block {
	return schema.SingleNestedBlock{
		Description: `A lookup table, evaluated in order until one matches.`,
		Attributes: map[string]schema.Attribute{
			"as": schema.StringAttribute{
				Computed:    true,
				Description: "The type every branch result returns.",
			},
			"array": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether each branch returns several values rather than one.",
			},
		},
		Blocks: map[string]schema.Block{
			"if":      branchBlockDataSource("The first branch to try."),
			"else_if": branchListBlockDataSource("Tried in order, after if."),
		},
	}
}

func branchBlockDataSource(description string) schema.Block {
	return schema.SingleNestedBlock{
		Description: description,
		Attributes:  branchDataSourceAttributes(),
	}
}

func branchListBlockDataSource(description string) schema.Block {
	return schema.ListNestedBlock{
		Description:  description,
		NestedObject: schema.NestedBlockObject{Attributes: branchDataSourceAttributes()},
	}
}

func branchDataSourceAttributes() map[string]schema.Attribute {
	attributes := conditionsDataSourceAttributes()
	attributes["result"] = schema.SingleNestedAttribute{
		Computed:    true,
		Description: "The value this branch produces.",
		Attributes:  BindingDataSourceAttributes(),
	}

	return attributes
}

func conditionsDataSourceAttributes() map[string]schema.Attribute {
	condition := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"subject": schema.StringAttribute{
				Computed:    true,
				Description: "The reference this condition tests.",
			},
			"operation": schema.StringAttribute{
				Computed:    true,
				Description: "How the subject is tested.",
			},
			"params": schema.ListNestedAttribute{
				Computed:     true,
				Description:  "Positional parameters for the operation.",
				NestedObject: schema.NestedAttributeObject{Attributes: BindingDataSourceAttributes()},
			},
		},
	}

	return map[string]schema.Attribute{
		"conditions": schema.ListNestedAttribute{
			Computed:     true,
			Description:  "All of these must hold. Sugar for a single condition group.",
			NestedObject: condition,
		},
		"condition_groups": schema.ListNestedAttribute{
			Computed:    true,
			Description: "Groups are OR'd; conditions within a group are AND'd.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"conditions": schema.ListNestedAttribute{
						Computed:     true,
						Description:  "All of these must hold for the group to hold.",
						NestedObject: condition,
					},
				},
			},
		},
	}
}

func fallbackBlockDataSource() schema.Block {
	return schema.SingleNestedBlock{
		Description: "What this expression produces when nothing else matched.",
		Attributes: map[string]schema.Attribute{
			"result": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "A flat, unconditional value.",
				Attributes:  BindingDataSourceAttributes(),
			},
			"expression_ref": schema.StringAttribute{
				Computed:    true,
				Description: "The name of a named_expression in this resource.",
			},
		},
		Blocks: map[string]schema.Block{
			"if":      branchBlockDataSource("Shorthand for a branching fallback."),
			"else_if": branchListBlockDataSource("Tried in order, after if."),
			"else": schema.SingleNestedBlock{
				Description: "The unconditional default for the shorthand above.",
				Attributes: map[string]schema.Attribute{
					"result": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "The value to fall back to.",
						Attributes:  BindingDataSourceAttributes(),
					},
				},
			},
		},
	}
}
