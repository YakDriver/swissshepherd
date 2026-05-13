// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// HeadingStyleRule checks that block headings use the preferred format.
type HeadingStyleRule struct {
	Preferred doc.HeadingTemplates
}

func (r *HeadingStyleRule) Name() string { return "heading_style" }

func (r *HeadingStyleRule) Check(resource string, rs *schema.ResourceSchema, d *doc.Document) []Result {
	if len(r.Preferred) == 0 {
		return nil
	}
	// Non-block-schema types have no nested block structure to align headings
	// against. No findings.
	if rs == nil {
		return nil
	}

	// Build set of schema block leaf names and detect ambiguous ones
	// (same leaf name appears with different attribute sets).
	schemaLeaves := make(map[string]bool)
	ambiguousLeaves := make(map[string]bool)
	leafAttrs := make(map[string]string) // leaf -> sorted attr signature
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
	for _, blocks := range []map[string]*doc.DocBlock{d.ArgumentBlocks, d.AttributeBlocks} {
		for _, block := range blocks {
			if block.Name == "" || block.Heading == "" {
				continue
			}
			// Only lint headings for blocks that exist in the schema.
			if !schemaLeaves[block.Name] {
				continue
			}
			// Check if the heading matches any preferred template
			if r.Preferred.Match(block.Heading) != "" {
				continue
			}
			// Suggest {Parent} form for ambiguous blocks, plain form otherwise.
			var suggested string
			if ambiguousLeaves[block.Name] {
				suggested = fmt.Sprintf("`{parent}` `%s` Block", block.Name)
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: resource,
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("block %q heading %q is ambiguous (multiple blocks share this name with different schemas); use parent to disambiguate: %s", block.Name, block.Heading, suggested),
					Block:    block.Name,
				})
			} else {
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
					Rule:     r.Name(),
					Resource: resource,
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("block %q heading %q should be %q", block.Name, block.Heading, suggested),
					Block:    block.Name,
				})
			}
		}
	}
	return results
}

// blockSignature returns a string representing the attribute names of a block,
// used to detect when the same leaf name has different schemas.
func blockSignature(block *schema.Block) string {
	var names []string
	for _, a := range block.Attributes {
		names = append(names, a.Name)
	}
	return strings.Join(names, ",")
}
