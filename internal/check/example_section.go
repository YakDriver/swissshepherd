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
//   - Heading text is "Example Usage"
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

	expected := "Example Usage"

	var results []Result
	add := func(sev Severity, msg string) {
		results = append(results, Result{Severity: sev, Rule: r.Name(), Resource: ctx.Resource, Message: msg})
	}

	if section.Text != expected {
		add(SeverityError, fmt.Sprintf("example section heading %q should be: %q", section.Text, expected))
	}

	allowed := r.AllowedLanguages
	if len(allowed) == 0 {
		allowed = []string{"terraform", "hcl"}
	}

	// Check that at least one code block uses an allowed language.
	// Non-terraform blocks (json, console, etc.) are permitted as
	// supplementary material.
	hasAllowed := false
	for _, cb := range section.FencedCodeBlocks {
		lang := string(cb.Language(source))
		if lang == "" || slices.Contains(allowed, lang) {
			hasAllowed = true
		}
	}
	if len(section.FencedCodeBlocks) > 0 && !hasAllowed {
		add(SeverityError, fmt.Sprintf("example section has no code block with an allowed language (%s)", strings.Join(allowed, ", ")))
	}

	// Check that at least one allowed-language code block contains the resource name.
	hasResource := false
	for _, cb := range section.FencedCodeBlocks {
		lang := string(cb.Language(source))
		if lang != "" && !slices.Contains(allowed, lang) {
			continue
		}
		if strings.Contains(codeBlockText(cb, source), ctx.Resource) {
			hasResource = true
			break
		}
	}
	if len(section.FencedCodeBlocks) > 0 && !hasResource {
		add(SeverityWarning, fmt.Sprintf("no example code block contains resource name %q", ctx.Resource))
	}

	return results
}
