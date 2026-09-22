// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// DefaultImplicitAttributes are attribute names that are always implicitly
// present in provider docs and don't need explicit documentation entries.
var DefaultImplicitAttributes = []string{"id", "tags_all"}

// DefaultAllowPhantoms are attribute names that may appear in docs without
// a corresponding schema entry (provider-injected attributes).
var DefaultAllowPhantoms = []string{"tags", "tags_all"}

// DefaultSkipBlocks are block names that are always skipped.
var DefaultSkipBlocks = []string{"timeouts"}

// DefaultBadDescriptionPrefixes is the list of weak or redundant description starts.
var DefaultBadDescriptionPrefixes = []string{
	"A ", "An ", "The ", "This ", "It ",
	"Indicates ", "Specifies ", "Describes ", "Defines ",
	"Contains ", "Determines ", "Identifies ", "Represents ", "Denotes ", "Holds ", "Used ",
}

// SchemaDocsRule validates argument and attribute sections against the schema.
// Sub-checks (all enabled by default, disable with pointer-to-false):
//   - Coverage: every schema attr documented, every documented attr in schema
//   - Ordering: alphabetical within required/optional/unmarked groups
//   - Description: descriptions don't start with bad prefixes
//   - Heading: block headings match preferred style
//   - Format: no code blocks, single-line attrs, uninterrupted lists
//   - Labels: arguments have (Required)/(Optional), attributes do not
type SchemaDocsRule struct {
	// Coverage options
	IgnoreDeprecated   bool
	ImplicitAttributes []string
	AllowPhantoms      []string
	SkipBlocks         []string

	// AllowInlineReadOnly permits Read-Only (computed-only) attributes to
	// be documented inline in Argument Reference with a "(Read-Only)" label,
	// alongside Required and Optional siblings, instead of requiring them
	// in Attribute Reference. Default false: strict separation — Argument
	// Reference is for configurable attributes only.
	AllowInlineReadOnly *bool

	// Sub-check toggles (nil = enabled)
	Coverage    *bool
	Ordering    *bool
	Description *bool
	Heading     *bool
	Format      *bool
	Labels      *bool
	Byline      *bool
	Deprecated  *bool

	// Description options
	BadPrefixes []string

	// Heading options
	Preferred doc.HeadingTemplates

	// Format sub-options (nil = enabled)
	NoCodeBlocks              *bool
	SingleLineAttrs           *bool
	UninterruptedLists        *bool
	AllowAttributeIndentation *bool // nil/true = allow indented sub-attrs in Attribute Reference
}

func (r *SchemaDocsRule) Name() string { return "schema_docs" }

func (r *SchemaDocsRule) implicit() []string {
	if r.ImplicitAttributes != nil {
		return r.ImplicitAttributes
	}
	return DefaultImplicitAttributes
}

func (r *SchemaDocsRule) phantom() []string {
	if r.AllowPhantoms != nil {
		return r.AllowPhantoms
	}
	return DefaultAllowPhantoms
}

func (r *SchemaDocsRule) skipBlocks() []string {
	if r.SkipBlocks != nil {
		return r.SkipBlocks
	}
	return DefaultSkipBlocks
}

func (r *SchemaDocsRule) prefixes() []string {
	if r.BadPrefixes != nil {
		return r.BadPrefixes
	}
	return DefaultBadDescriptionPrefixes
}

// allowInlineReadOnly reports whether (Read-Only) labels are permitted in
// Argument Reference. Default false: strict separation between Argument
// Reference (configurable) and Attribute Reference (Read-Only).
func (r *SchemaDocsRule) allowInlineReadOnly() bool {
	return r.AllowInlineReadOnly != nil && *r.AllowInlineReadOnly
}

func enabled(b *bool) bool { return b == nil || *b }

func (r *SchemaDocsRule) Check(ctx CheckContext) []Result {
	var results []Result

	if enabled(r.Coverage) {
		results = append(results, r.checkCoverage(ctx)...)
	}
	if enabled(r.Ordering) {
		results = append(results, r.checkOrdering(ctx)...)
	}
	if enabled(r.Description) {
		results = append(results, r.checkDescriptions(ctx)...)
	}
	if enabled(r.Heading) {
		results = append(results, r.checkHeadings(ctx)...)
	}
	if enabled(r.Format) {
		results = append(results, r.checkFormat(ctx)...)
	}
	if enabled(r.Labels) {
		results = append(results, r.checkLabels(ctx)...)
	}
	if enabled(r.Byline) {
		results = append(results, r.checkBylines(ctx)...)
	}
	if enabled(r.Deprecated) {
		results = append(results, r.checkDeprecated(ctx)...)
	}

	return results
}

// --- Coverage ---

func (r *SchemaDocsRule) checkCoverage(ctx CheckContext) []Result {
	rs := ctx.Schema
	if rs == nil {
		return nil
	}

	var results []Result
	reportedMissingBlocks := make(map[string]bool)
	reportedExtraAttrs := make(map[string]bool)

	// Arguments: configurable attrs
	for blockPath, schemaBlock := range rs.Blocks {
		if slices.Contains(r.skipBlocks(), blockPath) {
			continue
		}

		docBlockName := leafName(blockPath)
		docBlocks := findAllDocBlocksIn(ctx.Doc.Blocks(), docBlockName, blockPath, nil)

		if len(docBlocks) == 0 {
			if hasConfigurableAttributes(schemaBlock) {
				if reportedMissingBlocks[docBlockName] {
					continue
				}
				reportedMissingBlocks[docBlockName] = true
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
					Message: fmt.Sprintf("block %q is not documented", displayPath(blockPath)),
					Block:   blockPath,
				})
			}
			continue
		}

		// Aggregate attributes across all matching doc blocks so that
		// coverage works whether the schema block is documented under
		// its leaf name (e.g. `### \`probabilistic\`` Block), under its
		// full path (e.g. via a dot-notation reference like
		// `rule[*].probabilistic[*].x` routed during parsing), or both.
		documented := make(map[string]bool)
		var malformed []doc.MalformedAttr
		var allDocAttrs []doc.DocAttribute
		for _, b := range docBlocks {
			for _, attr := range b.Attributes {
				documented[attr.Name] = true
			}
			malformed = append(malformed, b.MalformedAttributes...)
			allDocAttrs = append(allDocAttrs, b.Attributes...)
		}

		for _, attr := range schemaBlock.Attributes {
			if r.shouldSkipAttribute(attr) {
				continue
			}
			if !documented[attr.Name] {
				msg := fmt.Sprintf("attribute %q in block %q is not documented", attr.Name, displayPath(blockPath))
				if m, ok := findMalformed(malformed, attr.Name); ok {
					msg = fmt.Sprintf("attribute %q in block %q is documented but missing the \" - \" separator (expected: * `%s` - (Required|Optional) ...)", attr.Name, displayPath(blockPath), attr.Name)
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: severity(attr), Message: msg, Block: blockPath, Line: m.Line,
					})
				} else {
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: severity(attr), Message: msg, Block: blockPath,
					})
				}
			} else if m, ok := findMalformed(malformed, attr.Name); ok {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("attribute %q in block %q is documented but missing the \" - \" separator (expected: * `%s` - (Required|Optional) ...)", attr.Name, displayPath(blockPath), attr.Name),
					Block:   blockPath, Line: m.Line,
				})
			}
		}

		// Doc → schema: documented attributes should exist in schema.
		if len(schemaBlock.Attributes) == 0 && len(schemaBlock.ChildBlocks) == 0 {
			continue
		}
		schemaAttrNames := make(map[string]bool, len(schemaBlock.Attributes))
		for _, attr := range schemaBlock.Attributes {
			schemaAttrNames[attr.Name] = true
		}
		for _, child := range schemaBlock.ChildBlocks {
			schemaAttrNames[leafName(child)] = true
		}

		for _, docAttr := range allDocAttrs {
			if !schemaAttrNames[docAttr.Name] && !slices.Contains(r.phantom(), docAttr.Name) {
				if existsInSiblingBlock(rs, leafName(blockPath), docAttr.Name) {
					continue
				}
				key := docBlockName + "." + docAttr.Name
				if reportedExtraAttrs[key] {
					continue
				}
				reportedExtraAttrs[key] = true
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("documented attribute %q in block %q does not exist in schema", docAttr.Name, displayPath(blockPath)),
					Block:   blockPath,
					Line:    docAttr.Line,
				})
			}
		}

		// Computed-only misplacement in arguments. Suppressed when
		// AllowInlineReadOnly is true: under the permissive convention,
		// inline (Read-Only) labels in Argument Reference are
		// intentional, not misplaced.
		if blockPath == "" && !r.allowInlineReadOnly() {
			if argBlock := ctx.Doc.ArgumentBlocks[""]; argBlock != nil {
				results = append(results, r.checkComputedMisplacement(ctx, schemaBlock, argBlock, ctx.Doc.AttributeBlocks[""])...)
			}
		}
	}

	// Attributes: Read-Only (computed-only) coverage across all schema blocks.
	results = append(results, r.checkAttributeCoverage(ctx)...)

	results = append(results, r.checkPhantomBlocks(ctx)...)

	return results
}

