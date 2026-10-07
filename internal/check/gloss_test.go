// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
)

func runGloss(glosses map[string]string, content string) []check.Result {
	rule := check.NewGlossRule(glosses, false, check.SeverityWarning)
	return rule.CheckFile(check.FileCheckContext{
		Resource: "aws_thing",
		Path:     "website/docs/r/thing.html.markdown",
		Content:  []byte(content),
	})
}

func TestGlossRule_GlossFormAndStandalone(t *testing.T) {
	t.Parallel()

	glosses := map[string]string{"Amazon Resource Name": "ARN"}

	cases := []struct {
		name    string
		content string
		want    int // number of findings
	}{
		{"gloss form", "The Amazon Resource Name (ARN) of the thing.", 1},
		{"standalone phrase", "The Amazon Resource Name of the thing.", 1},
		{"phrase without amazon prefix", "The Resource Name of the thing.", 0}, // "Resource Name" alone is not the phrase
		{"bare abbreviation is fine", "The ARN of the thing.", 0},
		{"two on separate lines", "The Amazon Resource Name (ARN).\nAnother Amazon Resource Name here.", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runGloss(glosses, tc.content)
			if len(got) != tc.want {
				t.Fatalf("content %q: got %d findings, want %d: %+v", tc.content, len(got), tc.want, got)
			}
			for _, r := range got {
				if r.Rule != "banned_glosses" {
					t.Errorf("rule = %q, want banned_glosses", r.Rule)
				}
				if r.Severity != check.SeverityWarning {
					t.Errorf("severity = %v, want warning (from runGloss)", r.Severity)
				}
				if !strings.Contains(r.Message, `"ARN"`) {
					t.Errorf("message should recommend ARN: %q", r.Message)
				}
			}
		})
	}
}

func TestGlossRule_ReportsLineNumbers(t *testing.T) {
	t.Parallel()

	content := "line one\nline two\nThe Amazon Resource Name (ARN) here.\nline four"
	got := runGloss(map[string]string{"Amazon Resource Name": "ARN"}, content)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].Line != 3 {
		t.Errorf("Line = %d, want 3", got[0].Line)
	}
}

func TestGlossRule_SkipsCodeAndURLs(t *testing.T) {
	t.Parallel()

	glosses := map[string]string{"Amazon Resource Name": "ARN"}

	cases := []struct {
		name    string
		content string
	}{
		{"inline code", "Set `Amazon Resource Name` in the field."},
		{"fenced code block", "```\nAmazon Resource Name (ARN)\n```"},
		{"tilde fenced block", "~~~terraform\nAmazon Resource Name\n~~~"},
		{"markdown link url", "See [the docs](https://example.com/Amazon-Resource-Name)."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runGloss(glosses, tc.content); len(got) != 0 {
				t.Errorf("content %q: expected 0 findings, got %d: %+v", tc.content, len(got), got)
			}
		})
	}
}

func TestGlossRule_MismatchedFenceCharacterDoesNotCloseFence(t *testing.T) {
	t.Parallel()

	// A ~~~ line inside a ```-fenced block isn't a close: it's ordinary
	// fence content. Only a ``` line (same character, run at least as
	// long as the opener's) can close this fence. The banned phrase on
	// the middle line remains inside the real fence throughout and must
	// not be scanned.
	glosses := map[string]string{"Amazon Resource Name": "ARN"}
	content := "```\nsome code\n~~~\nAmazon Resource Name\n```"
	if got := runGloss(glosses, content); len(got) != 0 {
		t.Errorf("expected 0 findings (content stayed inside the ``` fence), got %d: %+v", len(got), got)
	}
}

func TestGlossRule_OptionalPrefixInsideParens(t *testing.T) {
	t.Parallel()

	// Phrase configured without the Amazon prefix; text uses it, and the
	// gloss carries an "Amazon " prefix inside the parentheses.
	got := runGloss(map[string]string{"Simple Storage Service": "S3"},
		"Uses Amazon Simple Storage Service (Amazon S3) for storage.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, `"S3"`) {
		t.Errorf("should recommend S3: %q", got[0].Message)
	}
}

func TestGlossRule_Plural(t *testing.T) {
	t.Parallel()

	got := runGloss(map[string]string{"Amazon Resource Name": "ARN"},
		"A list of Amazon Resource Names for the resources.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, `"ARNs"`) {
		t.Errorf("plural phrase should recommend ARNs: %q", got[0].Message)
	}
}

func TestGlossRule_SkipFrontmatter(t *testing.T) {
	t.Parallel()

	glosses := map[string]string{
		"Elastic Compute Cloud": "EC2",
		"Amazon Resource Name":  "ARN",
	}
	doc := "---\n" +
		"subcategory: \"EC2 (Elastic Compute Cloud)\"\n" +
		"page_title: \"AWS\"\n" +
		"---\n" +
		"\n" +
		"The Amazon Resource Name (ARN) of the thing.\n"

	// Default (scan everything): frontmatter phrase + body gloss = 2.
	if got := check.NewGlossRule(glosses, false, check.SeverityError).CheckFile(check.FileCheckContext{
		Resource: "aws_thing", Path: "p", Content: []byte(doc),
	}); len(got) != 2 {
		t.Fatalf("scan-all: got %d findings, want 2: %+v", len(got), got)
	}

	// SkipFrontmatter: only the body gloss remains.
	got := check.NewGlossRule(glosses, true, check.SeverityError).CheckFile(check.FileCheckContext{
		Resource: "aws_thing", Path: "p", Content: []byte(doc),
	})
	if len(got) != 1 {
		t.Fatalf("skip-frontmatter: got %d findings, want 1: %+v", len(got), got)
	}
	if got[0].Line != 6 {
		t.Errorf("remaining finding Line = %d, want 6 (body)", got[0].Line)
	}
}

