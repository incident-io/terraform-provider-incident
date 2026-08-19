package richtexttypes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/incident-io/terraform-provider-incident/internal/provider/jsontypes"
)

// Error slugs, shared with the product's own template parser via the testdata fixtures: they
// name the failure mode, not the wording, which each implementation owns.
const (
	slugUnclosedVariable  = "unclosed_variable"
	slugUnknownFilter     = "unknown_filter"
	slugInvalidFilterArg  = "invalid_filter_argument"
	slugEmptyVariableName = "empty_variable_name"
	slugNestedVariable    = "nested_variable"
)

// Template syntax.
const (
	openBrace         = "{{"
	closeBrace        = "}}"
	filterSeparator   = "|"
	argumentSeparator = ":"
	filterOmitIfUnset = "omit_if_unset"
	filterTruncate    = "truncate"

	// Parsing tolerates any spacing; emission always uses this, so it's deterministic.
	canonicalFilterJoin = " " + filterSeparator + " "
	canonicalArgJoin    = argumentSeparator + " "
)

const (
	nodeDoc       = "doc"
	nodeParagraph = "paragraph"
	nodeText      = "text"
	nodeLineBreak = "lineBreak"
	nodeVarSpec   = "varSpec"

	fieldType    = "type"
	fieldAttrs   = "attrs"
	fieldContent = "content"
)

// varSpec attrs. label and missing are display state: read, never emitted.
const (
	attrName        = "name"
	attrOmitIfUnset = "omitIfUnset"
	attrTruncateTo  = "truncateTo"
	attrLabel       = "label"
	attrMissing     = "missing"
)

// ParseError is a template syntax error. Slug identifies the failure mode for the
// shared fixtures; Summary and Detail become the plan-time diagnostic.
type ParseError struct {
	Slug    string
	Summary string
	Detail  string
}

func (e *ParseError) Error() string { return e.Detail }

func parseErrorf(slug, summary, format string, args ...any) *ParseError {
	return &ParseError{Slug: slug, Summary: summary, Detail: fmt.Sprintf(format, args...)}
}

// Two or more newlines start a new paragraph; a single newline stays inside one as a
// lineBreak.
var paragraphBreak = regexp.MustCompile(`\n{2,}`)

// node is the emitted ProseMirror shape. Everything is omitempty because we emit only
// what is set: the server stores our bytes verbatim, so any attr we add here joins a
// compatibility surface we can never take back.
type node struct {
	Type    string        `json:"type"`
	Text    string        `json:"text,omitempty"`
	Attrs   *varSpecAttrs `json:"attrs,omitempty"`
	Content []node        `json:"content,omitempty"`
}

// varSpecAttrs carries only the attrs a template can express.
type varSpecAttrs struct {
	Name        string `json:"name"`
	OmitIfUnset bool   `json:"omitIfUnset,omitempty"`
	TruncateTo  int    `json:"truncateTo,omitempty"`
}

// ToDocument converts a config string into the AST JSON sent to the API.
func ToDocument(template string) (json.RawMessage, error) {
	paragraphs := paragraphBreak.Split(template, -1)
	breaks := paragraphBreak.FindAllString(template, -1)
	content := make([]node, 0, len(paragraphs))

	// Offsets are template-relative, so a diagnostic points somewhere the user can find.
	offset := 0
	for i, paragraph := range paragraphs {
		inline, err := parseInline(paragraph, offset)
		if err != nil {
			return nil, err
		}

		content = append(content, node{Type: nodeParagraph, Content: inline})

		offset += len(paragraph)
		if i < len(breaks) {
			offset += len(breaks[i])
		}
	}

	return json.Marshal(node{Type: nodeDoc, Content: content})
}

