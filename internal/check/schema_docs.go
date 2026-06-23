// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"bufio"
	"bytes"
	"fmt"
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
	"A ", "An ", "The ", "Indicates ", "Specifies ", "Describes ", "Defines ",
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
		docBlocks := findAllDocBlocksIn(ctx.Doc.Blocks(), docBlockName, blockPath)

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

		argDocs := findAllDocBlocksIn(ctx.Doc.ArgumentBlocks, leafName(blockPath), blockPath)
		attrDocs := findAllDocBlocksIn(ctx.Doc.AttributeBlocks, leafName(blockPath), blockPath)

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
			if inAttrs || (allowInline && inArgs) {
				continue
			}

			key := blockPath + "." + attr.Name
			if reported[key] {
				continue
			}
			reported[key] = true

			var msg string
			if blockPath == "" {
				msg = fmt.Sprintf("Read-Only attribute %q should be documented in Attribute Reference section", attr.Name)
			} else {
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
	leafPaths := make(map[string][]string)
	for path, block := range rs.Blocks {
		leaf := leafName(path)
		schemaLeaves[leaf] = true
		if path != "" {
			leafPaths[leaf] = append(leafPaths[leaf], path)
		}
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
			// only safe disambiguator is the full dot-notation path.
			//
			// Skip the ambiguity warning when the heading is keyed by a
			// full path already (block.Name contains a dot) — the author
			// has already opted into the path form.
			if ambiguousLeaves[blockLeaf] && !strings.Contains(block.Name, ".") {
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
				paths := append([]string(nil), leafPaths[blockLeaf]...)
				sort.Strings(paths)
				example := ""
				if len(paths) > 0 {
					example = doc.RenderHeading(pathTemplate, paths[0])
				}
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf(
						"block %q heading %q is ambiguous (schema has %d blocks named %q: %s); use the full dot-path form, e.g. %q",
						block.Name, block.Heading, len(paths), blockLeaf, strings.Join(paths, ", "), example,
					),
					Block: block.Name,
				})
			}

			// Preferred-style check: does the heading match one of the
			// preferred templates? Independent of ambiguity.
			if r.Preferred.Match(block.Heading) != "" {
				continue
			}
			var suggested string
			for _, tmpl := range r.Preferred {
				if !strings.Contains(tmpl, "{Parent}") && !strings.Contains(tmpl, "{Path}") {
					suggested = doc.RenderHeading(tmpl, blockLeaf)
					break
				}
			}
			if suggested == "" {
				suggested = doc.RenderHeading("`{Block}` Block", blockLeaf)
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

func blockSignature(block *schema.Block) string {
	var names []string
	for _, a := range block.Attributes {
		names = append(names, a.Name)
	}
	return strings.Join(names, ",")
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

	// Attributes must NOT have (Required) or (Optional)
	for blockName, block := range ctx.Doc.AttributeBlocks {
		for _, attr := range block.Attributes {
			if attr.Required || attr.Optional {
				label := "(Optional)"
				if attr.Required {
					label = "(Required)"
				}
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("attribute %q in block %q should not have %s label", attr.Name, displayPath(blockName), label),
					Block:   blockName,
					Line:    attr.Line,
				})
			}
		}
	}

	return results
}

// --- Shared helpers ---

func hasConfigurableAttributes(block *schema.Block) bool {
	for _, attr := range block.Attributes {
		if attr.Required || attr.Optional {
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
	matches := findAllDocBlocksIn(blocks, leaf, fullPath)
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
func findAllDocBlocksIn(blocks map[string]*doc.DocBlock, leaf string, fullPath string) []*doc.DocBlock {
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