func TestGlossRule_SeverityIsConfigurable(t *testing.T) {
	t.Parallel()

	content := "The Amazon Resource Name (ARN) of the thing."
	glosses := map[string]string{"Amazon Resource Name": "ARN"}

	for _, sev := range []check.Severity{check.SeverityError, check.SeverityWarning} {
		got := check.NewGlossRule(glosses, false, sev).CheckFile(check.FileCheckContext{
			Resource: "aws_thing", Path: "p", Content: []byte(content),
		})
		if len(got) != 1 {
			t.Fatalf("sev %v: got %d findings, want 1", sev, len(got))
		}
		if got[0].Severity != sev {
			t.Errorf("Severity = %v, want %v", got[0].Severity, sev)
		}
	}
}

func TestGlossRule_EmptyConfigIsNoop(t *testing.T) {
	t.Parallel()

	content := "The Amazon Resource Name (ARN) of the thing."
	for _, m := range []map[string]string{
		nil,
		{},
		{"": "ARN"},                  // empty phrase skipped
		{"Amazon Resource Name": ""}, // empty abbreviation skipped
	} {
		if got := runGloss(m, content); len(got) != 0 {
			t.Errorf("map %v: expected no findings, got %d", m, len(got))
		}
	}
}

// TestGlossRule_ScopingViaCheckConfig confirms the rule participates in the
// standard per-check scoping: its name matches the config key the Runner uses
// with AppliesTo, so ignore_targets/prefixes/types apply like every other check.
func TestGlossRule_ScopingViaCheckConfig(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:          check.NewGlossRule(map[string]string{"Amazon Resource Name": "ARN"}, false, check.SeverityWarning).Name(),
		IgnoreTargets: []string{"aws_thing"},
	}
	if cc.AppliesTo("aws_thing", "resource") {
		t.Error("aws_thing is in ignore_targets; check should not apply")
	}
	if !cc.AppliesTo("aws_other", "resource") {
		t.Error("aws_other is not ignored; check should apply")
	}
}

// The multi-entry weak-config gloss map, mirroring
// terraform-provider-aws/.ci/swissshepherd-weak.hcl. Used to exercise the
// combined pre-filter gate with realistic entry counts.
var weakGlosses = map[string]string{
	"Amazon Machine Image": "AMI", "Amazon Resource Name": "ARN", "Amazon Resource Names": "ARNs",
	"Application Programming Interface": "API", "Central Processing Unit": "CPU",
	"Command-Line Interface": "CLI", "Domain Name System": "DNS", "Elastic Compute Cloud": "EC2",
	"Extensible Markup Language": "XML", "Graphics Processing Unit": "GPU",
	"HyperText Markup Language": "HTML", "Hypertext Transfer Protocol": "HTTP",
	"Internet Protocol": "IP", "JavaScript Object Notation": "JSON", "Key Management Service": "KMS",
	"Relational Database Service": "RDS", "Simple Storage Service": "S3", "Software Development Kit": "SDK",
	"Structured Query Language": "SQL", "Transmission Control Protocol": "TCP",
	"Transport Layer Security": "TLS", "Unicode Transformation Format": "UTF",
	"Uniform Resource Identifier": "URI", "Uniform Resource Locator": "URL",
	"Universal Serial Bus": "USB", "Virtual Private Cloud": "VPC", "Virtual Private Network": "VPN",
	"YAML Ain't Markup Language": "YAML",
}

// TestGlossRule_GateEquivalence is a regression guard for the combined
// pre-filter optimization: gating (fast reject) must not change which lines
// produce findings. Lines with no phrase are skipped; lines with a phrase in
// prose are reported; lines where the only phrase sits in code/URL are gated
// in but masked back out to zero findings. Verified against the full weak
// gloss map so alternation ordering cannot mask an entry.
func TestGlossRule_GateEquivalence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"no phrase at all", "This line mentions an ARN and a VPC, both fine.", 0},
		{"prose phrase", "The Amazon Resource Name (ARN) of the thing.", 1},
		{"plural distinct entry", "A list of Amazon Resource Names here.", 2}, // matches both singular and plural entries
		{"phrase only in code", "Set `Domain Name System` in the field.", 0},
		{"phrase only in url", "See [x](https://example.com/Virtual-Private-Cloud).", 0},
		{"mixed: code masked, prose live", "`Simple Storage Service` but Transport Layer Security too.", 1},
		{"multiple distinct phrases one line", "The Internet Protocol and the Key Management Service.", 2},
		{"near-miss substring", "The Resource Name and the Storage Service (no vendor prefix).", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runGloss(weakGlosses, tc.content); len(got) != tc.want {
				t.Errorf("content %q: got %d findings, want %d: %+v", tc.content, len(got), tc.want, got)
			}
		})
	}
}