// parseInline scans one paragraph into text, lineBreak and varSpec nodes. Scanning is
// non-greedy: each "{{" pairs with the nearest "}}", so "{{a}} and {{b}}" is two
// variables rather than one named "a}} and {{b". base is the paragraph's offset within
// the template, for diagnostics.
func parseInline(paragraph string, base int) ([]node, error) {
	var (
		nodes    []node
		consumed int
	)

	for rest := paragraph; rest != ""; {
		start := strings.Index(rest, openBrace)
		if start < 0 {
			return appendText(nodes, rest), nil
		}

		nodes = appendText(nodes, rest[:start])

		body := rest[start+len(openBrace):]

		end := strings.Index(body, closeBrace)
		if end < 0 {
			return nil, parseErrorf(slugUnclosedVariable, "Invalid template syntax",
				"Unclosed %q at offset %d. Use a raw JSON document if you need a literal %q.",
				openBrace, base+consumed+start, openBrace)
		}

		attrs, err := parseVariable(body[:end])
		if err != nil {
			return nil, err
		}

		nodes = append(nodes, node{Type: nodeVarSpec, Attrs: attrs})

		variable := start + len(openBrace) + end + len(closeBrace)
		consumed += variable
		rest = rest[variable:]
	}

	return nodes, nil
}

// parseVariable reads the inside of a "{{ }}" into its name and filters.
func parseVariable(inner string) (*varSpecAttrs, error) {
	if strings.Contains(inner, openBrace) {
		return nil, parseErrorf(slugNestedVariable, "Invalid template syntax",
			"Nested %q in %q.", openBrace, openBrace+inner+closeBrace)
	}

	parts := strings.Split(inner, filterSeparator)

	attrs := &varSpecAttrs{Name: strings.TrimSpace(parts[0])}
	if attrs.Name == "" {
		return nil, parseErrorf(slugEmptyVariableName, "Invalid template syntax",
			"Empty variable name in %q.", openBrace+inner+closeBrace)
	}

	for _, part := range parts[1:] {
		filter, arg, hasArg := strings.Cut(part, argumentSeparator)
		filter, arg = strings.TrimSpace(filter), strings.TrimSpace(arg)

		switch filter {
		case filterOmitIfUnset:
			if hasArg {
				return nil, parseErrorf(slugInvalidFilterArg, "Invalid filter argument",
					"%s takes no argument, got %q.", filterOmitIfUnset, arg)
			}
			attrs.OmitIfUnset = true

		case filterTruncate:
			limit, err := strconv.Atoi(arg)
			if !hasArg || err != nil || limit < 1 {
				return nil, parseErrorf(slugInvalidFilterArg, "Invalid filter argument",
					"%s expects a positive integer, got %q.", filterTruncate, arg)
			}
			attrs.TruncateTo = limit

		default:
			return nil, parseErrorf(slugUnknownFilter, "Unknown variable filter",
				"%q is not a recognised filter. Supported: %s, %s.",
				filter, filterTruncate, filterOmitIfUnset)
		}
	}

	return attrs, nil
}

// appendText splits literal text on single newlines, emitting a lineBreak between the
// pieces. Empty pieces produce no node, so adjacent variables get no empty text node
// between them.
func appendText(nodes []node, text string) []node {
	if text == "" {
		return nodes
	}

	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			nodes = append(nodes, node{Type: nodeLineBreak})
		}
		if line != "" {
			nodes = append(nodes, node{Type: nodeText, Text: line})
		}
	}

	return nodes
}

// wireNode is the parsed ProseMirror shape. Attrs stays raw to distinguish an absent
// attr from one explicitly null, and to spot attrs outside the known set.
type wireNode struct {
	Type    string                     `json:"type"`
	Text    string                     `json:"text"`
	Attrs   map[string]json.RawMessage `json:"attrs"`
	Marks   []json.RawMessage          `json:"marks"`
	Content []wireNode                 `json:"content"`
}

