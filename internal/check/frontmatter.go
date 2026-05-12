// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"bytes"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// FrontmatterRule validates the YAML frontmatter of a provider documentation
// file — the block delimited by "---" at the top of each .html.markdown / .md
// file. It is the successor to tfproviderdocs's FrontMatterCheck; the field
// coverage and error shapes are chosen to find the same problems.
//
// All require/forbid toggles default to false so the rule is inert until
// configured. AllowedSubcategories, when non-empty, restricts the set of valid
// subcategory values; an empty slice means "any subcategory is fine".
type FrontmatterRule struct {
	RequireSubcategory   bool
	RequirePageTitle     bool
	RequireDescription   bool
	RequireLayout        bool
	ForbidSubcategory    bool
	ForbidPageTitle      bool
	ForbidDescription    bool
	ForbidLayout         bool
	ForbidSidebarCurrent bool

	AllowedSubcategories []string
}

func (r *FrontmatterRule) Name() string { return "frontmatter" }

// CheckFile inspects the frontmatter block at the top of content. If no
// frontmatter block is present, every RequireX toggle produces a result. If
// the block exists but fails to parse as YAML, a single parse-error result is
// returned and individual field checks are skipped.
func (r *FrontmatterRule) CheckFile(resource, _ string, content []byte) []Result {
	block, ok := extractFrontmatter(content)
	if !ok {
		return r.missingFrontmatterResults(resource)
	}

	var fm frontmatter
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return []Result{{
			Rule:     r.Name(),
			Resource: resource,
			Severity: SeverityError,
			Message:  fmt.Sprintf("YAML frontmatter parse error: %s", err),
		}}
	}

	return r.checkFields(resource, &fm)
}

// missingFrontmatterResults produces one error per RequireX toggle when the
// file has no frontmatter block at all.
func (r *FrontmatterRule) missingFrontmatterResults(resource string) []Result {
	var results []Result
	add := func(field string) {
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: resource,
			Severity: SeverityError,
			Message:  fmt.Sprintf("YAML frontmatter missing required %s (no frontmatter block found)", field),
		})
	}
	if r.RequireSubcategory {
		add("subcategory")
	}
	if r.RequirePageTitle {
		add("page_title")
	}
	if r.RequireDescription {
		add("description")
	}
	if r.RequireLayout {
		add("layout")
	}
	return results
}

// checkFields validates an already-parsed frontmatter struct against every
// configured toggle. It aggregates results rather than short-circuiting so a
// single run surfaces every problem.
func (r *FrontmatterRule) checkFields(resource string, fm *frontmatter) []Result {
	var results []Result
	fail := func(msg string) {
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: resource,
			Severity: SeverityError,
			Message:  msg,
		})
	}

	requireRules := []struct {
		want  bool
		have  bool
		field string
	}{
		{r.RequireSubcategory, fm.has("subcategory"), "subcategory"},
		{r.RequirePageTitle, fm.has("page_title"), "page_title"},
		{r.RequireDescription, fm.has("description"), "description"},
		{r.RequireLayout, fm.has("layout"), "layout"},
	}
	for _, rr := range requireRules {
		if rr.want && !rr.have {
			fail(fmt.Sprintf("YAML frontmatter missing required %s", rr.field))
		}
	}

	forbidRules := []struct {
		want  bool
		have  bool
		field string
	}{
		{r.ForbidSubcategory, fm.has("subcategory"), "subcategory"},
		{r.ForbidPageTitle, fm.has("page_title"), "page_title"},
		{r.ForbidDescription, fm.has("description"), "description"},
		{r.ForbidLayout, fm.has("layout"), "layout"},
		{r.ForbidSidebarCurrent, fm.has("sidebar_current"), "sidebar_current"},
	}
	for _, fr := range forbidRules {
		if fr.want && fr.have {
			fail(fmt.Sprintf("YAML frontmatter should not contain %s", fr.field))
		}
	}

	if len(r.AllowedSubcategories) > 0 && fm.has("subcategory") {
		if !slices.Contains(r.AllowedSubcategories, fm.Subcategory) {
			fail(fmt.Sprintf("YAML frontmatter subcategory %q is not in the allowed list", fm.Subcategory))
		}
	}

	return results
}

// frontmatter models the five frontmatter fields swissshepherd cares about.
// A custom UnmarshalYAML records which keys were present in the source so the
// rule can distinguish missing from empty — something a plain struct decode
// would lose.
type frontmatter struct {
	Description    string
	Layout         string
	PageTitle      string
	SidebarCurrent string
	Subcategory    string

	present map[string]bool
}

func (f *frontmatter) has(field string) bool {
	return f.present[field]
}

// UnmarshalYAML records the set of top-level keys present in the mapping node
// before delegating to a standard struct decode. This preserves presence
// information for every known field without resorting to *string everywhere.
func (f *frontmatter) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected YAML mapping, got kind %d", node.Kind)
	}

	f.present = make(map[string]bool, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if k := node.Content[i]; k.Kind == yaml.ScalarNode {
			f.present[k.Value] = true
		}
	}

	// Decode values into a sibling type to avoid recursing back into
	// UnmarshalYAML on the same receiver.
	type fmFields struct {
		Description    string `yaml:"description"`
		Layout         string `yaml:"layout"`
		PageTitle      string `yaml:"page_title"`
		SidebarCurrent string `yaml:"sidebar_current"`
		Subcategory    string `yaml:"subcategory"`
	}
	var raw fmFields
	if err := node.Decode(&raw); err != nil {
		return err
	}
	f.Description = raw.Description
	f.Layout = raw.Layout
	f.PageTitle = raw.PageTitle
	f.SidebarCurrent = raw.SidebarCurrent
	f.Subcategory = raw.Subcategory
	return nil
}

// extractFrontmatter returns the YAML block between the leading "---" and the
// next "---" line. Accepts either LF or CRLF line endings. Returns (nil,
// false) when the file does not begin with a frontmatter delimiter.
func extractFrontmatter(content []byte) ([]byte, bool) {
	// Accept "---\n" or "---\r\n" as the opener.
	after, ok := bytes.CutPrefix(content, []byte("---\n"))
	if !ok {
		after, ok = bytes.CutPrefix(content, []byte("---\r\n"))
		if !ok {
			return nil, false
		}
	}

	// Find the closing "---" at the start of a line. Accept either terminator
	// (including a closing "---" with no trailing newline at EOF).
	for _, sep := range [][]byte{
		[]byte("\n---\n"),
		[]byte("\n---\r\n"),
		[]byte("\n---"),
	} {
		if block, _, ok := bytes.Cut(after, sep); ok {
			return block, true
		}
	}

	// Opener found but no closer — treat as absent so the caller emits a
	// cleaner "no frontmatter" diagnostic rather than a confusing parse error.
	return nil, false
}
