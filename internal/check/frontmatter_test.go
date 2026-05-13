// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"os"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
)

// validFrontmatter is the minimal frontmatter block with every field
// swissshepherd inspects. Individual tests mutate or strip fields.
const validFrontmatter = `---
subcategory: "Test"
layout: "test"
page_title: "Test: test_instance"
description: |-
  Manages a Test Instance.
---

# Resource: test_instance

Body.
`

func TestFrontmatterRule_NoToggles_EmitsNothing(t *testing.T) {
	t.Parallel()

	rule := &check.FrontmatterRule{}
	results := rule.CheckFile("test_instance", "test.md", []byte(validFrontmatter))

	if len(results) != 0 {
		t.Fatalf("expected 0 results with zero-value rule, got %d: %v", len(results), results)
	}
}

func TestFrontmatterRule_Require(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		rule    check.FrontmatterRule
		content string
		wantMsg string // substring, empty means expect no results
	}{
		"subcategory present passes": {
			rule:    check.FrontmatterRule{RequireSubcategory: true},
			content: validFrontmatter,
		},
		"subcategory missing fails": {
			rule:    check.FrontmatterRule{RequireSubcategory: true},
			content: replaceFrontmatter(validFrontmatter, "subcategory: \"Test\"\n", ""),
			wantMsg: "missing required subcategory",
		},
		"page_title missing fails": {
			rule:    check.FrontmatterRule{RequirePageTitle: true},
			content: replaceFrontmatter(validFrontmatter, "page_title: \"Test: test_instance\"\n", ""),
			wantMsg: "missing required page_title",
		},
		"description missing fails": {
			rule: check.FrontmatterRule{RequireDescription: true},
			content: replaceFrontmatter(validFrontmatter,
				"description: |-\n  Manages a Test Instance.\n", ""),
			wantMsg: "missing required description",
		},
		"layout missing fails": {
			rule:    check.FrontmatterRule{RequireLayout: true},
			content: replaceFrontmatter(validFrontmatter, "layout: \"test\"\n", ""),
			wantMsg: "missing required layout",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			results := tt.rule.CheckFile("test_instance", "test.md", []byte(tt.content))
			assertMessage(t, results, tt.wantMsg)
		})
	}
}

func TestFrontmatterRule_Forbid(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		rule    check.FrontmatterRule
		content string
		wantMsg string
	}{
		"layout present fails": {
			rule:    check.FrontmatterRule{ForbidLayout: true},
			content: validFrontmatter,
			wantMsg: "should not contain layout",
		},
		"layout absent passes": {
			rule:    check.FrontmatterRule{ForbidLayout: true},
			content: replaceFrontmatter(validFrontmatter, "layout: \"test\"\n", ""),
		},
		"sidebar_current present fails": {
			rule: check.FrontmatterRule{ForbidSidebarCurrent: true},
			content: insertIntoFrontmatter(validFrontmatter,
				"sidebar_current: \"docs-test\"\n"),
			wantMsg: "should not contain sidebar_current",
		},
		"sidebar_current absent passes": {
			rule:    check.FrontmatterRule{ForbidSidebarCurrent: true},
			content: validFrontmatter,
		},
		"subcategory present fails": {
			rule:    check.FrontmatterRule{ForbidSubcategory: true},
			content: validFrontmatter,
			wantMsg: "should not contain subcategory",
		},
		"page_title present fails": {
			rule:    check.FrontmatterRule{ForbidPageTitle: true},
			content: validFrontmatter,
			wantMsg: "should not contain page_title",
		},
		"description present fails": {
			rule:    check.FrontmatterRule{ForbidDescription: true},
			content: validFrontmatter,
			wantMsg: "should not contain description",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			results := tt.rule.CheckFile("test_instance", "test.md", []byte(tt.content))
			assertMessage(t, results, tt.wantMsg)
		})
	}
}

func TestFrontmatterRule_AllowedSubcategories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		allowed             []string
		allowEmptySubcatFor []string
		content             string
		wantMsg             string
	}{
		"value in allowlist passes": {
			allowed: []string{"Test", "Other"},
			content: validFrontmatter,
		},
		"value not in allowlist fails": {
			allowed: []string{"Other"},
			content: validFrontmatter,
			wantMsg: `subcategory "Test" is not in the allowed list`,
		},
		"empty allowlist treats any value as fine": {
			allowed: nil,
			content: validFrontmatter,
		},
		"allowlist does not fire when subcategory absent": {
			allowed: []string{"Other"},
			content: replaceFrontmatter(validFrontmatter, "subcategory: \"Test\"\n", ""),
		},
		"empty subcategory fails allowlist by default": {
			allowed: []string{"Other"},
			content: replaceFrontmatter(validFrontmatter, "subcategory: \"Test\"\n", "subcategory: \"\"\n"),
			wantMsg: `subcategory "" is not in the allowed list`,
		},
		"empty subcategory passes for named target": {
			allowed:             []string{"Other"},
			allowEmptySubcatFor: []string{"test_instance"},
			content:             replaceFrontmatter(validFrontmatter, "subcategory: \"Test\"\n", "subcategory: \"\"\n"),
		},
		"empty subcategory still fails for unlisted target": {
			allowed:             []string{"Other"},
			allowEmptySubcatFor: []string{"other_resource"},
			content:             replaceFrontmatter(validFrontmatter, "subcategory: \"Test\"\n", "subcategory: \"\"\n"),
			wantMsg:             `subcategory "" is not in the allowed list`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rule := &check.FrontmatterRule{AllowedSubcategories: tt.allowed, AllowEmptySubcategoryTargets: tt.allowEmptySubcatFor}
			results := rule.CheckFile("test_instance", "test.md", []byte(tt.content))
			assertMessage(t, results, tt.wantMsg)
		})
	}
}

