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
// Overridable via check "completeness" { implicit_attributes = [...] }.
var DefaultImplicitAttributes = []string{"id", "tags_all"}

// DefaultPhantomAllowlist are attribute names that may appear in docs without
// a corresponding schema entry (provider-injected attributes). Overridable
// via check "completeness" { phantom_allowlist = [...] }.
var DefaultPhantomAllowlist = []string{"tags", "tags_all"}

// DefaultSkipBlocks are block names that are always skipped by the
// completeness check. Overridable via check "completeness" { skip_blocks = [...] }.
var DefaultSkipBlocks = []string{"timeouts"}

// CompletenessRule checks that all schema attributes are documented and vice versa.
type CompletenessRule struct {
	// IgnoreDeprecated skips deprecated attributes. Default true in wiring.
	IgnoreDeprecated bool
	// ImplicitAttributes overrides DefaultImplicitAttributes when non-nil.
	ImplicitAttributes []string
	// PhantomAllowlist overrides DefaultPhantomAllowlist when non-nil.
	PhantomAllowlist []string
	// SkipBlocks overrides DefaultSkipBlocks when non-nil.
	SkipBlocks []string
}

func (r *CompletenessRule) implicit() []string {
	if r.ImplicitAttributes != nil {
		return r.ImplicitAttributes
	}
	return DefaultImplicitAttributes
}

func (r *CompletenessRule) phantom() []string {
	if r.PhantomAllowlist != nil {
		return r.PhantomAllowlist
	}
	return DefaultPhantomAllowlist
}

func (r *CompletenessRule) skipBlocks() []string {
	if r.SkipBlocks != nil {
		return r.SkipBlocks
	}
	return DefaultSkipBlocks
}

func (r *CompletenessRule) Name() string { return "completeness" }

func (r *CompletenessRule) Check(ctx CheckContext) []Result {
	resource, rs, d := ctx.Resource, ctx.Schema, ctx.Doc
	// Types without a block schema (functions, content-only categories) can't
	// be validated for completeness — their "arguments" aren't attribute sets
	// at all. Return no findings rather than panicking on rs.Blocks.
	if rs == nil {
		return nil
	}

	var results []Result

	// Track which leaf names we've already reported as missing blocks.
	// For recursive schemas (e.g., wafv2), the same block structure appears at
	// hundreds of paths — report it once.
	reportedMissingBlocks := make(map[string]bool)
	// Track reported "doc attr not in schema" warnings by leaf+attr to deduplicate.
	reportedExtraAttrs := make(map[string]bool)

	for blockPath, schemaBlock := range rs.Blocks {
		// Skip configured skip-blocks (default: timeouts).
		if slices.Contains(r.skipBlocks(), blockPath) {
			continue
		}

		// Determine which doc block to compare against.
		// Schema uses dot-paths ("rule.action"), docs use the leaf block name ("action").
		docBlockName := leafName(blockPath)
		docBlock := findDocBlock(d, docBlockName, blockPath)

		if docBlock == nil {
			// If the schema block has configurable attributes, report the missing doc section.
			// Skip if it only has child blocks, no attributes, or only computed-only attributes.
			if hasConfigurableAttributes(schemaBlock) {
				// Deduplicate: only report each leaf name once.
				if reportedMissingBlocks[docBlockName] {
					continue
				}

				reportedMissingBlocks[docBlockName] = true
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: resource,
					Severity: SeverityError,
					Message:  fmt.Sprintf("block %q is not documented", displayPath(blockPath)),
					Block:    blockPath,
				})
			}
			continue
		}

		// Build set of documented attribute names.
		documented := make(map[string]bool, len(docBlock.Attributes))
		for _, attr := range docBlock.Attributes {
			documented[attr.Name] = true
		}

		// Check each schema attribute is documented.
		for _, attr := range schemaBlock.Attributes {
			if r.shouldSkipAttribute(attr) {
				continue
			}
			if !documented[attr.Name] {
				msg := fmt.Sprintf("attribute %q in block %q is not documented", attr.Name, displayPath(blockPath))
				if m, ok := findMalformed(docBlock.MalformedAttributes, attr.Name); ok {
					msg = fmt.Sprintf("attribute %q in block %q is documented but missing the \" - \" separator (expected: * `%s` - (Required|Optional) ...)", attr.Name, displayPath(blockPath), attr.Name)
					results = append(results, Result{
						Rule:     r.Name(),
						Resource: resource,
						Severity: severity(attr),
						Message:  msg,
						Block:    blockPath,
						Line:     m.Line,
					})
				} else {
					results = append(results, Result{
						Rule:     r.Name(),
						Resource: resource,
						Severity: severity(attr),
						Message:  msg,
						Block:    blockPath,
					})
				}
			} else if m, ok := findMalformed(docBlock.MalformedAttributes, attr.Name); ok {
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: resource,
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("attribute %q in block %q is documented but missing the \" - \" separator (expected: * `%s` - (Required|Optional) ...)", attr.Name, displayPath(blockPath), attr.Name),
					Block:    blockPath,
					Line:     m.Line,
				})
			}
		}

		// Check each documented attribute exists in schema.
		// Skip this check if the schema block is empty (leaf-name collision artifact).
		if len(schemaBlock.Attributes) == 0 && len(schemaBlock.ChildBlocks) == 0 {
			continue
		}
		schemaAttrNames := make(map[string]bool, len(schemaBlock.Attributes))
		for _, attr := range schemaBlock.Attributes {
			schemaAttrNames[attr.Name] = true
		}
		// Also include child block names (they appear as attributes in docs via "See [block] below").
		for _, child := range schemaBlock.ChildBlocks {
			schemaAttrNames[leafName(child)] = true
		}

		for _, docAttr := range docBlock.Attributes {
			if !schemaAttrNames[docAttr.Name] && !slices.Contains(r.phantom(), docAttr.Name) {
				// When multiple schema blocks share the same leaf name (e.g.
				// connection_pool.tcp and timeout.tcp both map to doc block "tcp"),
				// an attribute may be valid in a sibling block. Only report phantom
				// if no schema block with this leaf name contains the attribute.
				if existsInSiblingBlock(rs, leafName(blockPath), docAttr.Name) {
					continue
				}
				key := docBlockName + "." + docAttr.Name
				if reportedExtraAttrs[key] {
					continue
				}
				reportedExtraAttrs[key] = true
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: resource,
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("documented attribute %q in block %q does not exist in schema", docAttr.Name, displayPath(blockPath)),
					Block:    blockPath,
				})
			}
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

