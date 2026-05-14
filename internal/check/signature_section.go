// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"strings"
)

// SignatureSectionRule validates the content of the ## Signature section.
// Checks:
//   - At least one code block is present
//   - Code block contains the function name
//   - Code block contains all parameter names from the schema
type SignatureSectionRule struct{}

func (r *SignatureSectionRule) Name() string { return "signature_section" }

func (r *SignatureSectionRule) Check(ctx CheckContext) []Result {
	section := ctx.Doc.Sections.Signature
	if section == nil {
		return nil // presence checked by section_presence rule
	}

	source := ctx.Doc.Source()
	if len(source) == 0 {
		return nil
	}

	var results []Result
	add := func(sev Severity, msg string) {
		results = append(results, Result{Severity: sev, Rule: r.Name(), Resource: ctx.Resource, Message: msg})
	}

	if len(section.FencedCodeBlocks) == 0 {
		add(SeverityError, "signature section must include a code block")
		return results
	}

	text := codeBlockText(section.FencedCodeBlocks[0], source)

	// Check function name is present
	names := ctx.ResourceNames()
	hasName := false
	for _, n := range names {
		if strings.Contains(text, n) {
			hasName = true
			break
		}
	}
	if !hasName {
		add(SeverityError, fmt.Sprintf("signature code block should contain function name %q", ctx.Resource))
	}

	// Schema-driven: check parameter names are documented
	if ctx.FunctionSchema != nil {
		for _, param := range ctx.FunctionSchema.ParameterNames {
			// Check for "param " (param followed by type) to avoid substring matches
			if !strings.Contains(text, param+" ") {
				add(SeverityWarning, fmt.Sprintf("signature code block missing parameter %q", param))
			}
		}
		if ctx.FunctionSchema.VariadicParameter != "" && !strings.Contains(text, ctx.FunctionSchema.VariadicParameter+" ") {
			add(SeverityWarning, fmt.Sprintf("signature code block missing variadic parameter %q", ctx.FunctionSchema.VariadicParameter))
		}
	}

	return results
}
