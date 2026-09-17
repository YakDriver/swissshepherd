// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"sort"
)

// AnchorsRule verifies that every in-page link (`](#fragment)`) in a document
// resolves to a heading that actually exists in the same file. When a block
// heading is renamed to the required "`name` Block" style (anchor slug
// "name-block"), "See below" links left pointing at the old slug ("#name")
// become dead links that render silently on the Terraform Registry. The rule
// validates link fragments captured anywhere in the document — prose, bullets,
// callouts, any section — against the set of heading anchors the page
// generates (including GitHub's duplicate "-1"/"-2" suffixes).
//
// Only in-page fragments are validated; external URLs and cross-file links are
// out of scope. The rule is schema-independent: it needs only the parsed
// document.
type AnchorsRule struct {
	severity Severity
}

// NewAnchorsRule returns an AnchorsRule that emits findings at the given
// severity.
func NewAnchorsRule(severity Severity) *AnchorsRule {
	return &AnchorsRule{severity: severity}
}

func (r *AnchorsRule) Name() string { return "anchors" }

func (r *AnchorsRule) Check(ctx CheckContext) []Result {
	d := ctx.Doc
	if d == nil || len(d.InPageLinks) == 0 {
		return nil
	}
	anchors := d.HeadingAnchors

	// Report each unresolved fragment once, at its earliest line, so a
	// fragment reused across the document is flagged a single time and output
	// is deterministic. Fragments are already normalized to their
	// browser-resolved form (entities/backslash escapes resolved,
	// percent-decoded) by the parser.
	dangling := make(map[string]int)
	for _, link := range d.InPageLinks {
		if link.Fragment == "" || anchors[link.Fragment] {
			continue
		}
		if ln, ok := dangling[link.Fragment]; !ok || link.Line < ln {
			dangling[link.Fragment] = link.Line
		}
	}

	frags := make([]string, 0, len(dangling))
	for frag := range dangling {
		frags = append(frags, frag)
	}
	sort.Strings(frags)

	var results []Result
	for _, frag := range frags {
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: ctx.Resource,
			Severity: r.severity,
			Line:     dangling[frag],
			Message:  fmt.Sprintf("in-page link %q does not resolve to any heading in the document (dead anchor)", "#"+frag),
		})
	}
	return results
}
