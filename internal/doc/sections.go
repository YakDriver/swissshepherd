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
}
