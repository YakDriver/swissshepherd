// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc

import "github.com/yuin/goldmark/ast"

// Section captures one top-level section of a provider documentation file:
// a heading and the content that follows it up to the next heading of the
// same or lower level. Only the pieces rules actually read — the heading
// node, its text for convenience, the fenced code blocks, and the paragraphs —
// are tracked. List nodes are deliberately excluded because ArgumentBlocks
// and AttributeBlocks already cover the only sections where lists matter.
type Section struct {
	Heading          *ast.Heading
	Text             string
	FencedCodeBlocks []*ast.FencedCodeBlock
	Paragraphs       []*ast.Paragraph

	// ChildHeadings are headings at level > the section's own level that
	// appear before the next same-or-lower-level heading. Used by rules
	// that check for required sub-sections (e.g. "### Basic Usage" inside
	// "## Example Usage").
	ChildHeadings []ChildHeading

	// ListItems are the top-level list items found in this section. Used by
	// rules that validate structured lists (e.g. timeout actions).
	ListItems []SectionListItem

	// StartOffset and EndOffset are byte offsets into the source marking the
	// section's extent (from the heading's first byte to the byte before the
	// next same-or-lower-level heading, or EOF). Rules that need raw source
	// patterns (e.g. import block format) slice source[StartOffset:EndOffset].
	StartOffset int
	EndOffset   int
}

// ChildHeading records a heading nested inside a section.
type ChildHeading struct {
	Level int
	Text  string
	Line  int // 1-based line number of the heading in source
}

// SectionListItem records a single top-level list item in a section.
// Name is the backtick-wrapped identifier (e.g. "create") and Value is the
// remainder after the " - " separator (e.g. "(Default `60m`)").
type SectionListItem struct {
	Name  string
	Value string
	Line  int
}

// Sections is the typed view of the top-level sections of a doc file. Any
// field is nil when the corresponding section is absent — for example a data
// source will typically have Import == nil, a function will have Signature
// set but no Import or Timeouts.
//
// The struct has fixed fields (rather than a map) so rules get type-safe
// access and new categories are an explicit code change. Add fields when a
// new section genuinely needs rule-level support; don't use this as a
// general-purpose bag.
type Sections struct {
	Title      *Section // level-1 heading (# Resource: foo)
	Signature  *Section // ## Signature (functions)
	Example    *Section // ## Example Usage
	Arguments  *Section // ## Argument Reference
	Attributes *Section // ## Attribute Reference
	Timeouts   *Section // ## Timeouts
	Import     *Section // ## Import

	// UnknownHeadings records every level-2 heading that did not map to one
	// of the recognized sections above. The parser captures these so the
	// section_presence rule can flag stray sections like ## My wild heading.
	// Each entry's Level is always 2; Text is the raw heading text.
	UnknownHeadings []ChildHeading
}
