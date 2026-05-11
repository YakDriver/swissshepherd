// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// OrderingRule checks that documented arguments and attributes are alphabetically ordered.
// For arguments, ordering is checked within required and optional groups separately.
type OrderingRule struct{}

func (r *OrderingRule) Name() string { return "ordering" }

func (r *OrderingRule) Check(resource string, _ *schema.ResourceSchema, d *doc.Document) []Result {
	var results []Result

	results = append(results, checkBlockOrdering(resource, r.Name(), "argument", d.ArgumentBlocks)...)
	results = append(results, checkBlockOrdering(resource, r.Name(), "attribute", d.AttributeBlocks)...)

	return results
}

func checkBlockOrdering(resource, ruleName, section string, blocks map[string]*doc.DocBlock) []Result {
	var results []Result

	for blockName, block := range blocks {
		// Split into groups: required, optional, and unmarked (attributes section).
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

		// Check each group independently.
		for _, group := range [][]string{required, optional, unmarked} {
			if r := checkSliceOrdering(group, resource, ruleName, section, blockName); r != nil {
				results = append(results, *r)
			}
		}
	}

	return results
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
