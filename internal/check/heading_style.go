// Copyright (c) YakDriver, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"

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

	// Build set of schema block leaf names for filtering.
	schemaLeaves := make(map[string]bool)
	for path := range rs.Blocks {
		schemaLeaves[leafName(path)] = true
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
			// Heading matched an accepted style but not a preferred one — suggest fix
			suggested := doc.RenderHeading(string(r.Preferred[0]), block.Name)
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: resource,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("block %q heading %q should be %q", block.Name, block.Heading, suggested),
				Block:    block.Name,
			})
		}
	}
	return results
}