// checkPhantomBlocks reports doc blocks in the Argument Reference section
// whose name has no counterpart in the schema. This catches stray
// subheadings that get parsed as block names — e.g. a `### \`rules\“
// heading followed by a `#### Arguments` subheading creates a phantom
// "arguments" block in the doc model. The schema has no such block; the
// H4 should not be there.
//
// Limited to ArgumentBlocks because block headings in the Attribute
// Reference section commonly document the nested structure of computed
// attributes (e.g. `### Endpoint`, `### master_user_secret`) that have
// no Block representation in the schema. Flagging those would produce
// false positives on standard provider doc patterns.
//
// A doc block name matches the schema if any schema block path has the
// same leaf name. (Matching by leaf alone is consistent with how the
// rest of the rule resolves doc blocks against the schema, including
// findDocBlock and existsInSiblingBlock.)
func (r *SchemaDocsRule) checkPhantomBlocks(ctx CheckContext) []Result {
	if ctx.Schema == nil {
		return nil
	}

	schemaLeaves := make(map[string]bool, len(ctx.Schema.Blocks))
	for path := range ctx.Schema.Blocks {
		if path == "" {
			continue
		}
		schemaLeaves[leafName(path)] = true
	}
	// Object-typed nested attributes (list/set/single object attributes,
	// e.g. list(object({...}))) are documented with block-style headings
	// (### `application_settings`) even though they are attributes, not
	// blocks. They live as Attribute.Children in the schema model rather
	// than as rs.Blocks entries, so add their leaf names too — otherwise
	// a correctly-documented nested attribute is flagged as a phantom
	// block.
	for leaf := range nestedAttributeLeaves(ctx.Schema) {
		schemaLeaves[leaf] = true
	}

	reported := make(map[string]bool)
	var results []Result
	for blockName, block := range ctx.Doc.ArgumentBlocks {
		if blockName == "" || block.Heading == "" {
			continue
		}
		leaf := leafName(blockName)
		if schemaLeaves[leaf] {
			continue
		}
		// Dedupe by heading text — combined headings ("X and Y") create
		// multiple doc blocks but should produce one finding per heading.
		if reported[block.Heading] {
			continue
		}
		reported[block.Heading] = true
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: ctx.Resource,
			Severity: SeverityError,
			Message:  fmt.Sprintf("block heading %q in Argument Reference has no matching block in schema", block.Heading),
			Block:    blockName,
		})
	}
	return results
}

// nestedAttributeLeaves collects the leaf names of object-typed nested
// attributes (those with children) at every depth of the schema. Such
// attributes — encoded as list(object({...})), set(object({...})), or a
// bare object, whether SDK cty types or Framework nested types — are
// conventionally documented with block-style headings and a list of
// their sub-attributes, exactly like a nested block. They are stored as
// Attribute.Children rather than as rs.Blocks entries, so callers that
// only consult rs.Blocks would otherwise treat these legitimate
// headings as phantom blocks.
func nestedAttributeLeaves(rs *schema.ResourceSchema) map[string]bool {
	leaves := make(map[string]bool)
	var walk func(attrs []schema.Attribute)
	walk = func(attrs []schema.Attribute) {
		for _, a := range attrs {
			if len(a.Children) == 0 {
				continue
			}
			leaves[a.Name] = true
			walk(a.Children)
		}
	}
	for _, b := range rs.Blocks {
		if b == nil {
			continue
		}
		walk(b.Attributes)
	}
	return leaves
}

// checkAttributeCoverage ensures every Read-Only (computed-only) schema
// attribute, at every depth of nesting, is documented somewhere reachable.
//
// The default expectation is that Read-Only attributes appear in
// ## Attribute Reference (under the appropriate block heading for nested
// ones). When AllowInlineReadOnly is true, the rule additionally accepts
// Read-Only attributes documented inline in ## Argument Reference with a
// (Read-Only) label, alongside Required and Optional siblings — the
// taxonomy used by tfplugindocs.
//
// This is the per-attribute presence rule. Misplacement (Read-Only in
// Argument Reference when the toggle is off) is handled separately by
// checkComputedMisplacement.
func (r *SchemaDocsRule) checkAttributeCoverage(ctx CheckContext) []Result {
	if ctx.Schema == nil {
		return nil
	}

	allowInline := r.allowInlineReadOnly()
	var results []Result
	reported := make(map[string]bool)

	for blockPath, schemaBlock := range ctx.Schema.Blocks {
		if slices.Contains(r.skipBlocks(), blockPath) {
			continue
		}
		if schemaBlock == nil {
			continue
		}

		argDocs := findAllDocBlocksIn(ctx.Doc.ArgumentBlocks, leafName(blockPath), blockPath, ctx.Doc.BlockAnchors)
		attrDocs := findAllDocBlocksIn(ctx.Doc.AttributeBlocks, leafName(blockPath), blockPath, ctx.Doc.BlockAnchors)

		for _, attr := range schemaBlock.Attributes {
			// Only Read-Only (computed-only) attributes are covered here.
			// Required / Optional coverage is handled in checkCoverage.
			if !(attr.Computed && !attr.Optional && !attr.Required) {
				continue
			}
			if slices.Contains(r.implicit(), attr.Name) {
				continue
			}
			if r.IgnoreDeprecated && attr.Deprecated {
				continue
			}

			inAttrs := anyDocBlockHasAttr(attrDocs, attr.Name)
			inArgs := anyDocBlockHasAttr(argDocs, attr.Name)

			// Documented in Attribute Reference always satisfies the rule.
			// Documented inline in Argument Reference satisfies the rule
			// only when AllowInlineReadOnly is true.
			//
			// For ConfigUnknown blocks — object-typed attributes whose
			// configurable parent leaves per-field Required/Optional/Computed
			// unknowable — documentation in either section satisfies the rule,
			// since the schema gives us no basis to demand a specific one.
			if schemaBlock.ConfigUnknown {
				if inAttrs || inArgs {
					continue
				}
			} else if inAttrs || (allowInline && inArgs) {
				continue
			}

			key := blockPath + "." + attr.Name
			if reported[key] {
				continue
			}
			reported[key] = true

			var msg string
			switch {
			case schemaBlock.ConfigUnknown:
				// Section can't be dictated; report neutrally as undocumented.
				msg = fmt.Sprintf("attribute %q in block %q is not documented", attr.Name, displayPath(blockPath))
			case blockPath == "":
				msg = fmt.Sprintf("Read-Only attribute %q should be documented in Attribute Reference section", attr.Name)
			default:
				msg = fmt.Sprintf("Read-Only attribute %q in block %q should be documented in Attribute Reference section", attr.Name, displayPath(blockPath))
			}
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: SeverityError,
				Message:  msg,
				Block:    blockPath,
			})
		}
	}
	return results
}

