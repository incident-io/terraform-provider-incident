package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// The API stores an escalation path as a tree: a branch node holds its two child paths
// inline, so the config nests one level deeper per branch. This resource stores the same
// path as a flat map of named sequences that reference each other by key, which is what
// removes the nesting limit.
//
// unflattenSequences turns the flat map into the tree the API wants, flattenSequences
// turns the tree back into the map, and validateSequences rejects the shapes that would
// make either of those impossible.

// nodeIDSeparator separates a sequence key from a node's index in a node ID we derive.
// validateSequences rejects a user-authored id containing it, so an id carrying one is
// always ours and can be parsed back into the sequence that produced it.
const nodeIDSeparator = "/"

// rootSequenceKey names the sequence an escalation path starts with when we can't recover
// the author's own name for it: an import, which has no prior config to read the naming
// out of, and any path not written through this resource.
const rootSequenceKey = "main"

// sequenceKeyPattern is what a sequence key may look like. Keys appear in node IDs and in
// derived names, so they're kept to something that reads well in both.
var sequenceKeyPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// derivedNodeID builds the ID for a node the author didn't name. It's derived rather than
// minted at random so it stays stable across applies, and so an ID coming back from the
// API tells us which sequence the node belongs to.
func derivedNodeID(sequenceKey string, index int) string {
	return fmt.Sprintf("%s%s%d", sequenceKey, nodeIDSeparator, index)
}

// sequenceKeyFromNodeID recovers the sequence key from a node ID we derived, reporting
// false for a user-authored one.
func sequenceKeyFromNodeID(id string) (string, bool) {
	key, _, found := strings.Cut(id, nodeIDSeparator)
	if !found || key == "" {
		return "", false
	}
	return key, true
}

// nodeID returns the ID a node will be stored under: the author's own, or one derived
// from its position.
func (n escalationPathBetaNode) nodeID(sequenceKey string, index int) string {
	if id := n.ID.ValueString(); id != "" {
		return id
	}
	return derivedNodeID(sequenceKey, index)
}

// blockNames lists the type blocks this node sets, in the order the schema declares them.
// Exactly one is valid; more than one leaves the conversions with nothing to go on.
func (n escalationPathBetaNode) blockNames() []string {
	var names []string
	if n.Level != nil {
		names = append(names, "level")
	}
	if n.NotifyChannel != nil {
		names = append(names, "notify_channel")
	}
	if n.Delay != nil {
		names = append(names, "delay")
	}
	if n.Branch != nil {
		names = append(names, "branch")
	}
	if n.Loop != nil {
		names = append(names, "loop")
	}
	return names
}

// decodeSequences decodes the sequences map into node slices keyed by sequence name.
func decodeSequences(ctx context.Context, sequences types.Map, diags *diag.Diagnostics) map[string][]escalationPathBetaNode {
	if sequences.IsNull() || sequences.IsUnknown() {
		return nil
	}

	var decoded map[string]escalationPathBetaSequence
	diags.Append(sequences.ElementsAs(ctx, &decoded, false)...)
	if diags.HasError() {
		return nil
	}

	out := make(map[string][]escalationPathBetaNode, len(decoded))
	for key, sequence := range decoded {
		var nodes []escalationPathBetaNode
		if !sequence.Nodes.IsNull() && !sequence.Nodes.IsUnknown() {
			diags.Append(sequence.Nodes.ElementsAs(ctx, &nodes, false)...)
		}
		out[key] = nodes
	}
	return out
}

// unflattenSequences walks the sequences from start and builds the tree the API stores,
// inlining each branch's target sequences as its then and else paths.
func unflattenSequences(ctx context.Context, start string, sequences map[string][]escalationPathBetaNode, diags *diag.Diagnostics) []client.EscalationPathNodePayloadV2 {
	return unflattenSequence(ctx, start, sequences, map[string]bool{}, map[string]bool{}, diags)
}

