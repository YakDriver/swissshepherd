// Copyright (c) YakDriver, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// ComputedAttributeRule checks that computed-only attributes appear in the
// Attribute Reference section (not the Argument Reference section).
type ComputedAttributeRule struct{}

func (r *ComputedAttributeRule) Name() string { return "computed_attribute" }

func (r *ComputedAttributeRule) Check(resource string, rs *schema.ResourceSchema, d *doc.Document) []Result {
	var results []Result

	// Only check the root block for now — nested computed attrs are typically
	// documented as block[*].attr in the attribute section.
	rootBlock := rs.Blocks[""]
	if rootBlock == nil {
		return nil
	}

	// Collect computed-only attribute names from schema root.
	var computedOnly []string
	for _, attr := range rootBlock.Attributes {
		if attr.Computed && !attr.Optional && !attr.Required {
			if slices.Contains(implicitAttributes, attr.Name) {
				continue
			}
			computedOnly = append(computedOnly, attr.Name)
		}
	}

	if len(computedOnly) == 0 {
		return nil
	}

	// Build set of attributes documented in the Attribute Reference section.
	attrBlock := d.AttributeBlocks[""]
	documentedInAttrs := make(map[string]bool)
	if attrBlock != nil {
		for _, a := range attrBlock.Attributes {
			documentedInAttrs[a.Name] = true
		}
	}

	// Build set of attributes documented in the Argument Reference section.
	argBlock := d.ArgumentBlocks[""]
	documentedInArgs := make(map[string]bool)
	if argBlock != nil {
		for _, a := range argBlock.Attributes {
			documentedInArgs[a.Name] = true
		}
	}

	for _, name := range computedOnly {
		if !documentedInAttrs[name] {
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: resource,
				Severity: SeverityError,
				Message:  fmt.Sprintf("computed attribute %q should be documented in Attribute Reference section", name),
			})
		}
		if documentedInArgs[name] {
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: resource,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("computed-only attribute %q should not appear in Argument Reference section", name),
			})
		}
	}

	return results
}
