// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
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

	// Sub-check toggles (nil = enabled)
	Coverage    *bool
	Ordering    *bool
	Description *bool
	Heading     *bool
	Format      *bool
	Labels      *bool
	Byline      *bool

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
		docBlock := findDocBlock(ctx.Doc, docBlockName, blockPath)

		if docBlock == nil {
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

		documented := make(map[string]bool, len(docBlock.Attributes))
		for _, attr := range docBlock.Attributes {
			documented[attr.Name] = true
		}

		for _, attr := range schemaBlock.Attributes {
			if r.shouldSkipAttribute(attr) {
				continue
			}
			if !documented[attr.Name] {
				msg := fmt.Sprintf("attribute %q in block %q is not documented", attr.Name, displayPath(blockPath))
				if m, ok := findMalformed(docBlock.MalformedAttributes, attr.Name); ok {
					msg = fmt.Sprintf("attribute %q in block %q is documented but missing the \" - \" separator (expected: * `%s` - (Required|Optional) ...)", attr.Name, displayPath(blockPath), attr.Name)
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: severity(attr), Message: msg, Block: blockPath, Line: m.Line,
					})
				} else {
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: severity(attr), Message: msg, Block: blockPath,
					})
				}
			} else if m, ok := findMalformed(docBlock.MalformedAttributes, attr.Name); ok {
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

		for _, docAttr := range docBlock.Attributes {
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
				})
			}
		}

		// Computed-only misplacement in arguments (use ArgumentBlocks directly,
		// not the merged view, to avoid false positives from attribute section).
		if blockPath == "" {
			if argBlock := ctx.Doc.ArgumentBlocks[""]; argBlock != nil {
				results = append(results, r.checkComputedMisplacement(ctx, schemaBlock, argBlock, ctx.Doc.AttributeBlocks[""])...)
			}
		}
	}

	// Attributes: computed-only coverage
	rootBlock := rs.Blocks[""]
	if rootBlock != nil {
		results = append(results, r.checkAttributeCoverage(ctx, rootBlock)...)
	}

	return results
}

func (r *SchemaDocsRule) checkAttributeCoverage(ctx CheckContext, rootBlock *schema.Block) []Result {
	var results []Result

	attrBlock := ctx.Doc.AttributeBlocks[""]
	documentedInAttrs := make(map[string]bool)
	if attrBlock != nil {
		for _, a := range attrBlock.Attributes {
			documentedInAttrs[a.Name] = true
		}
	}

	for _, attr := range rootBlock.Attributes {
		if attr.Computed && !attr.Optional && !attr.Required {
			if slices.Contains(r.implicit(), attr.Name) {
				continue
			}
			if !documentedInAttrs[attr.Name] {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
					Message: fmt.Sprintf("computed attribute %q should be documented in Attribute Reference section", attr.Name),
				})
			}
		}
	}
	return results
}

func (r *SchemaDocsRule) checkComputedMisplacement(ctx CheckContext, schemaBlock *schema.Block, argBlock *doc.DocBlock, attrBlock *doc.DocBlock) []Result {
	var results []Result
	documented := make(map[string]bool, len(argBlock.Attributes))
	for _, attr := range argBlock.Attributes {
		documented[attr.Name] = true
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
			if !schemaLeaves[block.Name] {
				continue
			}
			if r.Preferred.Match(block.Heading) != "" {
				continue
			}
			if ambiguousLeaves[block.Name] {
				suggested := fmt.Sprintf("`{parent}` `%s` Block", block.Name)
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("block %q heading %q is ambiguous (multiple blocks share this name with different schemas); use parent to disambiguate: %s", block.Name, block.Heading, suggested),
					Block:   block.Name,
				})
			} else {
				var suggested string
				for _, tmpl := range r.Preferred {
					if !strings.Contains(tmpl, "{Parent}") {
						suggested = doc.RenderHeading(tmpl, block.Name)
						break
					}
				}
				if suggested == "" {
					suggested = doc.RenderHeading("`{Block}` Block", block.Name)
				}
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("block %q heading %q should be %q", block.Name, block.Heading, suggested),
					Block:   block.Name,
				})
			}
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

	// Arguments must have (Required) or (Optional)
	for blockName, block := range ctx.Doc.ArgumentBlocks {
		for _, attr := range block.Attributes {
			if !attr.Required && !attr.Optional {
				// Skip if this attr is also in the attribute section (template bleed)
				if ns, ok := attrSectionNames[blockName]; ok && ns[attr.Name] {
					continue
				}
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("argument %q in block %q is missing (Required) or (Optional) label", attr.Name, displayPath(blockName)),
					Block:   blockName,
				})
			}
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
	blocks := d.Blocks()
	if fullPath == "" {
		return blocks[""]
	}
	parts := strings.Split(fullPath, ".")
	if len(parts) >= 3 {
		for i := len(parts) - 3; i >= 0; i-- {
			composite := parts[i] + "." + parts[i+1] + "." + leaf
			if b, ok := blocks[composite]; ok {
				return b
			}
		}
	}
	if len(parts) >= 2 {
		for i := len(parts) - 2; i >= 0; i-- {
			composite := parts[i] + "." + leaf
			if b, ok := blocks[composite]; ok {
				return b
			}
		}
	}
	if b, ok := blocks[leaf]; ok {
		return b
	}
	return nil
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
	for _, group := range [][]string{required, optional, unmarked} {
		if r := checkSliceOrdering(group, resource, ruleName, section, blockPath); r != nil {
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