// unflattenSequence converts one sequence, recursing into the sequences its branches name.
//
// validateSequences rejects a sequences map that isn't a tree at plan time, but a plan
// holding unknown values skips that check, so this walk guards against a cycle and a
// sequence two branches name rather than trusting it.
func unflattenSequence(ctx context.Context, key string, sequences map[string][]escalationPathBetaNode, visiting, inlined map[string]bool, diags *diag.Diagnostics) []client.EscalationPathNodePayloadV2 {
	nodes, ok := sequences[key]
	if !ok {
		diags.AddError(
			"Unknown sequence",
			fmt.Sprintf("The escalation path references a sequence named %q, which doesn't exist.", key),
		)
		return nil
	}
	if visiting[key] {
		diags.AddError(
			"Escalation path loops back on itself",
			fmt.Sprintf("The sequence %q is reachable from itself through a branch. Escalation paths must be a tree; use a loop node to repeat.", key),
		)
		return nil
	}
	// Inlining it twice would send the API two copies sharing their node ids, which a read
	// then splits back into a sequence the config never named.
	if inlined[key] {
		diags.AddError(
			"Sequence branched to more than once",
			fmt.Sprintf("More than one branch names %q. Each sequence may be named by one branch; give the others their own copy.", key),
		)
		return nil
	}

	inlined[key] = true
	visiting[key] = true
	defer delete(visiting, key)

	out := make([]client.EscalationPathNodePayloadV2, 0, len(nodes))
	for index, node := range nodes {
		payload := client.EscalationPathNodePayloadV2{Id: node.nodeID(key, index)}

		switch {
		case node.Branch != nil:
			payload.Type = client.EscalationPathNodePayloadV2TypeIfElse
			ifElse := &client.EscalationPathNodeIfElsePayloadV2{
				Conditions: lo.ToPtr(node.Branch.If.toPayload(ctx, diags)),
				ThenPath:   unflattenSequence(ctx, node.Branch.Then.ValueString(), sequences, visiting, inlined, diags),
				ElsePath:   []client.EscalationPathNodePayloadV2{},
			}
			// else is optional: a branch with nothing on the false side just falls off the
			// end of the escalation path.
			if elseKey := node.Branch.Else.ValueString(); elseKey != "" {
				ifElse.ElsePath = unflattenSequence(ctx, elseKey, sequences, visiting, inlined, diags)
			}
			payload.IfElse = ifElse

		case node.Loop != nil:
			payload.Type = client.EscalationPathNodePayloadV2TypeRepeat
			payload.Repeat = &client.EscalationPathNodeRepeatV2{
				RepeatTimes: node.Loop.Times.ValueInt64(),
				ToNode:      node.Loop.BackTo.ValueString(),
			}

		case node.Level != nil:
			payload.Type = client.EscalationPathNodePayloadV2TypeLevel
			payload.Level = levelToPayload(ctx, node.Level, diags)

		case node.NotifyChannel != nil:
			payload.Type = client.EscalationPathNodePayloadV2TypeNotifyChannel
			payload.NotifyChannel = notifyChannelToPayload(ctx, node.NotifyChannel, diags)

		case node.Delay != nil:
			payload.Type = client.EscalationPathNodePayloadV2TypeDelay
			payload.Delay = delayToPayload(node.Delay)

		default:
			diags.AddError(
				"Empty escalation path node",
				fmt.Sprintf("Node %d of sequence %q sets none of level, notify_channel, delay, branch or loop.", index, key),
			)
			continue
		}

		out = append(out, payload)
	}

	return out
}

// escalationPathBetaPriorNames is the naming from the config we last wrote: the sequence the
// path started with, and the sequences themselves, whose branches say what their children
// were called.
//
// Sequence names are ours, not the API's: it stores a tree and knows nothing about them, so
// a read can only recover the author's names from here.
type escalationPathBetaPriorNames struct {
	start     string
	sequences map[string][]escalationPathBetaNode
}

