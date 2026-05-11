// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// Attributes that are always implicitly present and don't need documentation.
var implicitAttributes = []string{"id", "tags_all"}

// Attributes with special documentation handling.
var specialAttributes = []string{"tags", "tags_all"}

// CompletenessRule checks that all schema attributes are documented and vice versa.
type CompletenessRule struct {
	IgnoreDeprecated bool
}

func (r *CompletenessRule) Name() string { return "completeness" }

func (r *CompletenessRule) Check(resource string, rs *schema.ResourceSchema, d *doc.Document) []Result {
	var results []Result

	// Track which leaf names we've already reported as missing blocks.
	// For recursive schemas (e.g., wafv2), the same block structure appears at
	// hundreds of paths — report it once.
	reportedMissingBlocks := make(map[string]bool)

	for blockPath, schemaBlock := range rs.Blocks {
		// Skip the timeouts block — it has its own ## Timeouts section.
		if blockPath == "timeouts" {
			continue
		}

		// Determine which doc block to compare against.
		// Schema uses dot-paths ("rule.action"), docs use the leaf block name ("action").
		docBlockName := leafName(blockPath)
		docBlock := findDocBlock(d, docBlockName, blockPath)

		if docBlock == nil {
			// If the schema block has attributes, report the missing doc section.
			// Skip if it only has child blocks and no attributes.
			if len(schemaBlock.Attributes) > 0 {
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
			if shouldSkipAttribute(attr, r.IgnoreDeprecated) {
				continue
			}
			if !documented[attr.Name] {
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: resource,
					Severity: severity(attr),
					Message:  fmt.Sprintf("attribute %q in block %q is not documented", attr.Name, displayPath(blockPath)),
					Block:    blockPath,
				})
			}
		}

		// Check each documented attribute exists in schema.
		schemaAttrNames := make(map[string]bool, len(schemaBlock.Attributes))
		for _, attr := range schemaBlock.Attributes {
			schemaAttrNames[attr.Name] = true
		}
		// Also include child block names (they appear as attributes in docs via "See [block] below").
		for _, child := range schemaBlock.ChildBlocks {
			schemaAttrNames[child] = true
		}

		for _, docAttr := range docBlock.Attributes {
			if !schemaAttrNames[docAttr.Name] && !slices.Contains(specialAttributes, docAttr.Name) {
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

func shouldSkipAttribute(attr schema.Attribute, ignoreDeprecated bool) bool {
	if slices.Contains(implicitAttributes, attr.Name) {
		return true
	}
	if ignoreDeprecated && attr.Deprecated {
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
// Falls back to checking the full path segments.
func findDocBlock(d *doc.Document, leafName string, fullPath string) *doc.DocBlock {
	blocks := d.Blocks()
	if b, ok := blocks[leafName]; ok {
		return b
	}
	// For root block, look up ""
	if fullPath == "" {
		return blocks[""]
	}
	return nil
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
