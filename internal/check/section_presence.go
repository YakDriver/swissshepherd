// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"

	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
)

// SectionPresenceRule checks that required sections are present and forbidden
// sections are absent, as declared by the Type's section requirements.
//
// It reads require_attributes, require_import, require_timeouts, and
// require_signature from ctx.Type. No config fields — all requirements come
// from the type block.
type SectionPresenceRule struct{}

func (r *SectionPresenceRule) Name() string { return "section_presence" }

func (r *SectionPresenceRule) Check(ctx CheckContext) []Result {
	t := ctx.Type
	if t == nil {
		return nil
	}

	var results []Result
	check := func(req config.SectionRequirement, section *doc.Section, name string) {
		switch req {
		case config.SectionRequired:
			if section == nil {
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: ctx.Resource,
					Severity: SeverityError,
					Message:  fmt.Sprintf("missing required section: ## %s", name),
				})
			}
		case config.SectionForbidden:
			if section != nil {
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: ctx.Resource,
					Severity: SeverityError,
					Message:  fmt.Sprintf("section ## %s is not allowed for type %q", name, t.Name),
				})
			}
		}
	}

	s := ctx.Doc.Sections
	check(t.RequireAttributes, s.Attributes, "Attribute Reference")
	check(t.RequireImport, s.Import, "Import")
	check(t.RequireTimeouts, s.Timeouts, "Timeouts")
	check(t.RequireSignature, s.Signature, "Signature")

	return results
}