func docBlockHasAttr(b *doc.DocBlock, name string) bool {
	if b == nil {
		return false
	}
	for _, a := range b.Attributes {
		if a.Name == name {
			return true
		}
	}
	return false
}

func anyDocBlockHasAttr(blocks []*doc.DocBlock, name string) bool {
	for _, b := range blocks {
		if docBlockHasAttr(b, name) {
			return true
		}
	}
	return false
}

func (r *SchemaDocsRule) checkComputedMisplacement(ctx CheckContext, schemaBlock *schema.Block, argBlock *doc.DocBlock, attrBlock *doc.DocBlock) []Result {
	var results []Result
	documented := make(map[string]bool, len(argBlock.Attributes))
	docLines := make(map[string]int, len(argBlock.Attributes))
	for _, attr := range argBlock.Attributes {
		documented[attr.Name] = true
		docLines[attr.Name] = attr.Line
	}

	// Build set of attrs in the attribute section to avoid false positives
	// when broad heading templates cause attribute-section items to also
	// appear in ArgumentBlocks.
	inAttrSection := make(map[string]bool)
	if attrBlock != nil {
		for _, attr := range attrBlock.Attributes {
			inAttrSection[attr.Name] = true
		}
	}

	for _, attr := range schemaBlock.Attributes {
		if attr.Computed && !attr.Optional && !attr.Required {
			if slices.Contains(r.implicit(), attr.Name) {
				continue
			}
			if documented[attr.Name] && !inAttrSection[attr.Name] {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("computed-only attribute %q should not appear in Argument Reference section", attr.Name),
					Line:    docLines[attr.Name],
				})
			}
		}
	}
	return results
}

func (r *SchemaDocsRule) shouldSkipAttribute(attr schema.Attribute) bool {
	if slices.Contains(r.implicit(), attr.Name) {
		return true
	}
	if r.IgnoreDeprecated && attr.Deprecated {
		return true
	}
	if attr.Computed && !attr.Optional && !attr.Required {
		return true
	}
	return false
}

// --- Ordering ---

func (r *SchemaDocsRule) checkOrdering(ctx CheckContext) []Result {
	var results []Result
	for blockPath, block := range ctx.Doc.ArgumentBlocks {
		results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "argument", blockPath, block)...)
	}
	for blockPath, block := range ctx.Doc.AttributeBlocks {
		results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "attribute", blockPath, block)...)
	}
	return results
}

// --- Description style ---

func (r *SchemaDocsRule) checkDescriptions(ctx CheckContext) []Result {
	seen := make(map[string]bool)
	var results []Result
	results = append(results, checkDescriptionBlocks(ctx.Resource, r.Name(), r.prefixes(), ctx.Doc.ArgumentBlocks, seen)...)
	results = append(results, checkDescriptionBlocks(ctx.Resource, r.Name(), r.prefixes(), ctx.Doc.AttributeBlocks, seen)...)
	return results
}

func checkDescriptionBlocks(resource, ruleName string, prefixes []string, blocks map[string]*doc.DocBlock, seen map[string]bool) []Result {
	var results []Result
	for blockName, block := range blocks {
		for _, attr := range block.Attributes {
			if attr.Description == "" {
				continue
			}
			key := blockName + "." + attr.Name
			if seen[key] {
				continue
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(attr.Description, prefix) {
					seen[key] = true
					results = append(results, Result{
						Rule: ruleName, Resource: resource, Severity: SeverityError,
						Message: fmt.Sprintf("attribute %q description should not start with %q (block %q)", attr.Name, strings.TrimSpace(prefix), displayPath(blockName)),
						Block:   blockName,
						Line:    attr.Line,
					})
					break
				}
			}
		}
	}
	return results
}

// --- Heading style ---

func (r *SchemaDocsRule) checkHeadings(ctx CheckContext) []Result {
	rs := ctx.Schema
	if rs == nil || len(r.Preferred) == 0 {
		return nil
	}

	schemaLeaves := make(map[string]bool)
	ambiguousLeaves := make(map[string]bool)
	leafAttrs := make(map[string]string)
	for path, block := range rs.Blocks {
		leaf := leafName(path)
		schemaLeaves[leaf] = true
		sig := blockSignature(block)
		if prev, exists := leafAttrs[leaf]; exists && prev != sig {
			ambiguousLeaves[leaf] = true
		}
		leafAttrs[leaf] = sig
	}

	var results []Result
	for _, blocks := range []map[string]*doc.DocBlock{ctx.Doc.ArgumentBlocks, ctx.Doc.AttributeBlocks} {
		for _, block := range blocks {
			if block.Name == "" || block.Heading == "" {
				continue
			}
			// block.Name may be a leaf ("match") or a full dot-path
			// ("spec.grpc_route.match"). Look up by leaf so path-keyed
			// blocks still participate in the preferred-style check.
			blockLeaf := leafName(block.Name)
			if !schemaLeaves[blockLeaf] {
				continue
			}

			// Ambiguity check is orthogonal to preferred-style check: a
			// heading can be in a perfectly preferred style and still
			// collide with multiple schema blocks sharing its leaf. The
			// only safe disambiguator is a doc key that resolves to
			// exactly one schema block under findAllDocBlocksIn.
			//
			// Skipping by "block.Name contains a dot" is too coarse —
			// {Parent}-template composites and other partial-path keys
			// can still match multiple schema paths via the existing
			// 2- and 3-segment composite-suffix lookups. Instead,
			// compute the actual set of schema paths this doc key
			// would resolve to. If it's 1, the heading uniquely
			// identifies a schema block. If it's ≥2, the heading is
			// still ambiguous and we warn with the precise collision
			// list.
			if ambiguousLeaves[blockLeaf] {
				resolved := schemaPathsResolvedByDocKey(rs, blocks, block.Name)
				if len(resolved) > 1 {
					pathTemplate := ""
					for _, tmpl := range r.Preferred {
						if strings.Contains(tmpl, "{Path}") {
							pathTemplate = tmpl
							break
						}
					}
					if pathTemplate == "" {
						pathTemplate = "`{Path}` Block"
					}
					sort.Strings(resolved)
					example := doc.RenderHeading(pathTemplate, resolved[0])
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
						Message: fmt.Sprintf(
							"block %q heading %q is ambiguous (resolves to %d schema blocks: %s); use the full dot-path form, e.g. %q",
							block.Name, block.Heading, len(resolved), strings.Join(resolved, ", "), example,
						),
						Block: block.Name,
					})
				}
			}

			// Preferred-style check: does the heading match one of the
			// preferred templates? Independent of ambiguity.
			if r.Preferred.Match(block.Heading) != "" {
				continue
			}

			// Suggestion strategy depends on whether the block is
			// already path-keyed:
			//
			//  - path-keyed (block.Name contains a dot): pick the first
			//    {Path} template so the suggestion preserves the full
			//    dot-path disambiguator. Falling back to a leaf-only
			//    template here would reintroduce the ambiguity this
			//    rule is designed to catch.
			//
			//  - leaf-keyed: pick the first non-{Parent}, non-{Path}
			//    template since a single segment can't fill {Path} in
			//    a useful way and {Parent} requires words we don't
			//    have.
			var suggested string
			if strings.Contains(block.Name, ".") {
				for _, tmpl := range r.Preferred {
					if strings.Contains(tmpl, "{Path}") {
						suggested = doc.RenderHeading(tmpl, block.Name)
						break
					}
				}
				if suggested == "" {
					suggested = doc.RenderHeading("`{Path}` Block", block.Name)
				}
			} else {
				for _, tmpl := range r.Preferred {
					if !strings.Contains(tmpl, "{Parent}") && !strings.Contains(tmpl, "{Path}") {
						suggested = doc.RenderHeading(tmpl, blockLeaf)
						break
					}
				}
				if suggested == "" {
					suggested = doc.RenderHeading("`{Block}` Block", blockLeaf)
				}
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("block %q heading %q should be %q", block.Name, block.Heading, suggested),
				Block:   block.Name,
			})
		}
	}
	return results
}

