// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"
	"strings"
)

// DefaultTitleSectionPrefixes is the allow-list of "<kind>:" prefixes that
// Terraform Registry and historical legacy layouts accept on a level-1
// documentation heading. Kept in sync with tfproviderdocs so migrating
// providers see identical verdicts. Callers may supply their own list via
// TitleSectionRule.AllowedPrefixes when a provider needs a different set.
var DefaultTitleSectionPrefixes = []string{
	"Action",
	"Data Source",
	"Ephemeral",
	"Function",
	"List Resource",
	"Resource",
}

// TitleSectionRule validates the first-level heading of a provider doc page:
//
//   - a title is present,
//   - its heading level is 1,
//   - its text begins with one of the allow-listed prefixes followed by ": ",
//   - no fenced code blocks appear before the first level-2 heading (example
//     code belongs under ## Example Usage).
//
// This is the successor to tfproviderdocs's checkTitleSection. Error messages
// are chosen to look familiar to anyone migrating from the older tool so the
// signal a provider acts on is unchanged.
type TitleSectionRule struct {
	// AllowedPrefixes overrides DefaultTitleSectionPrefixes. Empty uses the
	// default.
	AllowedPrefixes []string
}

func (r *TitleSectionRule) Name() string { return "title_section" }

func (r *TitleSectionRule) Check(ctx CheckContext) []Result {
	resource, d := ctx.Resource, ctx.Doc
	section := d.Sections.Title
	if section == nil {
		return []Result{{
			Rule:     r.Name(),
			Resource: resource,
			Severity: SeverityError,
			Message:  fmt.Sprintf("missing title section: # <Kind>: %s", resource),
		}}
	}

	var results []Result
	fail := func(msg string) {
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: resource,
			Severity: SeverityError,
			Message:  msg,
		})
	}

	if level := section.Heading.Level; level != 1 {
		fail(fmt.Sprintf("title section heading level (%d) should be: 1", level))
	}

	prefixes := r.AllowedPrefixes
	if len(prefixes) == 0 {
		prefixes = DefaultTitleSectionPrefixes
	}
	if !hasAnyPrefix(section.Text, prefixes) {
		fail(fmt.Sprintf("title section heading %q should have one of these prefixes: %v",
			section.Text, prefixes))
	}

	if len(section.FencedCodeBlocks) > 0 {
		fail("title section should not contain code blocks; move code examples to ## Example Usage")
	}

	return results
}

// hasAnyPrefix reports whether s begins with any of the given prefixes
// followed by ": ". The ": " separator is required so "Resource" alone does
// not match "Resources" — aligning with tfproviderdocs's format exactly.
func hasAnyPrefix(s string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool {
		return strings.HasPrefix(s, p+": ")
	})
}
