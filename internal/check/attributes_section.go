// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"
)

// AttributesSectionRule validates the ## Attribute Reference section:
//   - Schema coverage: every computed-only attribute is documented
//   - Ordering: alphabetical
//   - No Required/Optional labels (those belong in arguments)
type AttributesSectionRule struct {
	ImplicitAttributes []string
}

func (r *AttributesSectionRule) Name() string { return "attributes_section" }

func (r *AttributesSectionRule) implicit() []string {
	if r.ImplicitAttributes != nil {
		return r.ImplicitAttributes
	}
	return DefaultImplicitAttributes
}

func (r *AttributesSectionRule) Check(ctx CheckContext) []Result {
	rs := ctx.Schema

	// When no schema is available, still check ordering on doc blocks.
	if rs == nil {
		var results []Result
		for blockPath, block := range ctx.Doc.AttributeBlocks {
			results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "attribute", blockPath, block)...)
		}
		return results
	}

	rootBlock := rs.Blocks[""]
	if rootBlock == nil {
		return nil
	}

	var results []Result

	// Collect computed-only attribute names from schema root.
	var computedOnly []string
	for _, attr := range rootBlock.Attributes {
		if attr.Computed && !attr.Optional && !attr.Required {
			if slices.Contains(r.implicit(), attr.Name) {
				continue
			}
			computedOnly = append(computedOnly, attr.Name)
		}
	}

	// Build set of attributes documented in the Attribute Reference section.
	attrBlock := ctx.Doc.AttributeBlocks[""]
	documentedInAttrs := make(map[string]bool)
	if attrBlock != nil {
		for _, a := range attrBlock.Attributes {
			documentedInAttrs[a.Name] = true
		}
	}

	// Schema → doc: every computed-only attribute should be documented.
	for _, name := range computedOnly {
		if !documentedInAttrs[name] {
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
				Message: fmt.Sprintf("computed attribute %q should be documented in Attribute Reference section", name),
			})
		}
	}

	// Ordering: alphabetical within the attribute block.
	if attrBlock != nil {
		results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "attribute", "", attrBlock)...)
	}

	return results
}
