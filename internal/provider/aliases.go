package provider

import "fmt"

// The resources that spent v6 behind a `_beta` suffix lost it in v7. Renaming a resource
// type is normally a breaking change - a configuration naming the old type no longer
// matches anything, and its state has nowhere to go - so each of them stays registered
// under its old name as well as its new one. Upgrading to v7 needs no change to a
// configuration written against the beta resources: both names are the same
// implementation, backed by the same schema and the same API, so the old name keeps
// planning and applying exactly as it did.
//
// The old names are deprecated rather than supported. Every plan that touches one warns,
// naming the new type and the `moved` block that gets there, and they go in v8. The move
// itself is state Terraform asks the provider to carry, which is what MoveState does on
// each of these resources: see migrateStateMover.
//
// An alias is a second registration of the same Go type, distinguished only by the
// betaAlias it is constructed with. The zero value is the resource's own registration.

// betaAlias marks an instance registered under the `_beta` name a resource had before
// v7, rather than under its own.
type betaAlias struct {
	// suffix is the type name to answer to, e.g. "_schedule_beta".
	suffix string
	// renamedTo is the type name the alias points at, e.g. "incident_schedule".
	renamedTo string
	// example is the resource label the documented `moved` block uses.
	example string
}

var (
	scheduleBetaAlias = betaAlias{
		suffix:    "_schedule_beta",
		renamedTo: "incident_schedule",
		example:   "primary",
	}
	scheduleRotationBetaAlias = betaAlias{
		suffix:    "_schedule_rotation_beta",
		renamedTo: "incident_schedule_rotation",
		example:   "weekdays",
	}
	escalationPathBetaAlias = betaAlias{
		suffix:    "_escalation_path_beta",
		renamedTo: "incident_escalation_path",
		example:   "urgent",
	}
	alertSourceBetaAlias = betaAlias{
		suffix:    "_alert_source_beta",
		renamedTo: "incident_alert_source",
		example:   "http",
	}
	alertSourceAttributeBetaAlias = betaAlias{
		suffix:    "_alert_source_attribute_beta",
		renamedTo: "incident_alert_source_attribute",
		example:   "team",
	}
)

// isAlias reports whether this instance is one of the deprecated `_beta` registrations.
func (a betaAlias) isAlias() bool {
	return a.suffix != ""
}

// typeName returns the type name to register under: the alias's when this is one, and the
// resource's own otherwise. ownSuffix is what the resource would answer to anyway, so a
// resource states its name once and the alias reads as an override of it.
func (a betaAlias) typeName(providerTypeName, ownSuffix string) string {
	if !a.isAlias() {
		return providerTypeName + ownSuffix
	}

	return providerTypeName + a.suffix
}

// oldName is the full type name of the alias, for the messages that have to spell it.
func (a betaAlias) oldName() string {
	return "incident" + a.suffix
}

// deprecationMessage is the warning shown against every resource still written under an
// old name, or empty for the resource's own registration. The framework raises it once per
// resource instance in the plan, so it says what to do rather than only that something is
// wrong.
func (a betaAlias) deprecationMessage() string {
	if !a.isAlias() {
		return ""
	}

	return fmt.Sprintf(
		"`%s` was renamed to `%s` in v7.0 and will be removed in v8.0. Both names manage the "+
			"same object, so renaming it plans no change:\n\nmoved {\n  from = %s.%s\n  to   = %s.%s\n}",
		a.oldName(), a.renamedTo, a.oldName(), a.example, a.renamedTo, a.example,
	)
}

// description replaces the resource's own documentation when this is an alias, so the
// generated page for an old name says where the resource went rather than restating a
// page that already exists under the new name.
func (a betaAlias) description(own string) string {
	if !a.isAlias() {
		return own
	}

	return fmt.Sprintf("`%[1]s` was renamed to `%[2]s` in v7.0."+`

This name still works and still manages the same thing, so upgrading to v7 needs no change
to a configuration that uses it. It is deprecated: every plan naming it warns, and it will
be removed in v8.0.

To move onto the new name:

`+"```terraform"+`
moved {
  from = %[1]s.%[3]s
  to   = %[2]s.%[3]s
}
`+"```"+`

Both names are the same resource, backed by the same schema and the same API, so the move
carries your state across and the plan after it is empty. See `+"`%[2]s`"+` for the
documentation.`,
		a.oldName(), a.renamedTo, a.example,
	)
}