// blockSignature returns an order-independent signature of a block's
// immediate attribute names, used to decide whether two schema blocks
// sharing a leaf name are structurally distinct. The names are sorted
// so that blocks with the same attribute set but different schema
// declaration order compare equal — otherwise identical blocks would be
// falsely flagged as an ambiguous leaf.
func blockSignature(block *schema.Block) string {
	names := make([]string, 0, len(block.Attributes))
	for _, a := range block.Attributes {
		names = append(names, a.Name)
	}
	slices.Sort(names)
	return strings.Join(names, ",")
}

// schemaPathsResolvedByDocKey returns every schema path P for which the
// doc heading keyed by docKey is P's MOST-SPECIFIC matching heading,
// given the full set of doc headings in docBlocks. It is the right
// notion of "does this doc key disambiguate to a single schema block?"
// — a 1-element result means yes, ≥2 means the key genuinely covers
// multiple schema blocks and is ambiguous.
//
// The most-specific qualifier is essential. findAllDocBlocksIn matches a
// schema path not only against an exact-path doc key but also against
// non-contiguous composites (e.g. parts[0].parts[1].leaf) and the bare
// leaf. Evaluating docKey in isolation therefore over-counts: a parent
// heading like "spec.http2_route.match" would appear to also "resolve" a
// descendant like "spec.http2_route.match.header.match" via the
// composite matcher, even when that descendant has its own exact
// full-path heading that should own it. By consulting the real
// docBlocks and taking findAllDocBlocksIn's most-specific match
// (returned first), a path counts toward docKey only when no more
// specific heading claims it — eliminating the phantom, self-suggesting
// ambiguity warnings.
func schemaPathsResolvedByDocKey(rs *schema.ResourceSchema, docBlocks map[string]*doc.DocBlock, docKey string) []string {
	if rs == nil || docKey == "" {
		return nil
	}
	docLeaf := leafName(docKey)
	var matches []string
	for path := range rs.Blocks {
		if path == "" {
			continue
		}
		if leafName(path) != docLeaf {
			continue
		}
		// The heading that owns this schema path is its most-specific
		// match, which findAllDocBlocksIn returns first. Count the path
		// toward docKey only when docKey is that owner.
		if best := findAllDocBlocksIn(docBlocks, leafName(path), path, nil); len(best) > 0 && best[0].Name == docKey {
			matches = append(matches, path)
		}
	}
	return matches
}

// --- Format (raw-line checks) ---

func (r *SchemaDocsRule) checkFormat(ctx CheckContext) []Result {
	source := ctx.Doc.Source()
	if len(source) == 0 {
		return nil
	}

	noCode := enabled(r.NoCodeBlocks)
	singleLine := enabled(r.SingleLineAttrs)
	uninterrupted := enabled(r.UninterruptedLists)

	var results []Result
	var inSection bool
	var inAttributes bool
	var inCodeBlock bool
	var inList bool
	var prevWasAttr bool
	// attrStack tracks attribute names at each indentation level for nesting validation.
	// Index 0 = top-level (0 spaces), 1 = first indent (4 spaces), etc.
	var attrStack []string
	scanner := bufio.NewScanner(bytes.NewReader(source))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "## Argument Reference") || strings.HasPrefix(line, "## Attribute Reference") {
			inSection = true
			inAttributes = strings.HasPrefix(line, "## Attribute")
			inList = false
			prevWasAttr = false
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") {
			inSection = false
			inCodeBlock = false
			inList = false
		}
		if !inSection {
			continue
		}

		if strings.HasPrefix(line, "```") {
			if noCode && !inCodeBlock {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
					Message: fmt.Sprintf("code block in argument/attribute section (line %d)", lineNum),
				})
			}
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		isAttrLine := strings.HasPrefix(line, "* `")
		isHeading := strings.HasPrefix(line, "#")
		isBlank := line == ""

		if singleLine && prevWasAttr && !isAttrLine && !isHeading && !isBlank && strings.HasPrefix(line, "  ") {
			trimmed := strings.TrimLeft(line, " ")
			isIndentedAttr := strings.HasPrefix(trimmed, "* `")
			if isIndentedAttr {
				if inAttributes && enabled(r.AllowAttributeIndentation) {
					indent := len(line) - len(trimmed)
					level := indent / 4 // 4 spaces per level
					name := extractAttrName(trimmed)
					// Update stack for this level.
					if level < len(attrStack) {
						attrStack = attrStack[:level]
					}
					attrStack = append(attrStack, name)
					// Validate against schema if available.
					if ctx.Schema != nil && level >= 1 {
						results = append(results, r.validateIndentedAttr(ctx, attrStack, lineNum)...)
					}
					continue
				}
				section := "Argument Reference"
				if inAttributes {
					section = "Attribute Reference"
				}
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("indented sub-attribute in %s (line %d); use a subsection heading instead", section, lineNum),
				})
			} else {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("multi-line attribute description (line %d); each attribute should be on one line", lineNum),
				})
			}
		}

		if uninterrupted && inList && !isAttrLine && !isHeading && !isBlank && !strings.HasPrefix(line, "  ") && !isListProse(line) {
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("attribute list interrupted (line %d): %q", lineNum, truncate(line, 60)),
			})
			inList = false
		}

		if isAttrLine {
			inList = true
			name := extractAttrName(line)
			attrStack = []string{name}
		}
		if isHeading {
			inList = false
			attrStack = nil
		}
		prevWasAttr = isAttrLine || (prevWasAttr && strings.HasPrefix(line, "  ") && strings.HasPrefix(strings.TrimLeft(line, " "), "* `"))
	}

	return results
}

// extractAttrName pulls the backticked name from a list item like "* `name` - ...".
func extractAttrName(line string) string {
	after, ok := strings.CutPrefix(line, "* `")
	if !ok {
		return ""
	}
	if i := strings.IndexByte(after, '`'); i > 0 {
		return after[:i]
	}
	return ""
}

