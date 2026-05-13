// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// FormatStyleRule checks structural formatting of argument/attribute sections:
// - No code blocks
// - Single-line attribute descriptions (no continuation lines)
// - Uninterrupted attribute lists (no unexpected prose between list items)
//
// All three checks default to enabled. Set a field to a pointer to false to
// disable it: check "format_style" { no_code_blocks = false }.
//
// Implements FileRule: it scans raw lines rather than the goldmark AST, so it
// can catch whitespace and continuation issues that are normalized away by the
// parser.
type FormatStyleRule struct {
	// nil means enabled (default true). Set to pointer-to-false to disable.
	NoCodeBlocks       *bool
	SingleLineAttrs    *bool
	UninterruptedLists *bool
}

func (r *FormatStyleRule) noCodeBlocks() bool { return r.NoCodeBlocks == nil || *r.NoCodeBlocks }
func (r *FormatStyleRule) singleLineAttrs() bool {
	return r.SingleLineAttrs == nil || *r.SingleLineAttrs
}
func (r *FormatStyleRule) uninterruptedLists() bool {
	return r.UninterruptedLists == nil || *r.UninterruptedLists
}

func (r *FormatStyleRule) Name() string { return "format_style" }

// CheckFile runs format checks against the raw bytes of a documentation file.
func (r *FormatStyleRule) CheckFile(ctx FileCheckContext) []Result {
	resource, content := ctx.Resource, ctx.Content
	var results []Result
	var inSection bool
	var inCodeBlock bool
	var inList bool
	var prevWasAttr bool
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "## Argument Reference") || strings.HasPrefix(line, "## Attribute Reference") {
			inSection = true
			inList = false
			prevWasAttr = false
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") {
			inSection = false
			inCodeBlock = false
			inList = false
		}
		if !inSection {
			continue
		}

		// Code block check
		if strings.HasPrefix(line, "```") {
			if r.noCodeBlocks() && !inCodeBlock {
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: resource,
					Severity: SeverityError,
					Message:  fmt.Sprintf("code block in argument/attribute section (line %d)", lineNum),
				})
			}
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		isAttrLine := strings.HasPrefix(line, "* `")
		isHeading := strings.HasPrefix(line, "#")
		isBlank := line == ""

		// Single-line check: continuation line after an attribute
		if r.singleLineAttrs() && prevWasAttr && !isAttrLine && !isHeading && !isBlank && strings.HasPrefix(line, "  ") {
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: resource,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("multi-line attribute description (line %d); each attribute should be on one line", lineNum),
			})
		}

		// Uninterrupted list check
		if r.uninterruptedLists() && inList && !isAttrLine && !isHeading && !isBlank && !strings.HasPrefix(line, "  ") && !isListProse(line) {
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: resource,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("attribute list interrupted (line %d): %q", lineNum, truncate(line, 60)),
			})
			inList = false
		}

		if isAttrLine {
			inList = true
		}
		if isHeading {
			inList = false
		}
		prevWasAttr = isAttrLine
	}

	return results
}

// isListProse returns true for accepted prose lines that separate argument groups.
func isListProse(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "the following arguments") || strings.Contains(lower, "the following attributes") {
		return true
	}
	if strings.Contains(lower, "this resource supports") || strings.Contains(lower, "this data source supports") {
		return true
	}
	if strings.Contains(lower, "this resource exports") || strings.Contains(lower, "this data source exports") {
		return true
	}
	// Callout boxes (~>, ->, !>)
	if strings.HasPrefix(line, "~>") || strings.HasPrefix(line, "->") || strings.HasPrefix(line, "!>") {
		return true
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
