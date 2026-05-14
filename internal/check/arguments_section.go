// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// DefaultImplicitAttributes are attribute names that are always implicitly
// present in provider docs and don't need explicit documentation entries.
var DefaultImplicitAttributes = []string{"id", "tags_all"}

// DefaultPhantomAllowlist are attribute names that may appear in docs without
// a corresponding schema entry (provider-injected attributes).
var DefaultPhantomAllowlist = []string{"tags", "tags_all"}

// DefaultSkipBlocks are block names that are always skipped.
var DefaultSkipBlocks = []string{"timeouts"}

// ArgumentsSectionRule validates the ## Argument Reference section:
//   - Schema coverage: every configurable schema attribute is documented
//   - Ordering: alphabetical within required/optional groups
//   - Computed-only attributes should not appear here
//   - Documented attributes exist in schema (phantom check)
type ArgumentsSectionRule struct {
	IgnoreDeprecated   bool
	ImplicitAttributes []string
	PhantomAllowlist   []string
	SkipBlocks         []string
}

func (r *ArgumentsSectionRule) Name() string { return "arguments_section" }

func (r *ArgumentsSectionRule) implicit() []string {
	if r.ImplicitAttributes != nil {
		return r.ImplicitAttributes
	}
	return DefaultImplicitAttributes
}

func (r *ArgumentsSectionRule) phantom() []string {
	if r.PhantomAllowlist != nil {
		return r.PhantomAllowlist
	}
	return DefaultPhantomAllowlist
}

func (r *ArgumentsSectionRule) skipBlocks() []string {
	if r.SkipBlocks != nil {
		return r.SkipBlocks
	}
	return DefaultSkipBlocks
}

func (r *ArgumentsSectionRule) Check(ctx CheckContext) []Result {
	rs := ctx.Schema

	// When no schema is available, still check ordering on doc blocks.
	if rs == nil {
		var results []Result
		for blockPath, block := range ctx.Doc.ArgumentBlocks {
			results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "argument", blockPath, block)...)
		}
		return results
	}

	var results []Result
	reportedMissingBlocks := make(map[string]bool)
	reportedExtraAttrs := make(map[string]bool)

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
					Rule:     r.Name(),
					Resource: ctx.Resource,
					Severity: SeverityError,
					Message:  fmt.Sprintf("block %q is not documented", displayPath(blockPath)),
					Block:    blockPath,
				})
			}
			continue
		}

		// Schema → doc: every configurable attribute should be documented.
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

		// Ordering: alphabetical within required/optional/unmarked groups.
		results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "argument", blockPath, docBlock)...)

		// Computed-only misplacement: computed-only attrs shouldn't be in arguments.
		if blockPath == "" {
			results = append(results, r.checkComputedMisplacement(ctx, schemaBlock, docBlock)...)
		}
	}

	return results
}

func (r *ArgumentsSectionRule) checkComputedMisplacement(ctx CheckContext, schemaBlock *schema.Block, docBlock *doc.DocBlock) []Result {
	var results []Result
	documented := make(map[string]bool, len(docBlock.Attributes))
	for _, attr := range docBlock.Attributes {
		documented[attr.Name] = true
	}

	for _, attr := range schemaBlock.Attributes {
		if attr.Computed && !attr.Optional && !attr.Required {
			if slices.Contains(r.implicit(), attr.Name) {
				continue
			}
			if documented[attr.Name] {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("computed-only attribute %q should not appear in Argument Reference section", attr.Name),
				})
			}
		}
	}
	return results
}

func (r *ArgumentsSectionRule) shouldSkipAttribute(attr schema.Attribute) bool {
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

// checkBlockOrdering checks alphabetical ordering within a single doc block.
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

// hasConfigurableAttributes returns true if the block has at least one attribute
// that is required or optional (i.e., user-configurable, not computed-only).
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

// findDocBlock looks up the doc block by the leaf name of the schema path.
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

// existsInSiblingBlock reports whether attrName exists as an attribute in any
// schema block whose leaf name matches leaf.
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

func checkSliceOrdering(names []string, resource, ruleName, section, blockName string) *Result {
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			return &Result{
				Rule:     ruleName,
				Resource: resource,
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s %q should come before %q in %s block %q", section, names[i], names[i-1], section, displayPath(blockName)),
				Block:    blockName,
			}
		}
	}
	return nil
}