// validateIndentedAttr checks that an indented sub-attribute exists in the schema
// at the correct nesting level. attrStack contains the attribute name chain from
// root to the current indented item.
func (r *SchemaDocsRule) validateIndentedAttr(ctx CheckContext, attrStack []string, lineNum int) []Result {
	if len(attrStack) < 2 {
		return nil
	}

	// Walk the schema attribute tree following the stack.
	// attrStack[0] is the root attribute, attrStack[1] is its child, etc.
	rootBlock := ctx.Schema.Blocks[""]
	if rootBlock == nil {
		return nil
	}

	// Find the root attribute.
	var children []schema.Attribute
	for _, a := range rootBlock.Attributes {
		if a.Name == attrStack[0] {
			children = a.Children
			break
		}
	}

	// Walk intermediate levels.
	for i := 1; i < len(attrStack)-1; i++ {
		found := false
		for _, a := range children {
			if a.Name == attrStack[i] {
				children = a.Children
				found = true
				break
			}
		}
		if !found {
			return nil // can't validate deeper if intermediate is unknown
		}
	}

	// Check the leaf (last element in stack).
	leaf := attrStack[len(attrStack)-1]

	if len(children) == 0 {
		// Parent has no known children in schema — the indentation is invalid.
		return []Result{{
			Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
			Message: fmt.Sprintf("indented attribute %q (line %d) under %q but schema has no nested attributes there", leaf, lineNum, attrStack[len(attrStack)-2]),
		}}
	}

	for _, a := range children {
		if a.Name == leaf {
			return nil // valid
		}
	}

	return []Result{{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
		Message: fmt.Sprintf("indented attribute %q (line %d) not found in schema under %q", leaf, lineNum, attrStack[len(attrStack)-2]),
	}}
}

// --- Labels ---

func (r *SchemaDocsRule) checkLabels(ctx CheckContext) []Result {
	var results []Result

	// Build set of attrs in attribute section to avoid false positives from
	// template bleed (broad heading templates can cause attribute-section
	// items to also appear in ArgumentBlocks).
	attrSectionNames := make(map[string]map[string]bool)
	for blockName, block := range ctx.Doc.AttributeBlocks {
		names := make(map[string]bool, len(block.Attributes))
		for _, attr := range block.Attributes {
			names[attr.Name] = true
		}
		attrSectionNames[blockName] = names
	}

	// Arguments must have (Required), (Optional), or (Read-Only)
	allowReadOnly := r.allowInlineReadOnly()
	for blockName, block := range ctx.Doc.ArgumentBlocks {
		for _, attr := range block.Attributes {
			if attr.Required || attr.Optional {
				continue
			}
			if attr.ReadOnly && allowReadOnly {
				// Inline Read-Only is permitted by config; the
				// label is present, so no labels-rule complaint.
				continue
			}
			// Skip if this attr is also in the attribute section (template bleed)
			if ns, ok := attrSectionNames[blockName]; ok && ns[attr.Name] {
				continue
			}
			label := "(Required) or (Optional)"
			if allowReadOnly {
				label = "(Required), (Optional), or (Read-Only)"
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("argument %q in block %q is missing %s label", attr.Name, displayPath(blockName), label),
				Block:   blockName,
				Line:    attr.Line,
			})
		}
	}

	// Attribute Reference: flag labeled configurable arguments as misplaced
	// (move them to Argument Reference) instead of pushing the author to strip
	// accurate labels. Detection is per attribute; the message collapses to a
	// single subsection move when an entire block is configurable. See
	// docs/rules/argument-attribute-misplacement.md.
	results = append(results, r.attributeMisplacementFindings(ctx)...)

	return results
}

// stripLabelResult builds the "attribute should not have (Required)/(Optional)
// label" warning for a labeled attribute documented under Attribute Reference.
func stripLabelResult(r *SchemaDocsRule, ctx CheckContext, blockName string, attr doc.DocAttribute) Result {
	label := "(Optional)"
	if attr.Required {
		label = "(Required)"
	}
	return Result{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
		Message: fmt.Sprintf("attribute %q in block %q should not have %s label", attr.Name, displayPath(blockName), label),
		Block:   blockName,
		Line:    attr.Line,
	}
}

// --- Shared helpers ---

// attributeMisplacementFindings implements the attribute-granular
// Argument/Attribute-Reference misplacement rule in a single pass over
// AttributeBlocks (docs/rules/argument-attribute-misplacement.md §3–§5):
// resolve each subsection to a schema path, classify every labeled attribute,
// then choose the message granularity per subsection and deduplicate.
//
// A configurable argument (Required/Optional and not Computed) documented under
// Attribute Reference is *misplaced* — the author should move it to Argument
// Reference, not strip its (correct) label. When an entire subsection is
// configurable (documents no computed-only field) the moves collapse into one
// "move this subsection" finding; a mixed subsection emits per-attribute moves
// and leaves its computed-only fields where they are.
func (r *SchemaDocsRule) attributeMisplacementFindings(ctx CheckContext) []Result {
	skip := r.skipBlocks()
	names := slices.Sorted(maps.Keys(ctx.Doc.AttributeBlocks))

	type subMeta struct {
		path       string
		resolved   bool
		heading    string
		line       int
		preHeading bool // entry spans pre-heading dot-path references; never collapse
		hasMis     bool // documents at least one misplaced (labeled pure-config) attribute
		hasComp    bool // documents a field that must stay under Attribute Reference (blocks collapse)
		eligible   bool // collapse-eligible subsection
	}
	subs := make(map[string]*subMeta, len(names))
	// pathHasMis[P] is true when a real-heading subsection resolving to P
	// documents at least one misplaced attribute (i.e. emits a move). Used to
	// dedup a child-block reference bullet only against a child subsection that
	// actually produces a finding.
	pathHasMis := make(map[string]bool)

	type misplacedAttr struct {
		block  string
		path   string
		attr   doc.DocAttribute
		target string // child block path if the attr is a child-block reference; else path
	}
	var misplaced []misplacedAttr
	type stripEntry struct {
		block string
		attr  doc.DocAttribute
	}
	var strips []stripEntry

	// Collect: resolve each subsection and classify its attributes.
	for _, name := range names {
		block := ctx.Doc.AttributeBlocks[name]
		path, _, ok := resolveSubsectionPath(ctx.Schema, ctx.Doc.AttributeBlocks, name)
		// skip_blocks: a block opted out of checks (applied to its resolved
		// schema path) must not produce a move. Treating it as unresolved keeps
		// its legacy strip-label guidance without emitting the new error.
		resolved := ok && !slices.Contains(skip, path)

		sm := &subMeta{path: path, resolved: resolved, heading: block.Heading, line: block.HeadingLine, preHeading: block.PreHeadingAttrs}
		subs[name] = sm

		for _, attr := range block.Attributes {
			switch classifyAttrPlacement(ctx.Schema, path, resolved, attr) {
			case placementOK:
				// Unlabeled computed output — correctly placed.
			case placementStripLabel:
				strips = append(strips, stripEntry{name, attr})
			case placementMisplaced:
				target := path
				if b, okb := ctx.Schema.Blocks[path]; okb {
					if cp, isChild := childPathForAttr(path, b, attr.Name); isChild {
						target = cp
					}
				}
				misplaced = append(misplaced, misplacedAttr{name, path, attr, target})
				sm.hasMis = true
				if sm.line == 0 {
					sm.line = attr.Line
				}
			}
			if resolved && fieldRequiresAttributeReference(ctx.Schema, path, attr.Name) {
				sm.hasComp = true
			}
		}
		if resolved && block.Heading != "" && sm.hasMis {
			pathHasMis[path] = true
		}
	}

	// Collapse eligibility is per physical subsection (§5): a resolved,
	// real-heading, non-root subsection that documents at least one misplaced
	// attribute and nothing that must stay under Attribute Reference. Judged per
	// subsection — not per schema path — so a subsection is never silenced by a
	// *different* subsection that happens to resolve to the same path.
	for _, name := range names {
		sm := subs[name]
		sm.eligible = sm.resolved && sm.heading != "" && sm.path != "" && sm.hasMis && !sm.hasComp && !sm.preHeading
	}

	// A collapse "move this subsection" covers exactly the fields documented in
	// that subsection. Mark those fields covered so an alternate subsection does
	// not re-report the same field — while its *distinct* fields still surface.
	covered := make(map[string]bool)
	for _, m := range misplaced {
		if subs[m.block].eligible {
			covered[m.target+"\x00"+m.attr.Name] = true
		}
	}

	var out []Result

	// Collapse findings: one per physical heading. Parser aliases share a
	// heading line, so dedup on it to avoid re-reporting one subsection under
	// each alias key; genuinely distinct headings have distinct lines.
	emittedCollapse := make(map[string]bool)
	for _, name := range names {
		sm := subs[name]
		if !sm.eligible {
			continue
		}
		phys := "name:" + name
		if sm.line > 0 {
			phys = fmt.Sprintf("line:%d", sm.line)
		}
		if emittedCollapse[phys] {
			continue
		}
		emittedCollapse[phys] = true
		out = append(out, Result{
			Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
			Message: fmt.Sprintf("block %q is documented under Attribute Reference but is a configurable argument block in the schema; move this subsection to Argument Reference", displayPath(name)),
			Block:   name,
			Line:    sm.line,
		})
	}

	// Per-attribute moves for misplaced attributes not covered by a collapse. A
	// child-block reference bullet is deduped only against a child subsection
	// that actually emits a move (pathHasMis), never against the mere existence
	// of a child heading. Severity is WARN only for a genuine root scalar (path
	// and target both root, #62); a child-block reference — even one documented
	// at the root — is a nested-block move at ERROR.
	seen := make(map[string]bool)
	for _, m := range misplaced {
		if subs[m.block].eligible {
			continue // covered by this subsection's own collapse
		}
		fk := m.target + "\x00" + m.attr.Name
		if covered[fk] || seen[fk] {
			continue
		}
		if m.target != m.path && pathHasMis[m.target] {
			continue // the child subsection carries the finding
		}
		seen[fk] = true

		sev := SeverityError
		var msg string
		if m.path == "" {
			if m.target == "" {
				sev = SeverityWarning // genuine root scalar (#62)
			}
			msg = fmt.Sprintf("argument %q is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference", m.attr.Name)
		} else {
			msg = fmt.Sprintf("argument %q in block %q is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference", m.attr.Name, displayPath(m.block))
		}
		out = append(out, Result{Rule: r.Name(), Resource: ctx.Resource, Severity: sev, Message: msg, Block: m.block, Line: m.attr.Line})
	}

	// Strip-label findings: labeled attributes that are not configurable
	// arguments (computed-only, Optional+Computed, ConfigUnknown, or unresolved)
	// keep the legacy "should not have label" guidance.
	for _, s := range strips {
		out = append(out, stripLabelResult(r, ctx, s.block, s.attr))
	}

	return out
}