func (r *CompletenessRule) shouldSkipAttribute(attr schema.Attribute) bool {
	if slices.Contains(r.implicit(), attr.Name) {
		return true
	}
	if r.IgnoreDeprecated && attr.Deprecated {
		return true
	}
	// Computed-only attributes (not optional, not required) are typically documented
	// in the Attribute Reference section, not in the block's argument list.
	if attr.Computed && !attr.Optional && !attr.Required {
		return true
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
// Tries parent.leaf composite keys at all levels, then falls back to leaf-only.
func findDocBlock(d *doc.Document, leafName string, fullPath string) *doc.DocBlock {
	blocks := d.Blocks()
	// For root block, look up ""
	if fullPath == "" {
		return blocks[""]
	}
	parts := strings.Split(fullPath, ".")
	// Try two-level parent combinations first: "parent2.parent1.leaf"
	// For path "a.b.c.d", try "b.c.d", then "a.b.d"
	if len(parts) >= 3 {
		for i := len(parts) - 3; i >= 0; i-- {
			composite := parts[i] + "." + parts[i+1] + "." + leafName
			if b, ok := blocks[composite]; ok {
				return b
			}
		}
	}
	// Try single-level parent combinations: "parent.leaf"
	// For path "a.b.c.d", try "c.d", then "b.d", then "a.d"
	if len(parts) >= 2 {
		for i := len(parts) - 2; i >= 0; i-- {
			composite := parts[i] + "." + leafName
			if b, ok := blocks[composite]; ok {
				return b
			}
		}
	}
	// Fall back to leaf-only
	if b, ok := blocks[leafName]; ok {
		return b
	}
	return nil
}

// existsInSiblingBlock reports whether attrName exists as an attribute in any
// schema block whose leaf name matches leaf. This suppresses false phantom
// warnings when multiple schema paths share the same leaf block name and the
// doc merges them into a single section (e.g. connection_pool.tcp and
// timeout.tcp both documented under "### tcp Block").
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
		// ChildBlocks may store full paths (e.g. "timeout.grpc.per_request").
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

// findMalformed looks up a name in the malformed attributes list.
func findMalformed(malformed []doc.MalformedAttr, name string) (doc.MalformedAttr, bool) {
	for _, m := range malformed {
		if m.Name == name {
			return m, true
		}
	}
	return doc.MalformedAttr{}, false
}