// escalationPathBetaPriorNamesFrom reads the naming out of a model, or returns nothing when
// there's no prior model to read.
func escalationPathBetaPriorNamesFrom(ctx context.Context, prior *escalationPathBetaModel) escalationPathBetaPriorNames {
	if prior == nil {
		return escalationPathBetaPriorNames{}
	}

	// The naming is a hint for keeping a round-trip stable, not input we check, so a prior
	// model we can't read leaves us with the fallbacks rather than failing the read.
	var ignored diag.Diagnostics
	sequences := decodeSequences(ctx, prior.Sequences, &ignored)
	if ignored.HasError() {
		return escalationPathBetaPriorNames{}
	}

	return escalationPathBetaPriorNames{
		start:     prior.Start.ValueString(),
		sequences: sequences,
	}
}

// branchTargets returns the sequences the branch in this sequence pointed at, which are the
// names its children were last written under. A sequence holds at most one branch, so
// there's no question which one is meant.
func (p escalationPathBetaPriorNames) branchTargets(key string) (thenKey string, elseKey string) {
	if key == "" {
		return "", ""
	}
	for _, node := range p.sequences[key] {
		if node.Branch == nil {
			continue
		}
		return node.Branch.Then.ValueString(), node.Branch.Else.ValueString()
	}
	return "", ""
}

// flattenSequences walks the tree the API returns and splits it into named sequences,
// returning the key the path starts with alongside them. The names come from prior.
func flattenSequences(ctx context.Context, nodes []client.EscalationPathNodeV2, prior escalationPathBetaPriorNames, diags *diag.Diagnostics) (string, map[string][]escalationPathBetaNode) {
	sequences := map[string][]escalationPathBetaNode{}
	start := flattenSequence(ctx, nodes, prior.start, rootSequenceKey, prior, sequences, diags)
	return start, sequences
}

// flattenSequence converts one node array into a sequence, recursing into each branch's
// then and else paths, and returns the key it was stored under. priorKey is what the author
// called this sequence, empty if we can't tell.
func flattenSequence(ctx context.Context, nodes []client.EscalationPathNodeV2, priorKey, fallbackKey string, prior escalationPathBetaPriorNames, sequences map[string][]escalationPathBetaNode, diags *diag.Diagnostics) string {
	key := chooseSequenceKey(nodes, priorKey, fallbackKey, sequences)

	// Claim the key before recursing, or a child sequence could take it back.
	sequences[key] = nil

	convertedNodes := make([]escalationPathBetaNode, 0, len(nodes))
	for index, node := range nodes {
		// An ID we derived says where the node lives, which we already know, so leaving it
		// out keeps state matching a config that never wrote one.
		converted := escalationPathBetaNode{ID: types.StringNull()}
		if _, derived := sequenceKeyFromNodeID(node.Id); !derived {
			converted.ID = types.StringValue(node.Id)
		}

		switch {
		case node.IfElse != nil:
			// The API rejects a branch that isn't last in its node list, so this shouldn't
			// be reachable. Saying so beats splitting the trailing nodes into a sequence
			// nothing reaches, which is what carrying on would do.
			if index != len(nodes)-1 {
				diags.AddError(
					"Unexpected escalation path shape",
					fmt.Sprintf("The API returned a path with nodes after the branch in sequence %q, which it shouldn't accept. Please report this issue.", key),
				)
			}

			priorThen, priorElse := prior.branchTargets(priorKey)
			branch := &escalationPathBetaBranch{
				If:   escalationPathBetaBranchIfFromAPI(ctx, node.IfElse.Conditions, diags),
				Then: types.StringValue(flattenSequence(ctx, node.IfElse.ThenPath, priorThen, key+"_then", prior, sequences, diags)),
				Else: types.StringNull(),
			}
			if len(node.IfElse.ElsePath) > 0 {
				branch.Else = types.StringValue(flattenSequence(ctx, node.IfElse.ElsePath, priorElse, key+"_else", prior, sequences, diags))
			}
			converted.Branch = branch

		case node.Repeat != nil:
			converted.Loop = &escalationPathBetaLoop{
				BackTo: types.StringValue(node.Repeat.ToNode),
				Times:  types.Int64Value(node.Repeat.RepeatTimes),
			}

		case node.Level != nil:
			converted.Level = levelFromAPI(ctx, node.Level, diags)

		case node.NotifyChannel != nil:
			converted.NotifyChannel = notifyChannelFromAPI(ctx, node.NotifyChannel, diags)

		case node.Delay != nil:
			converted.Delay = delayFromAPI(node.Delay)

		default:
			diags.AddError(
				"Unsupported escalation path node",
				fmt.Sprintf("Node %q is a %s node, which incident_escalation_path_beta can't represent yet.", node.Id, node.Type),
			)
			continue
		}

		convertedNodes = append(convertedNodes, converted)
	}

	sequences[key] = convertedNodes
	return key
}