func TestFrontmatterRule_MultipleFailuresAggregated(t *testing.T) {
	t.Parallel()

	// Strip subcategory and page_title; require both + forbid description.
	content := replaceFrontmatter(validFrontmatter, "subcategory: \"Test\"\n", "")
	content = replaceFrontmatter(content, "page_title: \"Test: test_instance\"\n", "")

	rule := &check.FrontmatterRule{
		RequireSubcategory: true,
		RequirePageTitle:   true,
		ForbidDescription:  true,
	}
	results := rule.CheckFile("test_instance", "test.md", []byte(content))

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d: %v", len(results), resultMessages(results))
	}

	want := []string{
		"missing required subcategory",
		"missing required page_title",
		"should not contain description",
	}
	for _, w := range want {
		if !anyContains(results, w) {
			t.Errorf("expected a result containing %q, got: %v", w, resultMessages(results))
		}
	}
}

func TestFrontmatterRule_NoFrontmatterBlock(t *testing.T) {
	t.Parallel()

	content := "# Resource: test\n\nNo frontmatter at all.\n"

	t.Run("inert rule passes", func(t *testing.T) {
		t.Parallel()
		rule := &check.FrontmatterRule{}
		if got := rule.CheckFile("test_resource", "p.md", []byte(content)); len(got) != 0 {
			t.Fatalf("expected 0 results, got %d: %v", len(got), resultMessages(got))
		}
	})

	t.Run("require toggles each emit one result", func(t *testing.T) {
		t.Parallel()
		rule := &check.FrontmatterRule{
			RequireSubcategory: true,
			RequirePageTitle:   true,
			RequireDescription: true,
			RequireLayout:      true,
		}
		results := rule.CheckFile("test_resource", "p.md", []byte(content))
		if len(results) != 4 {
			t.Fatalf("expected 4 results, got %d: %v", len(results), resultMessages(results))
		}
		for _, r := range results {
			if !strings.Contains(r.Message, "no frontmatter block found") {
				t.Errorf("expected 'no frontmatter block found' hint, got: %s", r.Message)
			}
		}
	})

	t.Run("forbid toggles are silent when block is absent", func(t *testing.T) {
		t.Parallel()
		rule := &check.FrontmatterRule{
			ForbidLayout:         true,
			ForbidSidebarCurrent: true,
		}
		if got := rule.CheckFile("test_resource", "p.md", []byte(content)); len(got) != 0 {
			t.Errorf("forbid_* must not fire without a frontmatter block, got: %v", resultMessages(got))
		}
	})
}

func TestFrontmatterRule_UnterminatedBlockTreatedAsAbsent(t *testing.T) {
	t.Parallel()

	// Opener with no closer — we prefer "missing" diagnostics over confusing
	// YAML parse errors for this case.
	content := "---\nsubcategory: \"Test\"\n\n# Resource: test\n"

	rule := &check.FrontmatterRule{RequireSubcategory: true}
	results := rule.CheckFile("test", "p.md", []byte(content))
	if len(results) != 1 {
		t.Fatalf("expected 1 result for unterminated frontmatter, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, "no frontmatter block") {
		t.Errorf("expected unterminated block to be reported as missing, got: %s", results[0].Message)
	}
}

func TestFrontmatterRule_MalformedYAML(t *testing.T) {
	t.Parallel()

	// Duplicate mapping key — rejected by yaml.v3 in strict mode. Use a value
	// that cannot be a mapping (a scalar) so Unmarshal has to error.
	content := "---\nnot a mapping, just a scalar\n---\n\n# body\n"

	rule := &check.FrontmatterRule{RequireSubcategory: true}
	results := rule.CheckFile("test", "p.md", []byte(content))
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 parse-error result, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, "parse error") {
		t.Errorf("expected parse-error diagnostic, got: %s", results[0].Message)
	}
}