// fieldRequiresAttributeReference reports whether a documented attribute must
// remain under Attribute Reference, which prevents its subsection from
// collapsing into a wholesale "move this subsection" finding (§5). That holds
// for a computed-only scalar (Computed and neither Required nor Optional), and
// for a documented child block that is entirely read-only (no configurable
// field anywhere in its subtree) or whose per-field configurability is
// unknowable (ConfigUnknown, handled conservatively so read-only child docs are
// never dragged into Argument Reference).
func fieldRequiresAttributeReference(rs *schema.ResourceSchema, path, attrName string) bool {
	if rs == nil {
		return false
	}
	b, ok := rs.Blocks[path]
	if !ok {
		return false
	}
	for _, a := range b.Attributes {
		if a.Name == attrName {
			return a.Computed && !a.Required && !a.Optional
		}
	}
	if cp, isChild := childPathForAttr(path, b, attrName); isChild {
		if cb, okc := rs.Blocks[cp]; okc && cb.ConfigUnknown {
			return true
		}
		return !blockTreeHasPureConfigurable(rs, cp, make(map[string]bool))
	}
	return false
}

func hasConfigurableAttributes(block *schema.Block) bool {
	for _, attr := range block.Attributes {
		if attr.Required || attr.Optional {
			return true
		}
	}
	return false
}

// resolutionClass records how an Attribute-Reference subsection heading resolved
// to a schema path. It is threaded into the emitted finding so severity can be
// assigned by false-positive risk rather than parsed back out of the message
// (see docs/rules/argument-attribute-misplacement.md §9, §10 step 2b).
type resolutionClass int

const (
	// resolveUnresolved: no schema path could be assigned; the caller falls
	// back to the legacy strip-label behavior (never a move).
	resolveUnresolved resolutionClass = iota
	// resolveRoot: the root block (top-level scalars, #62).
	resolveRoot
	// resolveDottedExact: a dotted heading matched an exact schema block.
	resolveDottedExact
	// resolveBareExact: a bare heading matched an exact (root-level) schema block.
	resolveBareExact
	// resolveUniqueLeaf: a bare heading was inferred to the sole schema path
	// carrying that leaf. This is the ONLY inference step (see §4).
	resolveUniqueLeaf
)

// resolveSubsectionPath maps an Attribute-Reference subsection's doc-block key to
// a single schema path for misplacement classification, using the strict,
// ownership-free rules in docs/rules/argument-attribute-misplacement.md §4:
//
//   - root (""):   resolves to "" (resolveRoot).
//   - dotted key:  the exact schema block, else unresolved. A dotted heading
//     claims an exact path and is never remapped by leaf (fixes 2/3).
//   - bare key:    the exact root-level block (resolveBareExact); else the sole
//     schema path carrying that leaf (resolveUniqueLeaf); else unresolved
//     (fixes 1/3).
//
// Unlike coverage's ownership resolver (schemaPathsResolvedByDocKey), this never
// uses most-specific-owner logic: ownership exists to avoid double-counting
// coverage and is the wrong tool for "does this documented argument correspond
// to a configurable schema field." The unique-leaf branch is the only step that
// infers a path; it fires for real headings only (a heading-less synthetic
// block is resolved by exact path alone, never inferred) and is double-gated
// downstream by configurableArgAtPath, so it can never fabricate a move on a
// non-configurable field.
func resolveSubsectionPath(rs *schema.ResourceSchema, docBlocks map[string]*doc.DocBlock, key string) (string, resolutionClass, bool) {
	if rs == nil {
		return "", resolveUnresolved, false
	}
	if key == "" {
		return "", resolveRoot, true
	}
	if strings.Contains(key, ".") {
		if _, ok := rs.Blocks[key]; ok {
			return key, resolveDottedExact, true
		}
		return "", resolveUnresolved, false
	}
	// Bare key: exact root-level block first.
	if _, ok := rs.Blocks[key]; ok {
		return key, resolveBareExact, true
	}
	// Unique-leaf inference, gated to real headings. A heading-less synthetic
	// block (dot-path reference bullet or prose lead-in) must never be inferred
	// by leaf; it resolves only by the exact-path branch above.
	if b := docBlocks[key]; b != nil && b.Heading != "" {
		if p, ok := uniqueSchemaPathForLeaf(rs, key); ok {
			return p, resolveUniqueLeaf, true
		}
	}
	return "", resolveUnresolved, false
}

// uniqueSchemaPathForLeaf returns the single non-root schema path whose leaf
// name equals leaf, reporting false when zero or more than one path carries it.
// It underpins the alternate-heading suppression: an alternate subsection can be
// associated with a moved schema path only when the leaf is unambiguous.
func uniqueSchemaPathForLeaf(rs *schema.ResourceSchema, leaf string) (string, bool) {
	if rs == nil || leaf == "" {
		return "", false
	}
	var found string
	n := 0
	for path := range rs.Blocks {
		if path != "" && leafName(path) == leaf {
			found = path
			n++
		}
	}
	if n == 1 {
		return found, true
	}
	return "", false
}

