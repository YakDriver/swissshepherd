// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
)

// SectionPresenceRule enforces the structural integrity of a documentation
// file: required sections are present, forbidden sections are absent,
// sections appear in the order declared on the Type, and no stray level-2
// headings occur outside the recognized set.
//
// Configuration sources:
//
//   - Type.Sections — declares which sections this type may contain, in
//     what order, with required and forbidden flags. The order of section
//     blocks IS the canonical order. A Type with no Sections is treated as
//     "no structural rules" and skipped.
//   - CheckConfig.EnforceOrder — when nil or true, out-of-order sections
//     are reported. Set to false to skip order enforcement.
//   - CheckConfig.AllowUnknownSections — when nil or false, level-2
//     headings that do not match a recognized section are reported. Set
//     to true to permit them silently (e.g. for free-form provider docs).
//
// Special handling: Timeouts is also driven by the schema when one is
// available — the schema's timeouts block, not the Type, decides whether
// the section is required.
type SectionPresenceRule struct {
	EnforceOrder         *bool
	AllowUnknownSections *bool
}

func (r *SectionPresenceRule) Name() string { return "section_presence" }

func (r *SectionPresenceRule) Check(ctx CheckContext) []Result {
	t := ctx.Type
	if t == nil || len(t.Sections) == 0 || ctx.Doc == nil || ctx.Doc.Sections == nil {
		return nil
	}

	enforceOrder := true
	if r.EnforceOrder != nil {
		enforceOrder = *r.EnforceOrder
	}
	allowUnknown := false
	if r.AllowUnknownSections != nil {
		allowUnknown = *r.AllowUnknownSections
	}

	var results []Result
	results = append(results, r.checkPresence(ctx)...)
	if enforceOrder {
		results = append(results, r.checkOrder(ctx)...)
	}
	if !allowUnknown {
		results = append(results, r.checkUnknown(ctx)...)
	}
	return results
}

// checkPresence reports missing required sections and present forbidden
// sections. Timeouts is special-cased to honor the schema when available.
func (r *SectionPresenceRule) checkPresence(ctx CheckContext) []Result {
	var results []Result
	for _, spec := range ctx.Type.Sections {
		name := spec.SectionName()
		section := lookupSection(ctx.Doc.Sections, name)
		// Schema-driven Timeouts: if a schema is loaded, the schema decides
		// whether timeouts are configured. The Type's required/forbidden
		// flag still matters when there's no schema (e.g. content-only
		// types).
		if name == config.SectionTimeouts && ctx.Schema != nil {
			results = append(results, checkTimeoutsAgainstSchema(ctx, section)...)
			continue
		}
		switch {
		case spec.Required && section == nil:
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: SeverityError,
				Message:  fmt.Sprintf("missing required section: %s", sectionLabel(name)),
			})
		case spec.Forbidden && section != nil:
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: SeverityError,
				Message: fmt.Sprintf("section %s is not allowed for type %q",
					sectionLabel(name), ctx.Type.Name),
			})
		}
	}
	return results
}

// checkOrder reports each section that appears earlier in the doc than the
// section it should follow per the Type's spec. Forbidden sections that are
// nonetheless present are skipped here (presence already reported them).
func (r *SectionPresenceRule) checkOrder(ctx CheckContext) []Result {
	type seen struct {
		name   config.SectionName
		offset int
	}

	// Map name → spec position. Forbidden sections are excluded — order
	// applies only to sections that may legitimately appear.
	pos := make(map[config.SectionName]int, len(ctx.Type.Sections))
	for i, spec := range ctx.Type.Sections {
		if spec.Forbidden {
			continue
		}
		pos[spec.SectionName()] = i
	}

	// Collect every section that is actually present in the doc and is in
	// the spec, ordered by document offset.
	var present []seen
	for _, name := range config.AllSectionNames {
		if _, ok := pos[name]; !ok {
			continue
		}
		section := lookupSection(ctx.Doc.Sections, name)
		if section == nil {
			continue
		}
		present = append(present, seen{name: name, offset: section.StartOffset})
	}
	slices.SortFunc(present, func(a, b seen) int {
		return a.offset - b.offset
	})

	// Walk in document order. Each section's spec position must be >= the
	// previous one's. Report the first inversion per pair.
	var results []Result
	for i := 1; i < len(present); i++ {
		if pos[present[i].name] < pos[present[i-1].name] {
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: SeverityError,
				Message: fmt.Sprintf("section %s appears before %s; expected the reverse order",
					sectionLabel(present[i-1].name), sectionLabel(present[i].name)),
			})
		}
	}
	return results
}

// checkUnknown reports every level-2 heading that did not match a recognized
// section. The parser captured these in Sections.UnknownHeadings.
func (r *SectionPresenceRule) checkUnknown(ctx CheckContext) []Result {
	var results []Result
	for _, h := range ctx.Doc.Sections.UnknownHeadings {
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: ctx.Resource,
			Severity: SeverityError,
			Message:  fmt.Sprintf("unknown level-2 section: ## %s", h.Text),
			Line:     h.Line,
		})
	}
	return results
}

// checkTimeoutsAgainstSchema bridges the schema-driven timeouts check with
// the new section-spec model. When a schema is present, it overrides the
// Type's required/forbidden hint — what really matters is whether the
// schema actually configures timeouts.
func checkTimeoutsAgainstSchema(ctx CheckContext, section *doc.Section) []Result {
	timeoutsBlock, hasTimeouts := ctx.Schema.Blocks["timeouts"]
	if section != nil && !hasTimeouts {
		return []Result{{
			Rule:     "section_presence",
			Resource: ctx.Resource,
			Severity: SeverityError,
			Message:  "## Timeouts section is documented but the schema does not configure timeouts",
		}}
	}
	if section == nil && hasTimeouts {
		actions := make([]string, 0, len(timeoutsBlock.Attributes))
		for _, attr := range timeoutsBlock.Attributes {
			actions = append(actions, "'"+attr.Name+"'")
		}
		return []Result{{
			Rule:     "section_presence",
			Resource: ctx.Resource,
			Severity: SeverityError,
			Message: fmt.Sprintf("schema configures timeouts (%s) but ## Timeouts section is missing",
				strings.Join(actions, ", ")),
		}}
	}
	return nil
}

// lookupSection returns the doc.Section pointer for a given section name,
// or nil when the section is absent from the doc.
func lookupSection(s *doc.Sections, name config.SectionName) *doc.Section {
	switch name {
	case config.SectionTitle:
		return s.Title
	case config.SectionSignature:
		return s.Signature
	case config.SectionExample:
		return s.Example
	case config.SectionArguments:
		return s.Arguments
	case config.SectionAttributes:
		return s.Attributes
	case config.SectionTimeouts:
		return s.Timeouts
	case config.SectionImport:
		return s.Import
	}
	return nil
}

// sectionLabel returns the user-facing label for a section, e.g.
// "## Argument Reference" or "# <title>" for the H1 title.
func sectionLabel(name config.SectionName) string {
	if name == config.SectionTitle {
		return "# <title>"
	}
	return "## " + name.HeadingText()
}
