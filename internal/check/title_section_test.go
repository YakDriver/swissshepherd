// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
)

// parseDoc is a tiny helper so each subtest can craft its own markdown without
// an on-disk fixture. Keeping the helper inline with the test file emphasizes
// that every case below is derived from a literal string — easy to eyeball,
// easy to amend.
func parseDoc(t *testing.T, source string) *doc.Document {
	t.Helper()
	d, err := doc.Parse([]byte(source), "test_instance")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return d
}

func TestTitleSectionRule_Valid(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"resource":      "# Resource: test_instance\n\nBody.\n",
		"data source":   "# Data Source: test_instance\n\nBody.\n",
		"ephemeral":     "# Ephemeral: test_instance\n\nBody.\n",
		"function":      "# Function: test_fn\n\nBody.\n",
		"list resource": "# List Resource: test_list\n\nBody.\n",
		"action":        "# Action: test_action\n\nBody.\n",
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rule := &check.TitleSectionRule{}
			results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: parseDoc(t, src)})
			if len(results) != 0 {
				t.Fatalf("expected 0 results for valid title, got %d: %v", len(results), resultMessages(results))
			}
		})
	}
}

func TestTitleSectionRule_MissingTitle(t *testing.T) {
	t.Parallel()

	// No level-1 heading at all.
	source := "## Example Usage\n\nBody.\n"
	rule := &check.TitleSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_example", Schema: nil, Doc: parseDoc(t, source)})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, "missing title section") {
		t.Errorf("unexpected message: %s", results[0].Message)
	}
	if !strings.Contains(results[0].Message, "aws_example") {
		t.Errorf("message should embed resource name for actionable output, got: %s", results[0].Message)
	}
}

func TestTitleSectionRule_BadPrefix(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"no prefix at all":      "# AWS S3 Bucket\n\nBody.\n",
		"lowercase kind":        "# resource: test_instance\n\nBody.\n",
		"wrong punctuation":     "# Resource - test_instance\n\nBody.\n",
		"plural resources":      "# Resources: test_instance\n\nBody.\n",
		"typo in kind":          "# Resourse: test_instance\n\nBody.\n",
		"missing space after :": "# Resource:test_instance\n\nBody.\n",
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rule := &check.TitleSectionRule{}
			results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: parseDoc(t, src)})

			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d: %v", len(results), resultMessages(results))
			}
			if !strings.Contains(results[0].Message, "should have one of these prefixes") {
				t.Errorf("expected prefix-mismatch message, got: %s", results[0].Message)
			}
		})
	}
}

func TestTitleSectionRule_CodeBlockInTitle(t *testing.T) {
	t.Parallel()

	source := "# Resource: test_instance\n\nBody.\n\n" +
		"```terraform\nresource \"test_instance\" \"x\" {}\n```\n\n" +
		"## Example Usage\n\nBody.\n"

	rule := &check.TitleSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: parseDoc(t, source)})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, "should not contain code blocks") {
		t.Errorf("unexpected message: %s", results[0].Message)
	}
}

func TestTitleSectionRule_MultipleFailures(t *testing.T) {
	t.Parallel()

	// Bad prefix AND code block in title. Expect two results so a single run
	// surfaces every fixable problem — the rule must not short-circuit.
	source := "# AWS Thing\n\nBody.\n\n" +
		"```terraform\nresource \"x\" \"y\" {}\n```\n\n" +
		"## Example Usage\n\nBody.\n"

	rule := &check.TitleSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: parseDoc(t, source)})

	if len(results) != 2 {
		t.Fatalf("expected 2 aggregated results, got %d: %v", len(results), resultMessages(results))
	}
	if !anyContains(results, "prefixes") {
		t.Errorf("expected a prefix-mismatch finding, got: %v", resultMessages(results))
	}
	if !anyContains(results, "code blocks") {
		t.Errorf("expected a code-blocks-in-title finding, got: %v", resultMessages(results))
	}
}

func TestTitleSectionRule_CustomAllowPrefixes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		prefixes []string
		source   string
		wantPass bool
	}{
		"custom kind passes with custom list": {
			prefixes: []string{"Widget"},
			source:   "# Widget: my_widget\n\nBody.\n",
			wantPass: true,
		},
		"default kind fails with custom list": {
			prefixes: []string{"Widget"},
			source:   "# Resource: test_instance\n\nBody.\n",
			wantPass: false,
		},
		"empty custom list falls back to default": {
			prefixes: []string{},
			source:   "# Resource: test_instance\n\nBody.\n",
			wantPass: true,
		},
		"nil custom list falls back to default": {
			prefixes: nil,
			source:   "# Data Source: test_instance\n\nBody.\n",
			wantPass: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rule := &check.TitleSectionRule{AllowPrefixes: tt.prefixes}
			results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: parseDoc(t, tt.source)})

			if tt.wantPass && len(results) != 0 {
				t.Fatalf("expected pass, got: %v", resultMessages(results))
			}
			if !tt.wantPass && len(results) != 1 {
				t.Fatalf("expected 1 failure, got %d: %v", len(results), resultMessages(results))
			}
		})
	}
}

// TestTitleSectionRule_DefaultPrefixesMatchesTfproviderdocs keeps the default
// list synchronized with tfproviderdocs so migrating providers see no verdict
// churn. Changing the default is a breaking compatibility choice — this test
// forces that decision to be explicit.
func TestTitleSectionRule_DefaultPrefixesMatchesTfproviderdocs(t *testing.T) {
	t.Parallel()

	want := []string{
		"Action",
		"Data Source",
		"Ephemeral",
		"Function",
		"List Resource",
		"Resource",
	}
	got := check.DefaultTitleSectionPrefixes
	if !slices.Equal(got, want) {
		t.Errorf("DefaultTitleSectionPrefixes = %v, want %v", got, want)
	}
}

func TestTitleSectionRule_EmitsRuleAndResourceMetadata(t *testing.T) {
	t.Parallel()

	source := "# AWS S3 Bucket\n\nBody.\n"
	rule := &check.TitleSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_s3_bucket", Schema: nil, Doc: parseDoc(t, source)})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Rule != "title_section" {
		t.Errorf("Rule = %q, want %q", r.Rule, "title_section")
	}
	if r.Resource != "aws_s3_bucket" {
		t.Errorf("Resource = %q, want %q", r.Resource, "aws_s3_bucket")
	}
	if r.Severity != check.SeverityError {
		t.Errorf("Severity = %v, want SeverityError", r.Severity)
	}
}

// Fixture-level integration tests — mirrors the pattern phase 1 established
// for FrontmatterRule. Each fixture reads from testdata/docs/r/ and asserts
// end-to-end: file → Parse → Rule.Check → Result.

func TestTitleSectionRule_ValidFixture(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	rule := &check.TitleSectionRule{}
	if got := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d}); len(got) != 0 {
		t.Fatalf("valid fixture should pass, got: %v", resultMessages(got))
	}
}

func TestTitleSectionRule_BadPrefixFixture(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_bad_title_prefix.html.markdown")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	rule := &check.TitleSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, "prefixes") {
		t.Errorf("unexpected message: %s", results[0].Message)
	}
}

func TestTitleSectionRule_CodeBlockFixture(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_title_with_code.html.markdown")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	rule := &check.TitleSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, "code blocks") {
		t.Errorf("unexpected message: %s", results[0].Message)
	}
}

func TestTitleSectionRule_NoTitleFixture(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_no_title.html.markdown")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	rule := &check.TitleSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), resultMessages(results))
	}
	if !strings.Contains(results[0].Message, "missing title section") {
		t.Errorf("unexpected message: %s", results[0].Message)
	}
}