// placement is the per-attribute verdict for an attribute documented under
// Attribute Reference (docs/rules/argument-attribute-misplacement.md §3).
type placement int

const (
	// placementOK: no finding. The attribute carries no (Required)/(Optional)
	// label, so it is a proper computed output living under Attribute Reference.
	placementOK placement = iota
	// placementStripLabel: the attribute is labeled but is not a purely
	// configurable argument at the resolved path (computed-only, Optional+
	// Computed, ConfigUnknown, or the subsection did not resolve) — the legacy
	// strip-label guidance, never a move.
	placementStripLabel
	// placementMisplaced: the attribute is labeled AND a purely configurable
	// argument at the resolved path — the section is wrong; move it to Argument
	// Reference rather than stripping its (correct) label (#60, #62).
	placementMisplaced
)

// classifyAttrPlacement judges a single attribute documented under Attribute
// Reference in a subsection resolved to schema path P, implementing the §3
// classification table by composing label state with configurableArgAtPath:
//
//   - unlabeled                          -> placementOK (proper computed output)
//   - labeled, pure-config arg at P       -> placementMisplaced (move it)
//   - labeled, not pure-config / unresolved -> placementStripLabel (legacy)
//
// resolved is the ok result from resolveSubsectionPath and must gate the
// misplacement branch: it distinguishes a genuine root subsection (path == ""
// with resolved == true, for #62 top-level scalars) from an *unresolved*
// subsection that also carries an empty path — without it, an attribute whose
// name happened to match a configurable root argument would be spuriously
// flagged as misplaced. Optional+Computed and ConfigUnknown are non-misplacement
// by construction (configurableArgAtPath returns false), honoring the #62 guard.
func classifyAttrPlacement(rs *schema.ResourceSchema, path string, resolved bool, attr doc.DocAttribute) placement {
	if !attr.Required && !attr.Optional {
		return placementOK
	}
	if resolved && configurableArgAtPath(rs, path, attr.Name) {
		return placementMisplaced
	}
	return placementStripLabel
}

// configurableArgAtPath reports whether attrName is a purely configurable
// argument ((Required || Optional) && !Computed) of the schema block at the
// given canonical map-key path — either as a scalar attribute, or as an
// immediate child block that (itself or via a descendant) carries such an
// attribute. ConfigUnknown blocks and unresolved paths return false so an
// ERROR never fires on a guess. Optional+Computed scalars are excluded because
// they may legitimately appear under either section.
func configurableArgAtPath(rs *schema.ResourceSchema, path, attrName string) bool {
	if rs == nil {
		return false
	}
	b, ok := rs.Blocks[path]
	if !ok || b.ConfigUnknown {
		return false
	}
	for _, a := range b.Attributes {
		if a.Name == attrName {
			return (a.Required || a.Optional) && !a.Computed
		}
	}
	if childPath, found := childPathForAttr(path, b, attrName); found {
		return blockTreeHasPureConfigurable(rs, childPath, make(map[string]bool))
	}
	return false
}

// childBlockPath returns the canonical ResourceSchema.Blocks key for a
// ChildBlocks entry of the block at parentPath. Entries appear in two forms in
// this codebase — a bare leaf name (from the provider loader) or a full
// dot-path (common in fixtures and normalized elsewhere via leafName). A
// full-path entry is used as-is; a bare leaf is joined to the parent path.
func childBlockPath(parentPath, child string) string {
	if strings.Contains(child, ".") {
		return child
	}
	if parentPath == "" {
		return child
	}
	return parentPath + "." + child
}

// childPathForAttr returns the canonical schema path of the immediate child
// block of pb (at parentPath) whose leaf name is attrName, matching by leaf so
// both ChildBlocks representations resolve. The second result reports whether
// such a child exists.
func childPathForAttr(parentPath string, pb *schema.Block, attrName string) (string, bool) {
	for _, child := range pb.ChildBlocks {
		if leafName(child) == attrName {
			return childBlockPath(parentPath, child), true
		}
	}
	return "", false
}

// blockTreeHasPureConfigurable reports whether the schema block at path, or any
// descendant block, has a purely configurable (Required or Optional and NOT
// Computed) attribute. Blocks are looked up by exact full path (unambiguous),
// ConfigUnknown blocks are skipped, and visited guards against pathological
// cycles.
func blockTreeHasPureConfigurable(rs *schema.ResourceSchema, path string, visited map[string]bool) bool {
	if rs == nil || visited[path] {
		return false
	}
	visited[path] = true
	b, ok := rs.Blocks[path]
	if !ok || b.ConfigUnknown {
		return false
	}
	for _, a := range b.Attributes {
		if (a.Required || a.Optional) && !a.Computed {
			return true
		}
	}
	for _, child := range b.ChildBlocks {
		if blockTreeHasPureConfigurable(rs, childBlockPath(path, child), visited) {
			return true
		}
	}
	return false
}

func severity(attr schema.Attribute) Severity {
	if attr.Deprecated {
		return SeverityWarning
	}
	return SeverityError
}

func findDocBlock(d *doc.Document, leaf string, fullPath string) *doc.DocBlock {
	return findDocBlockIn(d.Blocks(), leaf, fullPath)
}

// findDocBlockIn applies the same composite-path resolution as findDocBlock
// but against a caller-supplied map. Used to resolve a path within the
// AttributeBlocks-only or ArgumentBlocks-only view when a check needs to
// distinguish where a documented attribute lives.
//
// Returns the first matching block. For coverage checks that need the full
// set of documented attributes for a schema path (which can be split
// across multiple doc blocks — e.g. a leaf-keyed `### \`probabilistic\“
// heading and a dot-notation-routed `rule.probabilistic` block from a
// reference like `rule[*].probabilistic[*].x` in the attribute section),
// use findAllDocBlocksIn.
func findDocBlockIn(blocks map[string]*doc.DocBlock, leaf string, fullPath string) *doc.DocBlock {
	matches := findAllDocBlocksIn(blocks, leaf, fullPath, nil)
	if len(matches) == 0 {
		return nil
	}
	return matches[0]
}

// findAllDocBlocksIn returns every DocBlock that the schema path could
// resolve to: full path, then 3- and 2-segment composites suffixed by
// leaf, then leaf alone. Order matters for the single-block consumer
// (findDocBlockIn returns the first), but for coverage-style checks the
// caller should iterate all of them so attributes documented under
// alternative key shapes (e.g. leaf vs full path) are all counted.
//
// anchors (Document.BlockAnchors) enables shared-subsection resolution: when
// the parent block documents this leaf with a bullet that links to another
// subsection (`management` - ... See [Endpoint](#endpoint)), that subsection's
// block is included too. This is precise — anchored on the exact parent+leaf
// bullet — so it never mis-credits sibling paths. Pass nil to disable.
func findAllDocBlocksIn(blocks map[string]*doc.DocBlock, leaf string, fullPath string, anchors map[string]string) []*doc.DocBlock {
	if fullPath == "" {
		if b := blocks[""]; b != nil {
			return []*doc.DocBlock{b}
		}
		return nil
	}

	seen := make(map[*doc.DocBlock]bool)
	var matches []*doc.DocBlock
	add := func(b *doc.DocBlock) {
		if b == nil || seen[b] {
			return
		}
		seen[b] = true
		matches = append(matches, b)
	}

	add(blocks[fullPath])
	parts := strings.Split(fullPath, ".")
	if len(parts) >= 3 {
		for i := len(parts) - 3; i >= 0; i-- {
			add(blocks[parts[i]+"."+parts[i+1]+"."+leaf])
		}
	}
	if len(parts) >= 2 {
		for i := len(parts) - 2; i >= 0; i-- {
			add(blocks[parts[i]+"."+leaf])
		}
	}
	add(blocks[leaf])

	// Shared-subsection link: locate the bullet for this leaf in the parent
	// block and, if it links to a differently-named subsection, include that
	// subsection's block. Only the leaf's own bullet is followed, so a
	// structurally-identical sibling (endpoints.management vs
	// endpoints.intercluster) each resolves to the shared block, while
	// unrelated paths are untouched. The parent of a single-segment path is
	// the root block ("").
	if anchors != nil {
		parent := strings.Join(parts[:len(parts)-1], ".")
		for _, pb := range findAllDocBlocksIn(blocks, leafName(parent), parent, nil) {
			for _, a := range pb.Attributes {
				if a.Name == leaf && a.LinkAnchor != "" {
					if target, ok := anchors[a.LinkAnchor]; ok && target != leaf {
						add(blocks[target])
					}
				}
			}
		}
	}
	return matches
}

