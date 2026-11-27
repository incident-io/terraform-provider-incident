package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An escalation path is managed through the V2 or the V3 API, and its config says which.
// The two differ in two places:
//
//   - a target's high/low setting is urgency in V2 and interruption_level in V3;
//   - a branch tests the alert priority (priority_one_of) in V2, and the escalation urgency
//     (urgency_one_of) in V3. Neither API accepts the other's.
//
// A path setting any V2 field goes through V2, so existing configs keep working unchanged.
// Anything else goes through V3, which can also read every path V2 can. A path setting
// fields from both is an error, since neither API could take it.

type escalationPathAPIVersion int

const (
	escalationPathAPIV2 escalationPathAPIVersion = iota + 2
	escalationPathAPIV3
)

func (v escalationPathAPIVersion) String() string {
	return fmt.Sprintf("V%d", int(v))
}

// escalationPathVersionedFields lists the version-specific fields a path sets, as the
// paths a user would recognise in their config.
type escalationPathVersionedFields struct {
	v2 []string
	v3 []string
}

// version is the API this path goes through: V2 when it sets any V2 field, and V3
// otherwise.
func (f escalationPathVersionedFields) version() escalationPathAPIVersion {
	if len(f.v2) > 0 {
		return escalationPathAPIV2
	}
	return escalationPathAPIV3
}

// escalationPathFields walks a path's sequences for the fields that pick its version. An
// unknown value counts as set: it's a value another resource computes, and it will almost
// always resolve to one.
//
// A templated path has no sequences, so it sets nothing and goes through V3.
func escalationPathFields(ctx context.Context, data *escalationPathModel, diags *diag.Diagnostics) escalationPathVersionedFields {
	var fields escalationPathVersionedFields
	if data == nil {
		return fields
	}

	sequences := decodeSequences(ctx, data.Sequences, diags)
	for _, key := range sortedKeys(sequences) {
		for index, node := range sequences[key] {
			nodePath := fmt.Sprintf("sequences[%q].nodes[%d]", key, index)

			for _, holder := range escalationPathNodeTargets(node) {
				for targetIndex, target := range decodeTargets(ctx, holder.targets, diags) {
					targetPath := fmt.Sprintf("%s.%s.targets[%d]", nodePath, holder.block, targetIndex)
					if !target.Urgency.IsNull() {
						fields.v2 = append(fields.v2, targetPath+".urgency")
					}
					if !target.InterruptionLevel.IsNull() {
						fields.v3 = append(fields.v3, targetPath+".interruption_level")
					}
				}
			}

			if node.Branch != nil && node.Branch.If != nil {
				ifPath := nodePath + ".branch.if"
				if !node.Branch.If.PriorityOneOf.IsNull() {
					fields.v2 = append(fields.v2, ifPath+".priority_one_of")
				}
				if !node.Branch.If.UrgencyOneOf.IsNull() {
					fields.v3 = append(fields.v3, ifPath+".urgency_one_of")
				}
			}
		}
	}

	return fields
}

// escalationPathTargetHolder is a node block holding targets, and the targets it holds.
type escalationPathTargetHolder struct {
	block   string
	targets types.List
}

// escalationPathNodeTargets returns the blocks of a node that hold targets, in schema
// order. A valid node has at most one.
func escalationPathNodeTargets(node escalationPathNode) []escalationPathTargetHolder {
	var holders []escalationPathTargetHolder
	if node.Level != nil {
		holders = append(holders, escalationPathTargetHolder{"level", node.Level.Targets})
	}
	if node.NotifyChannel != nil {
		holders = append(holders, escalationPathTargetHolder{"notify_channel", node.NotifyChannel.Targets})
	}
	return holders
}

// escalationPathVersionOf is the API version a plan or state goes through.
func escalationPathVersionOf(ctx context.Context, data *escalationPathModel, diags *diag.Diagnostics) escalationPathAPIVersion {
	return escalationPathFields(ctx, data, diags).version()
}

// validateEscalationPathVersion rejects a path mixing V2 and V3 fields, listing every one
// of them so the author can see what to change in one pass rather than one error at a time.
func validateEscalationPathVersion(ctx context.Context, data *escalationPathModel, diags *diag.Diagnostics) {
	fields := escalationPathFields(ctx, data, diags)
	if len(fields.v2) == 0 || len(fields.v3) == 0 {
		return
	}

	diags.AddError("Escalation path mixes deprecated and current attributes", escalationPathMixedFieldsDetail(fields))
}

func escalationPathMixedFieldsDetail(fields escalationPathVersionedFields) string {
	var b strings.Builder
	b.WriteString("This escalation path uses the deprecated urgency and priority_one_of attributes ")
	b.WriteString("together with their replacements, interruption_level and urgency_one_of. ")
	b.WriteString("The two sets of attributes correspond to versions 2 and 3 of the incident.io public API, ")
	b.WriteString("so an escalation path must use one set throughout. ")
	b.WriteString("Replace the deprecated attributes with interruption_level and urgency_one_of.\n\n")

	b.WriteString("Deprecated (urgency, priority_one_of):\n")
	for _, field := range fields.v2 {
		fmt.Fprintf(&b, "  - %s\n", field)
	}

	b.WriteString("\nCurrent (interruption_level, urgency_one_of):\n")
	for _, field := range fields.v3 {
		fmt.Fprintf(&b, "  - %s\n", field)
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// validateEscalationPathTargetInterruptionLevels requires every target to say how loudly
// to page. A target setting both names is reported by validateEscalationPathVersion.
//
// A path that still uses the deprecated attributes is told to set urgency, since following
// advice to set interruption_level there would only lead to the mixed-attributes error.
func validateEscalationPathTargetInterruptionLevels(ctx context.Context, data *escalationPathModel, diags *diag.Diagnostics) {
	name, detail := "interruption_level", `Set interruption_level on this target, to "high" or "low".`
	if escalationPathVersionOf(ctx, data, diags) == escalationPathAPIV2 {
		name, detail = "urgency", `Set urgency on this target, to "high" or "low", as the rest of this escalation path does. `+
			"urgency is deprecated: to replace it with interruption_level, change every target and branch in the escalation path at the same time."
	}

	sequences := decodeSequences(ctx, data.Sequences, diags)
	for _, key := range sortedKeys(sequences) {
		for index, node := range sequences[key] {
			for _, holder := range escalationPathNodeTargets(node) {
				for targetIndex, target := range decodeTargets(ctx, holder.targets, diags) {
					if !target.Urgency.IsNull() || !target.InterruptionLevel.IsNull() {
						continue
					}
					diags.AddAttributeError(
						path.Root("sequences").AtMapKey(key).AtName("nodes").AtListIndex(index).
							AtName(holder.block).AtName("targets").AtListIndex(targetIndex).AtName(name),
						"Missing interruption level",
						detail,
					)
				}
			}
		}
	}
}