// chooseSequenceKey picks the name to store a sequence under. What the author called it is
// what keeps a round-trip stable, so priorKey comes first. Failing that (an import has no
// prior config) a node ID we derived carries the key the sequence had when we last wrote it.
// Failing both we fall back to the name of the branch that reached it, and add a suffix if
// something already holds the name.
func chooseSequenceKey(nodes []client.EscalationPathNodeV2, priorKey, fallbackKey string, taken map[string][]escalationPathBetaNode) string {
	if priorKey != "" {
		if _, exists := taken[priorKey]; !exists {
			return priorKey
		}
	}

	for _, node := range nodes {
		key, derived := sequenceKeyFromNodeID(node.Id)
		if !derived {
			continue
		}
		if _, exists := taken[key]; !exists {
			return key
		}
		break
	}

	if _, exists := taken[fallbackKey]; !exists {
		return fallbackKey
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", fallbackKey, suffix)
		if _, exists := taken[candidate]; !exists {
			return candidate
		}
	}
}

// validateSequences checks the shape of the sequences map: that every reference resolves,
// that the sequences form a tree rooted at start, and that nothing is stranded. These are
// the rules the flat representation adds, not a second copy of what the API checks.
func validateSequences(ctx context.Context, data *escalationPathBetaModel, diags *diag.Diagnostics) {
	// A value another resource computes isn't there to check yet, and validating around the
	// gap reports problems an apply won't hit.
	if data.Sequences.IsNull() || data.Sequences.IsUnknown() || data.Start.IsUnknown() {
		return
	}

	// Same for one sequence's nodes. decodeSequences reads a list another resource computes
	// as no nodes at all, which every check below would then report as an empty sequence,
	// and as an escalation path whose other sequences nothing reaches.
	if hasUnknownSequenceNodes(ctx, data.Sequences, diags) || diags.HasError() {
		return
	}

	sequences := decodeSequences(ctx, data.Sequences, diags)
	if diags.HasError() {
		return
	}

	// Same again for a name another resource computes. Every check below reads these
	// through ValueString, where an unknown is indistinguishable from an empty string, so
	// carrying on reports problems an apply won't hit.
	if hasUnknownSequenceReference(sequences) {
		return
	}

	start := data.Start.ValueString()
	nodeIDs := validateSequenceNodes(sequences, diags)
	validateSequenceReferences(sequences, nodeIDs, diags)
	validateSequenceTree(start, sequences, diags)
	validateSequenceLoops(start, sequences, diags)
}

// hasUnknownSequenceNodes reports whether any sequence's whole nodes list is still unknown.
// This can't be read off the decoded sequences, which is the point: decodeSequences turns an
// unknown list into no nodes, so by then it looks like a sequence the author left empty.
func hasUnknownSequenceNodes(ctx context.Context, sequences types.Map, diags *diag.Diagnostics) bool {
	var decoded map[string]escalationPathBetaSequence
	diags.Append(sequences.ElementsAs(ctx, &decoded, false)...)
	if diags.HasError() {
		return false
	}

	for _, sequence := range decoded {
		if sequence.Nodes.IsUnknown() {
			return true
		}
	}

	return false
}