func existsInSiblingBlock(rs *schema.ResourceSchema, leaf, attrName string) bool {
	for path, block := range rs.Blocks {
		if leafName(path) != leaf {
			continue
		}
		for _, attr := range block.Attributes {
			if attr.Name == attrName {
				return true
			}
		}
		if slices.Contains(block.ChildBlocks, attrName) {
			return true
		}
		for _, child := range block.ChildBlocks {
			if leafName(child) == attrName {
				return true
			}
		}
	}
	return false
}

func leafName(path string) string {
	if path == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i+1:]
		}
	}
	return path
}

func displayPath(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

func findMalformed(malformed []doc.MalformedAttr, name string) (doc.MalformedAttr, bool) {
	for _, m := range malformed {
		if m.Name == name {
			return m, true
		}
	}
	return doc.MalformedAttr{}, false
}

func checkBlockOrdering(resource, ruleName, section, blockPath string, block *doc.DocBlock) []Result {
	var results []Result
	var required, optional, unmarked []string
	for _, attr := range block.Attributes {
		switch {
		case attr.Required:
			required = append(required, attr.Name)
		case attr.Optional:
			optional = append(optional, attr.Name)
		default:
			unmarked = append(unmarked, attr.Name)
		}
	}

	// If the doc explicitly split required/optional with separate bylines
	// ("The following arguments are required:" / "The following arguments
	// are optional:"), check each group independently. Otherwise check
	// all labeled attributes as a single alphabetical list.
	if block.SplitByLabel {
		for _, group := range [][]string{required, optional, unmarked} {
			if r := checkSliceOrdering(group, resource, ruleName, section, blockPath); r != nil {
				results = append(results, *r)
			}
		}
	} else {
		all := make([]string, 0, len(block.Attributes))
		for _, attr := range block.Attributes {
			all = append(all, attr.Name)
		}
		if r := checkSliceOrdering(all, resource, ruleName, section, blockPath); r != nil {
			results = append(results, *r)
		}
	}
	return results
}

func checkSliceOrdering(names []string, resource, ruleName, section, blockName string) *Result {
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			return &Result{
				Rule: ruleName, Resource: resource, Severity: SeverityError,
				Message: fmt.Sprintf("%s %q should come before %q in %s block %q", section, names[i], names[i-1], section, displayPath(blockName)),
				Block:   blockName,
			}
		}
	}
	return nil
}

func isListProse(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "the following arguments") || strings.Contains(lower, "the following attributes") {
		return true
	}
	if strings.Contains(lower, "this resource supports") || strings.Contains(lower, "this data source supports") {
		return true
	}
	if strings.Contains(lower, "this resource exports") || strings.Contains(lower, "this data source exports") {
		return true
	}
	if strings.HasPrefix(line, "~>") || strings.HasPrefix(line, "->") || strings.HasPrefix(line, "!>") {
		return true
	}
	// A legacy nested-block lead-in ("The `x[0].y[0]` block also exports:")
	// introduces a block, like a heading — not an interruption.
	if _, ok := doc.NestedBlockLeadIn(line); ok {
		return true
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// --- Byline ---

// checkBylines validates that the first paragraph after ## Argument Reference
// and ## Attribute Reference matches one of the expected byline texts defined
// in the type block.
func (r *SchemaDocsRule) checkBylines(ctx CheckContext) []Result {
	if ctx.Type == nil || ctx.Doc == nil {
		return nil
	}

	var results []Result

	// Arguments byline.
	if len(ctx.Type.ArgumentsBylines) > 0 && ctx.Doc.Sections.Arguments != nil {
		results = append(results, r.checkSectionByline(ctx, ctx.Doc.Sections.Arguments, ctx.Type.ArgumentsBylines, ctx.Type.AllowMissingArgumentsByline, "Argument Reference")...)
	}

	// Attributes byline.
	if len(ctx.Type.AttributesBylines) > 0 && ctx.Doc.Sections.Attributes != nil {
		results = append(results, r.checkSectionByline(ctx, ctx.Doc.Sections.Attributes, ctx.Type.AttributesBylines, false, "Attribute Reference")...)
	}

	return results
}

func (r *SchemaDocsRule) checkSectionByline(ctx CheckContext, section *doc.Section, expected []string, allowMissing bool, sectionName string) []Result {
	if len(section.Paragraphs) == 0 {
		if !allowMissing {
			return []Result{{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("%s section is missing a byline paragraph", sectionName),
			}}
		}
		return nil
	}

	// Get the text of the first paragraph.
	source := ctx.Doc.Source()
	firstPara := section.Paragraphs[0]
	paraText := strings.TrimSpace(string(firstPara.Text(source)))

	if slices.Contains(expected, paraText) {
		return nil
	}

	return []Result{{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
		Message: fmt.Sprintf("%s byline %q does not match expected texts", sectionName, truncate(paraText, 80)),
	}}
}

// --- Deprecated ---

// checkDeprecated verifies that attributes marked deprecated in the schema
// are also marked as deprecated in the documentation.
func (r *SchemaDocsRule) checkDeprecated(ctx CheckContext) []Result {
	if ctx.Schema == nil {
		return nil
	}

	var results []Result
	for blockPath, schemaBlock := range ctx.Schema.Blocks {
		docBlock := findDocBlock(ctx.Doc, leafName(blockPath), blockPath)
		if docBlock == nil {
			continue
		}

		schemaAttrs := make(map[string]schema.Attribute, len(schemaBlock.Attributes))
		for _, a := range schemaBlock.Attributes {
			schemaAttrs[a.Name] = a
		}

		docAttrs := make(map[string]*doc.DocAttribute, len(docBlock.Attributes))
		for i := range docBlock.Attributes {
			docAttrs[docBlock.Attributes[i].Name] = &docBlock.Attributes[i]
		}

		// Schema deprecated but doc not marked.
		for _, attr := range schemaBlock.Attributes {
			if !attr.Deprecated {
				continue
			}
			da, ok := docAttrs[attr.Name]
			if !ok {
				continue // not documented — coverage check handles this
			}
			if !da.Deprecated {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("attribute %q in block %q is deprecated in schema but not marked as deprecated in docs", attr.Name, displayPath(blockPath)),
					Block:   blockPath,
					Line:    da.Line,
				})
			}
		}

		// Doc marked deprecated but schema is not.
		for _, da := range docBlock.Attributes {
			if !da.Deprecated {
				continue
			}
			sa, ok := schemaAttrs[da.Name]
			if !ok {
				continue // phantom — coverage check handles this
			}
			if !sa.Deprecated {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("attribute %q in block %q is marked deprecated in docs but not in schema; either mark as deprecated in schema or remove the deprecation notice", da.Name, displayPath(blockPath)),
					Block:   blockPath,
					Line:    da.Line,
				})
			}
		}
	}
	return results
}