func TestFrontmatterRule_CRLFLineEndings(t *testing.T) {
	t.Parallel()

	// Swap every \n for \r\n to simulate Windows-authored files.
	content := strings.ReplaceAll(validFrontmatter, "\n", "\r\n")

	rule := &check.FrontmatterRule{
		RequireSubcategory: true,
		RequirePageTitle:   true,
		RequireDescription: true,
		RequireLayout:      true,
	}
	if got := rule.CheckFile("test", "p.md", []byte(content)); len(got) != 0 {
		t.Fatalf("CRLF frontmatter should parse identically, got: %v", resultMessages(got))
	}
}

func TestFrontmatterRule_EmitsRuleAndResourceMetadata(t *testing.T) {
	t.Parallel()

	content := replaceFrontmatter(validFrontmatter, "subcategory: \"Test\"\n", "")
	rule := &check.FrontmatterRule{RequireSubcategory: true}
	results := rule.CheckFile("aws_example", "p.md", []byte(content))

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Rule != "frontmatter" {
		t.Errorf("Rule = %q, want %q", r.Rule, "frontmatter")
	}
	if r.Resource != "aws_example" {
		t.Errorf("Resource = %q, want %q", r.Resource, "aws_example")
	}
	if r.Severity != check.SeverityError {
		t.Errorf("Severity = %v, want SeverityError", r.Severity)
	}
}

// End-to-end fixture tests exercise real on-disk markdown files. The
// testdata/docs/r/instance.html.markdown fixture is the canonical "valid"
// doc; instance_forbidden_fields.html.markdown adds layout + sidebar_current;
// instance_no_frontmatter.html.markdown omits the block entirely.

func TestFrontmatterRule_ValidFixture(t *testing.T) {
	t.Parallel()

	content := mustReadFile(t, "../../testdata/docs/r/instance.html.markdown")
	rule := &check.FrontmatterRule{
		RequireSubcategory: true,
		RequirePageTitle:   true,
		RequireDescription: true,
	}

	if got := rule.CheckFile("test_instance", "instance.html.markdown", content); len(got) != 0 {
		t.Fatalf("valid fixture should pass, got: %v", resultMessages(got))
	}
}

func TestFrontmatterRule_ForbiddenFixture(t *testing.T) {
	t.Parallel()

	content := mustReadFile(t, "../../testdata/docs/r/instance_forbidden_fields.html.markdown")
	rule := &check.FrontmatterRule{
		ForbidLayout:         true,
		ForbidSidebarCurrent: true,
	}

	results := rule.CheckFile("test_forbidden", "instance_forbidden_fields.html.markdown", content)
	if len(results) != 2 {
		t.Fatalf("expected 2 results (layout + sidebar_current), got %d: %v", len(results), resultMessages(results))
	}
	if !anyContains(results, "layout") {
		t.Errorf("expected a result mentioning layout")
	}
	if !anyContains(results, "sidebar_current") {
		t.Errorf("expected a result mentioning sidebar_current")
	}
}

func TestFrontmatterRule_NoFrontmatterFixture(t *testing.T) {
	t.Parallel()

	content := mustReadFile(t, "../../testdata/docs/r/instance_no_frontmatter.html.markdown")
	rule := &check.FrontmatterRule{RequireSubcategory: true, RequirePageTitle: true}

	results := rule.CheckFile("test_no_frontmatter", "instance_no_frontmatter.html.markdown", content)
	if len(results) != 2 {
		t.Fatalf("expected 2 missing-frontmatter results, got %d: %v", len(results), resultMessages(results))
	}
	for _, r := range results {
		if !strings.Contains(r.Message, "no frontmatter block") {
			t.Errorf("expected no-frontmatter phrasing, got: %s", r.Message)
		}
	}
}

// --- helpers -------------------------------------------------------------

func assertMessage(t *testing.T, results []check.Result, wantSub string) {
	t.Helper()
	if wantSub == "" {
		if len(results) != 0 {
			t.Fatalf("expected 0 results, got %d: %v", len(results), resultMessages(results))
		}
		return
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, wantSub) {
		t.Fatalf("result message = %q, want substring %q", results[0].Message, wantSub)
	}
}

func anyContains(results []check.Result, sub string) bool {
	for _, r := range results {
		if strings.Contains(r.Message, sub) {
			return true
		}
	}
	return false
}

// replaceFrontmatter is a deliberate "dumb" mutator: the test fixtures are
// short enough that string replacement is clearer than parsing-then-emitting.
func replaceFrontmatter(source, old, new string) string {
	return strings.Replace(source, old, new, 1)
}

// insertIntoFrontmatter inserts a new "key: value\n" line immediately before
// the closing "---" of the frontmatter block. Keeps ordering deterministic
// across subtests that would otherwise race on map iteration.
func insertIntoFrontmatter(source, line string) string {
	const closer = "---\n"
	// Find the SECOND "---\n" (the closer); the first is the opener.
	first := strings.Index(source, closer)
	if first < 0 {
		return source
	}
	second := strings.Index(source[first+len(closer):], closer)
	if second < 0 {
		return source
	}
	insertAt := first + len(closer) + second
	return source[:insertAt] + line + source[insertAt:]
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %s", path, err)
	}
	return b
}