// hasUnknownSequenceReference reports whether any name the checks resolve against is still
// unknown: a node's own id, or a branch or loop's reference to one.
func hasUnknownSequenceReference(sequences map[string][]escalationPathBetaNode) bool {
	for _, nodes := range sequences {
		for _, node := range nodes {
			if node.ID.IsUnknown() {
				return true
			}
			if node.Branch != nil && (node.Branch.Then.IsUnknown() || node.Branch.Else.IsUnknown()) {
				return true
			}
			if node.Loop != nil && node.Loop.BackTo.IsUnknown() {
				return true
			}
		}
	}
	return false
}

// sequenceParent is the branch a sequence hangs off, which is what says whether a loop
// inside it may name a given node.
type sequenceParent struct {
	key      string
	branchID string
}

// validateSequenceLoops checks each loop names a node the API will take. It only accepts
// the escalation path's first node, or a branch the loop sits underneath, and rejecting
// anything else here saves finding out on apply.
func validateSequenceLoops(start string, sequences map[string][]escalationPathBetaNode, diags *diag.Diagnostics) {
	startNodes, ok := sequences[start]
	if !ok || len(startNodes) == 0 {
		return
	}
	rootID := startNodes[0].nodeID(start, 0)

	parents := map[string]sequenceParent{}
	for _, key := range sortedKeys(sequences) {
		for index, node := range sequences[key] {
			if node.Branch == nil {
				continue
			}
			branchID := node.nodeID(key, index)
			for _, target := range []string{node.Branch.Then.ValueString(), node.Branch.Else.ValueString()} {
				if target != "" {
					parents[target] = sequenceParent{key: key, branchID: branchID}
				}
			}
		}
	}

	for _, key := range sortedKeys(sequences) {
		// The branches this sequence hangs off, nearest first. A cycle is reported
		// elsewhere; stopping on one here just keeps the walk finite.
		allowed := map[string]bool{rootID: true}
		seen := map[string]bool{key: true}
		for parent, ok := parents[key]; ok && !seen[parent.key]; parent, ok = parents[parent.key] {
			allowed[parent.branchID] = true
			seen[parent.key] = true
		}

		for index, node := range sequences[key] {
			if node.Loop == nil {
				continue
			}
			target := node.Loop.BackTo.ValueString()
			if target == "" || allowed[target] {
				continue
			}
			// An unknown node is already reported as such, and saying it's also in the
			// wrong place doesn't help.
			if !nodeExists(sequences, target) {
				continue
			}

			diags.AddAttributeError(
				path.Root("sequences").AtMapKey(key).AtName("nodes").AtListIndex(index).AtName("loop").AtName("back_to"),
				"Cannot loop back to this node",
				fmt.Sprintf("A loop may only go back to the escalation path's first node, or to a branch it sits underneath. %q is neither.", target),
			)
		}
	}
}

// nodeExists reports whether any sequence holds a node with this id.
func nodeExists(sequences map[string][]escalationPathBetaNode, id string) bool {
	for key, nodes := range sequences {
		for index, node := range nodes {
			if node.nodeID(key, index) == id {
				return true
			}
		}
	}
	return false
}

