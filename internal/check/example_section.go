// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"
	"strings"
)

// ExampleSectionRule validates the content of the ## Example Usage section.
// Checks:
//   - All fenced code blocks use an allowed language (default: terraform, hcl)
//   - All code blocks contain the resource name
type ExampleSectionRule struct {
	AllowedLanguages []string
}

func (r *ExampleSectionRule) Name() string { return "example_section" }

func (r *ExampleSectionRule) Check(ctx CheckContext) []Result {
	section := ctx.Doc.Sections.Example
	if section == nil {
		return nil // presence checked by section_presence rule
	}

	source := ctx.Doc.Source()
	if len(source) == 0 {
		return nil
	}

	allowed := r.AllowedLanguages
	if len(allowed) == 0 {
		allowed = []string{"terraform", "hcl"}
	}

	var results []Result
	add := func(sev Severity, msg string) {
		results = append(results, Result{Severity: sev, Rule: r.Name(), Resource: ctx.Resource, Message: msg})
	}

	for _, cb := range section.FencedCodeBlocks {
		lang := string(cb.Language(source))

		if lang != "" && !contains(allowed, lang) {
			add(SeverityError, fmt.Sprintf("example code block language %q should be one of: %s", lang, strings.Join(allowed, ", ")))
		}

		text := codeBlockText(cb, source)
		if !strings.Contains(text, ctx.Resource) {
			add(SeverityWarning, fmt.Sprintf("example code block should contain resource name %q", ctx.Resource))
		}
	}

	return results
}

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}
