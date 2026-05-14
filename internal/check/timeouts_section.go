// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"
)

// TimeoutsSectionRule validates that the documented timeout actions match the
// schema. It checks:
//   - Every schema timeout action is documented
//   - Every documented timeout action exists in the schema
//
// No config fields — entirely schema-driven. Path scoping (prefixes, targets)
// is handled by the Runner's AppliesTo mechanism.
type TimeoutsSectionRule struct{}

func (r *TimeoutsSectionRule) Name() string { return "timeouts_section" }

func (r *TimeoutsSectionRule) Check(ctx CheckContext) []Result {
	if ctx.Schema == nil || ctx.Doc.Sections.Timeouts == nil {
		return nil
	}

	timeoutsBlock, ok := ctx.Schema.Blocks["timeouts"]
	if !ok {
		// No timeouts in schema — section_presence already handles this.
		return nil
	}

	// Schema timeout actions.
	var schemaActions []string
	for _, attr := range timeoutsBlock.Attributes {
		schemaActions = append(schemaActions, attr.Name)
	}

	// Documented timeout actions.
	documented := make(map[string]bool, len(ctx.Doc.Sections.Timeouts.ListItems))
	for _, item := range ctx.Doc.Sections.Timeouts.ListItems {
		documented[item.Name] = true
	}

	var results []Result

	// Check every schema action is documented.
	for _, action := range schemaActions {
		if !documented[action] {
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: SeverityError,
				Message:  fmt.Sprintf("timeout action %q is configured in schema but not documented", action),
			})
		}
	}

	// Check every documented action exists in schema.
	schemaSet := make(map[string]bool, len(schemaActions))
	for _, a := range schemaActions {
		schemaSet[a] = true
	}
	for _, item := range ctx.Doc.Sections.Timeouts.ListItems {
		if !schemaSet[item.Name] && !slices.Contains(schemaActions, item.Name) {
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: SeverityError,
				Line:     item.Line,
				Message:  fmt.Sprintf("documented timeout action %q does not exist in schema", item.Name),
			})
		}
	}

	return results
}