// validateSequenceNodes checks each sequence in isolation and returns every node ID the
// path will hold, which the reference checks need.
func validateSequenceNodes(sequences map[string][]escalationPathBetaNode, diags *diag.Diagnostics) map[string]bool {
	nodeIDs := map[string]bool{}

	for _, key := range sortedKeys(sequences) {
		nodes := sequences[key]
		sequencePath := path.Root("sequences").AtMapKey(key)

		if !sequenceKeyPattern.MatchString(key) {
			diags.AddAttributeError(
				sequencePath,
				"Invalid sequence name",
				fmt.Sprintf("%q must start with a letter and contain only letters, numbers, underscores and hyphens.", key),
			)
		}

		if len(nodes) == 0 {
			diags.AddAttributeError(
				sequencePath,
				"Empty sequence",
				fmt.Sprintf("Sequence %q has no nodes. Remove it, or give it something to do.", key),
			)
			continue
		}

		for index, node := range nodes {
			nodePath := sequencePath.AtName("nodes").AtListIndex(index)

			if id := node.ID.ValueString(); id != "" {
				if strings.Contains(id, nodeIDSeparator) {
					diags.AddAttributeError(
						nodePath.AtName("id"),
						"Invalid node id",
						fmt.Sprintf("%q must not contain %q, which is reserved for the ids we derive.", id, nodeIDSeparator),
					)
				}
				if nodeIDs[id] {
					diags.AddAttributeError(
						nodePath.AtName("id"),
						"Duplicate node id",
						fmt.Sprintf("More than one node is called %q. Node ids must be unique across the escalation path.", id),
					)
				}
			}
			nodeIDs[node.nodeID(key, index)] = true

			// The conversion to the API matches one block and ignores the rest, so a node
			// setting two silently loses one: this is the only place that can catch it.
			switch blocks := node.blockNames(); len(blocks) {
			case 1:
			case 0:
				diags.AddAttributeError(
					nodePath,
					"Empty node",
					fmt.Sprintf("Node %d of sequence %q sets none of level, notify_channel, delay, branch or loop. Give it something to do, or remove it.", index, key),
				)
			default:
				diags.AddAttributeError(
					nodePath,
					"Node does more than one thing",
					fmt.Sprintf("Node %d of sequence %q sets %s. A node must set exactly one of level, notify_channel, delay, branch or loop; split it into one node each.", index, key, strings.Join(blocks, " and ")),
				)
			}

			// An escalation continues down whichever sequence the branch chose and never
			// comes back, so anything after one would be unreachable. The API rejects it
			// too; catching it here names the sequence rather than a payload index.
			if node.Branch != nil && index != len(nodes)-1 {
				diags.AddAttributeError(
					nodePath,
					"Nodes after a branch",
					fmt.Sprintf("Sequence %q continues after a branch node. A branch must be the last node in its sequence; move what follows into the sequences it names.", key),
				)
			}

			// Same for a loop, which goes back to an earlier node rather than on to the
			// next one. The API rejects one that isn't last.
			if node.Loop != nil && index != len(nodes)-1 {
				diags.AddAttributeError(
					nodePath,
					"Nodes after a loop",
					fmt.Sprintf("Sequence %q continues after a loop node. A loop must be the last node in its sequence; nothing after it would ever run.", key),
				)
			}
		}
	}

	return nodeIDs
}

// validateSequenceReferences checks that every branch names a sequence that exists and
// every loop names a node that does.
func validateSequenceReferences(sequences map[string][]escalationPathBetaNode, nodeIDs map[string]bool, diags *diag.Diagnostics) {
	for _, key := range sortedKeys(sequences) {
		for index, node := range sequences[key] {
			nodePath := path.Root("sequences").AtMapKey(key).AtName("nodes").AtListIndex(index)

			if node.Branch != nil {
				for _, side := range []struct {
					name  string
					value types.String
				}{
					{"then", node.Branch.Then},
					{"else", node.Branch.Else},
				} {
					// Only an unset else ends the escalation path here; an empty string
					// is a name like any other, and no sequence has it.
					if side.value.IsNull() {
						continue
					}
					target := side.value.ValueString()
					if _, exists := sequences[target]; !exists {
						diags.AddAttributeError(
							nodePath.AtName("branch").AtName(side.name),
							"Unknown sequence",
							fmt.Sprintf("There is no sequence named %q.", target),
						)
					}
				}
			}

			if node.Loop != nil {
				if target := node.Loop.BackTo.ValueString(); !nodeIDs[target] {
					diags.AddAttributeError(
						nodePath.AtName("loop").AtName("back_to"),
						"Unknown node",
						fmt.Sprintf("There is no node with id %q. Give the node you want to loop back to an explicit id.", target),
					)
				}
			}
		}
	}
}