// FromDocument converts stored AST JSON back to template form, and is the
// expressibility predicate. ok is false when the document can't be losslessly
// represented — callers then keep the AST verbatim.
//
// A false-optimistic answer silently drops content from a live alert, so the emitted
// template is re-parsed and compared against the original before being accepted.
// Structural checks alone are not enough: adjacent lineBreaks, a newline inside a text
// node, and an empty paragraph between two others all pass every rule below yet emit a
// template that re-parses into a different tree. Doing the round trip is the only honest
// answer, and it closes the cases nobody has thought of yet.
func FromDocument(doc json.RawMessage) (string, bool) {
	template, ok := emitDocument(doc)
	if !ok {
		return "", false
	}

	reparsed, err := ToDocument(template)
	if err != nil {
		return "", false
	}

	original, err := normalise(string(doc))
	if err != nil {
		return "", false
	}

	if !jsontypes.JSONStringsEqual(string(reparsed), string(original)) {
		return "", false
	}

	return template, true
}

// wireEnvelope captures the two wrapped shapes a stored literal can carry alongside the
// bare node: the current {schema_version, text_node} envelope the dashboard's template
// editor writes, and the legacy {root, value_markdown} one. Both wrap the same tree under
// a different key.
type wireEnvelope struct {
	TextNode   *json.RawMessage `json:"text_node"`
	LegacyRoot *json.RawMessage `json:"root"`
}

// unwrapDocument returns the bare ProseMirror node the rest of this file works on, peeling
// off whichever envelope wraps it. A bare node, an envelope with neither key, and anything
// that isn't JSON all pass through for the caller's own parse to judge.
//
// Both callers need it or neither does: emitDocument alone would produce a template that
// FromDocument then compares against a still-enveloped original and rejects, and normalise
// alone would leave the AST in state after an import.
func unwrapDocument(doc json.RawMessage) json.RawMessage {
	var envelope wireEnvelope
	if err := json.Unmarshal(doc, &envelope); err != nil {
		return doc
	}

	switch {
	case envelope.TextNode != nil:
		return *envelope.TextNode
	case envelope.LegacyRoot != nil:
		return *envelope.LegacyRoot
	default:
		return doc
	}
}

// emitDocument renders a document to template form if its shape allows it. Callers want
// FromDocument, which additionally proves the result round-trips.
func emitDocument(doc json.RawMessage) (string, bool) {
	var root wireNode
	if err := json.Unmarshal(unwrapDocument(doc), &root); err != nil || root.Type != nodeDoc {
		return "", false
	}

	// A doc with no blocks is malformed — the schema requires at least one. Calling it an
	// empty template would put "" in state and diff against the stored AST on every plan,
	// since normalise turns an empty template into a doc holding one empty paragraph.
	if len(root.Content) == 0 {
		return "", false
	}

	paragraphs := make([]string, 0, len(root.Content))
	for _, block := range root.Content {
		if block.Type != nodeParagraph || len(block.Marks) > 0 {
			return "", false
		}

		paragraph, ok := emitInline(block.Content)
		if !ok {
			return "", false
		}

		paragraphs = append(paragraphs, paragraph)
	}

	return strings.Join(paragraphs, "\n\n"), true
}

func emitInline(content []wireNode) (string, bool) {
	var out strings.Builder

	for _, inline := range content {
		// VarSpec keeps a Marks field of its own, so a bolded pill reaches here too.
		if len(inline.Marks) > 0 {
			return "", false
		}

		switch inline.Type {
		case nodeText:
			// No escape syntax, so this would re-parse as a variable.
			if strings.Contains(inline.Text, openBrace) {
				return "", false
			}
			out.WriteString(inline.Text)

		case nodeLineBreak:
			out.WriteString("\n")

		case nodeVarSpec:
			variable, ok := emitVariable(inline.Attrs)
			if !ok {
				return "", false
			}
			out.WriteString(variable)

		default:
			return "", false
		}
	}

	return out.String(), true
}

