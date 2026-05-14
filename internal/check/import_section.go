// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/yuin/goldmark/ast"
)

// ImportSectionRule validates the content of the ## Import section.
// Checks:
//   - Style: no passive voice ("can be imported"), no "e.g."
//   - Structure: code blocks present, contain resource name, correct types
//   - Ordering: terraform blocks before console blocks
//   - Identity: if resource has identity schema and RequireIdentitySection is
//     true, validates identity import block and ### Identity Schema subsection
type ImportSectionRule struct {
	RequireIdentitySection bool
}

func (r *ImportSectionRule) Name() string { return "import_section" }

func (r *ImportSectionRule) Check(ctx CheckContext) []Result {
	section := ctx.Doc.Sections.Import
	if section == nil {
		return nil
	}

	source := ctx.Doc.Source()
	var results []Result

	add := func(sev Severity, msg string) {
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: ctx.Resource,
			Severity: sev,
			Message:  msg,
		})
	}

	// --- Style checks on first paragraph ---
	if len(section.Paragraphs) > 0 {
		text := paragraphText(section.Paragraphs[0], source)

		problems := []struct {
			pattern string
			fix     string
		}{
			{"can be imported", "use active voice instead: Import X using A, B, C."},
			{"e.g.", `use "For example:" instead`},
			{"E.g.", `use "For example:" instead`},
		}
		for _, p := range problems {
			if strings.Contains(text, p.pattern) {
				add(SeverityWarning, fmt.Sprintf("import section should not include %q, %s", p.pattern, p.fix))
			}
		}
	}

	// --- Code block checks ---
	cannotImport := len(section.Paragraphs) > 0 && strings.Contains(paragraphText(section.Paragraphs[0], source), "cannot import")

	if !cannotImport && len(section.FencedCodeBlocks) == 0 {
		add(SeverityError, "import section should have at least one code block (or state that the resource cannot be imported)")
		return results
	}

	hitConsole := false
	for i, cb := range section.FencedCodeBlocks {
		lang := string(cb.Language(source))
		text := codeBlockText(cb, source)

		hasName := false
		for _, n := range ctx.ResourceNames() {
			if strings.Contains(text, n) {
				hasName = true
				break
			}
		}
		if !hasName {
			add(SeverityError, fmt.Sprintf("import code block should contain resource name %q", ctx.Resource))
		}

		if lang != "terraform" && lang != "console" {
			add(SeverityError, fmt.Sprintf("import code block type should be 'terraform' or 'console', got %q", lang))
		}

		if i == 0 && (lang != "terraform" || !strings.HasPrefix(text, "import {")) {
			add(SeverityError, "first import code block should be ```terraform with an import block (import {)")
		}

		if lang == "console" && !strings.HasPrefix(text, "% ") {
			add(SeverityError, "import code block type 'console' should begin with '% '")
		}

		if lang == "console" {
			hitConsole = true
		}
		if hitConsole && lang == "terraform" && strings.HasPrefix(text, "import ") {
			add(SeverityError, "all terraform import blocks should appear before console blocks")
		}
	}

	// --- Identity checks ---
	if r.RequireIdentitySection && ctx.IdentitySchema != nil {
		r.checkIdentity(ctx, section, source, &results)
	}

	return results
}

func (r *ImportSectionRule) checkIdentity(ctx CheckContext, section *doc.Section, source []byte, results *[]Result) {
	add := func(sev Severity, msg string) {
		*results = append(*results, Result{
			Rule:     r.Name(),
			Resource: ctx.Resource,
			Severity: sev,
			Message:  msg,
		})
	}

	// Check first code block uses identity = { ... }
	if len(section.FencedCodeBlocks) > 0 {
		text := codeBlockText(section.FencedCodeBlocks[0], source)
		if !strings.Contains(text, "identity") {
			add(SeverityError, "resource has identity schema; first import block should use identity = { ... }")
		}
	}

	// Check ### Identity Schema subsection exists
	hasIdentityHeading := false
	for _, ch := range section.ChildHeadings {
		if strings.Contains(ch.Text, "Identity Schema") {
			hasIdentityHeading = true
			break
		}
	}
	if !hasIdentityHeading {
		add(SeverityError, "resource has identity schema; import section should include ### Identity Schema subsection")
		return
	}

	// Parse documented identity attributes from the section source.
	// Format: * `name` (Type) Description
	sectionText := string(source[section.StartOffset:section.EndOffset])
	documentedAttrs := parseIdentityAttrs(sectionText)

	schemaAttrs := make(map[string]bool, len(ctx.IdentitySchema.Attributes))
	for _, attr := range ctx.IdentitySchema.Attributes {
		schemaAttrs[attr.Name] = true
	}

	for _, attr := range ctx.IdentitySchema.Attributes {
		if !documentedAttrs[attr.Name] {
			add(SeverityError, fmt.Sprintf("identity attribute %q is in schema but not documented in Identity Schema section", attr.Name))
		}
	}
	for name := range documentedAttrs {
		if !schemaAttrs[name] {
			add(SeverityWarning, fmt.Sprintf("documented identity attribute %q does not exist in identity schema", name))
		}
	}
}

// parseIdentityAttrs extracts attribute names from identity schema list items.
// Matches lines like: * `name` (Type) Description
// or: - `name` (Type) Description
func parseIdentityAttrs(text string) map[string]bool {
	attrs := make(map[string]bool)
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "* `") && !strings.HasPrefix(line, "- `") {
			continue
		}
		// Extract name between backticks
		start := strings.Index(line, "`")
		if start < 0 {
			continue
		}
		end := strings.Index(line[start+1:], "`")
		if end < 0 {
			continue
		}
		name := line[start+1 : start+1+end]
		if name != "" && !strings.Contains(name, " ") {
			attrs[name] = true
		}
	}
	return attrs
}

// paragraphText extracts the raw text content of a paragraph node.
func paragraphText(p *ast.Paragraph, source []byte) string {
	var sb strings.Builder
	for i := 0; i < p.Lines().Len(); i++ {
		line := p.Lines().At(i)
		sb.Write(line.Value(source))
	}
	return sb.String()
}

// codeBlockText extracts the text content of a fenced code block.
func codeBlockText(cb *ast.FencedCodeBlock, source []byte) string {
	var sb strings.Builder
	for i := 0; i < cb.Lines().Len(); i++ {
		line := cb.Lines().At(i)
		sb.Write(line.Value(source))
	}
	return sb.String()
}