// validateSequenceTree checks the sequences form a tree rooted at start: every sequence
// reachable, none reachable twice, and none reachable from itself.
func validateSequenceTree(start string, sequences map[string][]escalationPathBetaNode, diags *diag.Diagnostics) {
	if _, exists := sequences[start]; !exists {
		diags.AddAttributeError(
			path.Root("start"),
			"Unknown sequence",
			fmt.Sprintf("start names %q, which isn't one of the sequences.", start),
		)
		return
	}

	// A sequence named by two branches would be inlined into the tree twice, so the path
	// the API stores would hold two copies that drift apart on the next read.
	parents := map[string]int{}
	for _, key := range sortedKeys(sequences) {
		for _, node := range sequences[key] {
			if node.Branch == nil {
				continue
			}
			for _, target := range []string{node.Branch.Then.ValueString(), node.Branch.Else.ValueString()} {
				if target != "" {
					parents[target]++
				}
			}
		}
	}

	if parents[start] > 0 {
		diags.AddAttributeError(
			path.Root("start"),
			"Start sequence is branched to",
			fmt.Sprintf("%q is where the escalation path begins, so nothing may branch to it. Use a loop node to go back to the start.", start),
		)
	}

	for _, key := range sortedKeys(sequences) {
		if key == start || parents[key] <= 1 {
			continue
		}
		diags.AddAttributeError(
			path.Root("sequences").AtMapKey(key),
			"Sequence branched to more than once",
			fmt.Sprintf("%d branches name %q. Each sequence may be named by one branch; give the others their own copy.", parents[key], key),
		)
	}

	reachable := map[string]bool{}
	var walk func(key string, stack map[string]bool)
	walk = func(key string, stack map[string]bool) {
		if stack[key] {
			diags.AddAttributeError(
				path.Root("sequences").AtMapKey(key),
				"Escalation path loops back on itself",
				fmt.Sprintf("%q is reachable from itself through a branch. Escalation paths must be a tree; use a loop node to repeat.", key),
			)
			return
		}
		if reachable[key] {
			return
		}
		reachable[key] = true

		stack[key] = true
		defer delete(stack, key)

		for _, node := range sequences[key] {
			if node.Branch == nil {
				continue
			}
			for _, target := range []string{node.Branch.Then.ValueString(), node.Branch.Else.ValueString()} {
				if _, exists := sequences[target]; target != "" && exists {
					walk(target, stack)
				}
			}
		}
	}
	walk(start, map[string]bool{})

	for _, key := range sortedKeys(sequences) {
		if !reachable[key] {
			diags.AddAttributeError(
				path.Root("sequences").AtMapKey(key),
				"Unreachable sequence",
				fmt.Sprintf("Nothing branches to %q and it isn't the start, so it would never run.", key),
			)
		}
	}
}

// sortedKeys returns a map's keys in a fixed order, so a config with several problems
// reports them the same way every run.
func sortedKeys(sequences map[string][]escalationPathBetaNode) []string {
	keys := lo.Keys(sequences)
	sort.Strings(keys)
	return keys
}

// escalationPathBetaSequencesToMap builds the sequences map for state.
func escalationPathBetaSequencesToMap(ctx context.Context, sequences map[string][]escalationPathBetaNode, diags *diag.Diagnostics) types.Map {
	nodeType := types.ObjectType{AttrTypes: escalationPathBetaNodeAttrTypes()}
	sequenceType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"nodes": types.ListType{ElemType: nodeType},
	}}

	elements := make(map[string]attr.Value, len(sequences))
	for key, nodes := range sequences {
		list, d := types.ListValueFrom(ctx, nodeType, nodes)
		diags.Append(d...)

		sequence, d := types.ObjectValue(sequenceType.AttrTypes, map[string]attr.Value{"nodes": list})
		diags.Append(d...)

		elements[key] = sequence
	}

	value, d := types.MapValue(sequenceType, elements)
	diags.Append(d...)
	return value
}