// emitVariable renders a varSpec's attrs back to "{{ }}" form.
func emitVariable(attrs map[string]json.RawMessage) (string, bool) {
	var name string
	if err := json.Unmarshal(attrs[attrName], &name); err != nil || name == "" {
		return "", false
	}

	// Pointers so an explicit null reads the same as an absent attr.
	var (
		truncateTo  *int
		omitIfUnset *bool
	)

	for key, raw := range attrs {
		switch key {
		case attrName:
			// Already read.

		case attrLabel, attrMissing:
			// Ignored rather than rejected: dashboard-authored documents carry both, and
			// must still collapse to their template form.

		case attrOmitIfUnset:
			if err := json.Unmarshal(raw, &omitIfUnset); err != nil {
				return "", false
			}

		case attrTruncateTo:
			if err := json.Unmarshal(raw, &truncateTo); err != nil {
				return "", false
			}
			// The server ignores a limit below 1 rather than truncating, so emitting it
			// would round-trip to something that means something else.
			if truncateTo != nil && *truncateTo < 1 {
				return "", false
			}

		default:
			// An unknown attr would be silently dropped on emission.
			return "", false
		}
	}

	// Fixed order, so a document has exactly one template form.
	var filters []string
	if truncateTo != nil {
		filters = append(filters, filterTruncate+canonicalArgJoin+strconv.Itoa(*truncateTo))
	}
	if omitIfUnset != nil && *omitIfUnset {
		filters = append(filters, filterOmitIfUnset)
	}

	variable := name
	if len(filters) > 0 {
		variable += canonicalFilterJoin + strings.Join(filters, canonicalFilterJoin)
	}

	return openBrace + variable + closeBrace, true
}

// isDocument reports whether s is a stored document rather than a template.
//
// It gates on "unmarshals to a JSON object" rather than json.Valid, which the server
// uses: json.Valid accepts 123, true and null, so a scalar-shaped plain-text title would
// be misread as a document. Every stored shape is an object. Requiring a "type" key
// would be too tight — it misses the legacy {root, value_markdown} envelope.
func isDocument(s string) bool {
	var object map[string]json.RawMessage
	// A JSON null unmarshals into a map without error, leaving it nil.
	return json.Unmarshal([]byte(s), &object) == nil && object != nil
}

// normalise canonicalises either form to a comparable AST, so a template and the stored
// document it corresponds to compare equal. Unwrapping first is what makes that hold for a
// dashboard-authored document, which arrives enveloped while a template only ever compiles
// to a bare node.
//
// The result never leaves this package — it feeds semantic equality and FromDocument's
// round-trip check — so dropping the envelope's schema_version here changes only which
// values compare equal, never what we send or store.
func normalise(s string) (json.RawMessage, error) {
	if !isDocument(s) {
		return ToDocument(s)
	}

	var tree any
	if err := json.Unmarshal(unwrapDocument(json.RawMessage(s)), &tree); err != nil {
		return nil, err
	}

	return json.Marshal(stripPresentation(tree))
}

// stripPresentation drops the parts of a document that carry no meaning, so a
// dashboard-authored document compares equal to the one a template emits.
//
// Load-bearing rather than defensive: after an import, or a dashboard edit post-apply,
// state holds a document carrying label and missing while config still says
// "{{description}}". Without stripping, that is a permanent spurious diff.
func stripPresentation(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if attrs, ok := typed[fieldAttrs].(map[string]any); ok && typed[fieldType] == nodeVarSpec {
			// Rewritten from the live scope on every editor load.
			delete(attrs, attrLabel)
			delete(attrs, attrMissing)

			// Defaults the dashboard writes explicitly but a template never emits.
			if omit, isBool := attrs[attrOmitIfUnset].(bool); isBool && !omit {
				delete(attrs, attrOmitIfUnset)
			}
			if attrs[attrTruncateTo] == nil {
				delete(attrs, attrTruncateTo)
			}
		}

		// Means the same as no content at all, and the two forms arrive from different
		// writers.
		if content, ok := typed[fieldContent].([]any); ok && len(content) == 0 {
			delete(typed, fieldContent)
		}

		for key, child := range typed {
			typed[key] = stripPresentation(child)
		}

		return typed

	case []any:
		for i, child := range typed {
			typed[i] = stripPresentation(child)
		}

		return typed
	}

	return value
}
