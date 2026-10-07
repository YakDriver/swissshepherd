// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
)

func newProseCasing(t *testing.T, enforceCasing, ignoreWords []string) *check.ProseCasing {
	t.Helper()
	r, err := check.NewProseCasingRule(enforceCasing, ignoreWords, doc.DefaultHeadingTemplates(), false, check.SeverityWarning)
	if err != nil {
		t.Fatalf("NewProseCasingRule: %v", err)
	}
	return r
}

func runProseCasing(t *testing.T, enforceCasing, ignoreWords []string, content string) []check.Result {
	t.Helper()
	r := newProseCasing(t, enforceCasing, ignoreWords)
	return r.CheckFile(check.FileCheckContext{
		Resource: "aws_thing",
		Path:     "website/docs/r/thing.html.markdown",
		Content:  []byte(content),
	})
}

func TestProseCasing_ReportsWrongCasing(t *testing.T) {
	t.Parallel()

	got := runProseCasing(t, nil, nil, "The api format is shown below.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if got[0].Rule != "prose_casing" {
		t.Errorf("Rule = %q, want prose_casing", got[0].Rule)
	}
}

func TestProseCasing_CorrectlyCasedProducesNothing(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"The ID of the thing.",
		"Returns JSON.",
		"Uses HTTPS for the connection.",
	} {
		if got := runProseCasing(t, nil, nil, content); len(got) != 0 {
			t.Errorf("content %q: got %d findings, want 0: %+v", content, len(got), got)
		}
	}
}

func TestProseCasing_CanonicalNotLiteral(t *testing.T) {
	t.Parallel()

	// Both "Id" and "id" are wrong forms of the same canonical "ID": the
	// rule matches by canonical form, not by enumerating each wrong casing.
	for _, content := range []string{"The Id of the resource.", "The id of the resource."} {
		got := runProseCasing(t, nil, nil, content)
		if len(got) != 1 {
			t.Fatalf("content %q: got %d findings, want 1: %+v", content, len(got), got)
		}
		if !strings.Contains(got[0].Message, `"ID"`) {
			t.Errorf("content %q: message should recommend ID: %q", content, got[0].Message)
		}
	}
}

func TestProseCasing_MixedCaseCanonicalReproducedLetterForLetter(t *testing.T) {
	t.Parallel()

	got := runProseCasing(t, []string{"DynamoDB", "OAuth"}, nil, "Uses dynamodb and oauth for storage and auth.")
	if len(got) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(got), got)
	}
	var messages []string
	for _, r := range got {
		messages = append(messages, r.Message)
	}
	joined := strings.Join(messages, " | ")
	if !strings.Contains(joined, `"DynamoDB"`) {
		t.Errorf("expected a DynamoDB suggestion, got: %s", joined)
	}
	if !strings.Contains(joined, `"OAuth"`) {
		t.Errorf("expected an OAuth suggestion, got: %s", joined)
	}
}

func TestProseCasing_EnforceCasingOverridesDefault(t *testing.T) {
	t.Parallel()

	// "id" is in the default list as "ID"; overriding with a different
	// canonical casing replaces the default's casing for that word.
	got := runProseCasing(t, []string{"Id"}, nil, "The id of the resource.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, `"Id"`) {
		t.Errorf("message should recommend the overridden casing Id: %q", got[0].Message)
	}
}

func TestProseCasing_EmptyConfigStillUsesDefaultList(t *testing.T) {
	t.Parallel()

	// No enforce_casing supplied: the built-in default list still applies.
	// Unlike banned_glosses, this check is not a no-op without config.
	got := runProseCasing(t, nil, nil, "The arn is shown.")
	if len(got) != 0 {
		t.Fatalf("got %d findings for 'arn', want 0: ARN is not in the vendor-neutral default list: %+v", len(got), got)
	}
	got = runProseCasing(t, nil, nil, "The api is shown.")
	if len(got) != 1 {
		t.Fatalf("got %d findings for 'api', want 1: API is in the default list: %+v", len(got), got)
	}
}

func TestProseCasing_PluralsAndPossessives(t *testing.T) {
	t.Parallel()

	cases := []struct {
		content string
		want    string
	}{
		{"The cpus available.", "CPUs"},
		{"a cpu's performance", "CPU's"},
	}
	for _, tc := range cases {
		got := runProseCasing(t, nil, nil, tc.content)
		if len(got) != 1 {
			t.Fatalf("content %q: got %d findings, want 1: %+v", tc.content, len(got), got)
		}
		if !strings.Contains(got[0].Message, `"`+tc.want+`"`) {
			t.Errorf("content %q: message should recommend %q: %q", tc.content, tc.want, got[0].Message)
		}
	}
}

func TestProseCasing_TypographicApostropheFallsThroughToPlainBranch(t *testing.T) {
	t.Parallel()

	// A typographic apostrophe (U+2019) isn't the literal "cpu's" entry, so
	// it matches the plain "cpu" branch instead, which still yields the
	// right correction once applied: "cpu" -> "CPU" leaves "CPU’s".
	got := runProseCasing(t, nil, nil, "a cpu\u2019s performance")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, `"cpu"`) || !strings.Contains(got[0].Message, `"CPU"`) {
		t.Errorf("message should report plain cpu -> CPU: %q", got[0].Message)
	}
}

func TestProseCasing_SkipsCodeFencesAndURLs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
	}{
		{"inline code", "Set `api` in the field."},
		{"fenced code block", "```\napi = \"foo\"\n```"},
		{"tilde fenced block", "~~~terraform\napi = \"foo\"\n~~~"},
		{"markdown link url", "See [the docs](https://example.com/api-info)."},
		{"bare url", "See https://example.com/api-info for more."},
		{"frontmatter not skipped by default", "---\ndescription: api info\n---\n\nbody text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runProseCasing(t, nil, nil, tc.content)
			if tc.name == "frontmatter not skipped by default" {
				if len(got) != 1 {
					t.Errorf("content %q: got %d findings, want 1 (frontmatter scanned by default): %+v", tc.content, len(got), got)
				}
				return
			}
			if len(got) != 0 {
				t.Errorf("content %q: expected 0 findings, got %d: %+v", tc.content, len(got), got)
			}
		})
	}
}

func TestProseCasing_MismatchedFenceCharacterDoesNotCloseFence(t *testing.T) {
	t.Parallel()

	// A ~~~ line inside a ```-fenced block isn't a close: it's ordinary
	// fence content. Only a ``` line (same character, run at least as
	// long as the opener's) can close this fence. "api usage here"
	// remains inside the real fence throughout and must not be scanned.
	content := "```\nsome code\n~~~\napi usage here\n```"
	got := runProseCasing(t, nil, nil, content)
	if len(got) != 0 {
		t.Errorf("expected 0 findings (content stayed inside the ``` fence), got %d: %+v", len(got), got)
	}
}

func TestProseCasing_SkipFrontmatter(t *testing.T) {
	t.Parallel()

	content := "---\ndescription: api info\n---\n\nThe api is shown in prose.\n"
	r, err := check.NewProseCasingRule(nil, nil, doc.DefaultHeadingTemplates(), true, check.SeverityWarning)
	if err != nil {
		t.Fatalf("NewProseCasingRule: %v", err)
	}
	got := r.CheckFile(check.FileCheckContext{Resource: "aws_thing", Path: "p", Content: []byte(content)})
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1 (only the body api): %+v", len(got), got)
	}
	if got[0].Line != 5 {
		t.Errorf("Line = %d, want 5 (body)", got[0].Line)
	}
}

func TestProseCasing_UnterminatedCodeSpanSkipsLine(t *testing.T) {
	t.Parallel()

	// An odd number of backticks means maskUnscannable can't tell where the
	// span was meant to end, so the whole line is skipped rather than
	// scanned with the span unmasked.
	content := "Example: `api:aws:iam::cloudfront:user/foo"
	got := runProseCasing(t, nil, nil, content)
	if len(got) != 0 {
		t.Errorf("got %d findings, want 0 (unterminated code span): %+v", len(got), got)
	}
}

func TestProseCasing_GluedTokensNotReported(t *testing.T) {
	t.Parallel()

	cases := []string{
		"Uses execute-api for the gateway.",
		"Edit ~/.ssh/authorized_keys for login.",
		"See index.html for the page.",
		"Mount with 10.0.1.6@tcp as the target.",
		"Set core-site.xml for the cluster.",
		`Only "url" can be used here.`,
	}
	for _, content := range cases {
		if got := runProseCasing(t, nil, nil, content); len(got) != 0 {
			t.Errorf("content %q: expected 0 findings (glued token), got %d: %+v", content, len(got), got)
		}
	}
}

func TestProseCasing_SentenceFinalMatchIsReported(t *testing.T) {
	t.Parallel()

	// The glued-token dot-rule must not reject a sentence-final match: no
	// alphanumeric follows the "." so "api." is not a dotted compound.
	got := runProseCasing(t, nil, nil, "Configure access to the rest api.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
}

func TestProseCasing_ColonDelimitedLiteralsNotReported(t *testing.T) {
	t.Parallel()

	cases := []string{
		"The api:aws:ec2:us-east-1:123:instance/i-123 format is shown.",
		"Files are stored at s3://bucket/prefix for access.",
	}
	for _, content := range cases {
		if got := runProseCasing(t, nil, nil, content); len(got) != 0 {
			t.Errorf("content %q: expected 0 findings (colon literal), got %d: %+v", content, len(got), got)
		}
	}
}

func TestProseCasing_EmphasisMarkersStillReported(t *testing.T) {
	t.Parallel()

	// Markdown emphasis markers don't need special handling: emphasized
	// prose is still prose. "*" is not a word character, so \b already
	// finds the match.
	got := runProseCasing(t, nil, nil, "The *api* of the thing.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
}

func TestProseCasing_GluedUnderscoreNotReported(t *testing.T) {
	t.Parallel()

	// A following underscore (a word character) already prevents a \b
	// match; this is also structurally a glued compound.
	got := runProseCasing(t, nil, nil, "The api_prefix setting controls this.")
	if len(got) != 0 {
		t.Errorf("got %d findings, want 0 (arn_prefix is glued), got %+v", len(got), got)
	}
}

func TestProseCasing_BlockStyleHeadingGetsBackticksOnlyMessage(t *testing.T) {
	t.Parallel()

	// A bare, unbackticked {Block}-style heading: capitalizing "tls" would
	// stop isSnakeCaseSegment matching, breaking the heading's resolution.
	// Only the backticks fix is offered.
	content := "### tls Block\n\nSome text."
	got := runProseCasing(t, nil, nil, content)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if strings.Contains(got[0].Message, `"TLS"`) {
		t.Errorf("backticks-only message should not suggest capitalizing: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "backticks") {
		t.Errorf("message should mention backticks: %q", got[0].Message)
	}
}

func TestProseCasing_EmphasizedBlockStyleHeadingGetsBackticksOnlyMessage(t *testing.T) {
	t.Parallel()

	// The heading's inline markup (the emphasis around "tls") must be
	// resolved to plain text the same way the real parser resolves it
	// before testing whether capitalizing would break resolution.
	// Stripping only the leading "#"s and leaving "*tls*" in place would
	// make the heading look unresolvable (before == ""), wrongly offering
	// the unsafe capitalize fix.
	content := "### *tls* Block\n\nSome text."
	got := runProseCasing(t, nil, nil, content)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if strings.Contains(got[0].Message, `"TLS"`) {
		t.Errorf("backticks-only message should not suggest capitalizing: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "backticks") {
		t.Errorf("message should mention backticks: %q", got[0].Message)
	}
}

func TestProseCasing_TitleStyleHeadingGetsCapitalizationFix(t *testing.T) {
	t.Parallel()

	// A {Title}-style heading resolves to the same block name whether or
	// not the word is capitalized (titleToSnake lowercases first), so the
	// normal two-fix message applies.
	content := "### Ip Filter Argument Reference\n\nSome text."
	got := runProseCasing(t, nil, nil, content)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, `"IP"`) {
		t.Errorf("message should suggest capitalizing to IP: %q", got[0].Message)
	}
}

func TestProseCasing_ProseHeadingGetsCapitalizationFix(t *testing.T) {
	t.Parallel()

	// A heading that doesn't resolve to any block either way (before and
	// after are both "") gets the normal two-fix message.
	content := "### Usage with subnet id\n\nSome text."
	got := runProseCasing(t, nil, nil, content)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, `"ID"`) {
		t.Errorf("message should suggest capitalizing to ID: %q", got[0].Message)
	}
}

func TestProseCasing_EnumValueWithoutBackticksIsKnownResidual(t *testing.T) {
	t.Parallel()

	// No structural signal separates this from prose; it is a known
	// residual false positive, addressed by the message's backticks
	// suggestion rather than suppressed.
	got := runProseCasing(t, nil, nil, "Valid values are cpu and memory.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1 (known residual): %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "backticks") {
		t.Errorf("message should suggest backticks: %q", got[0].Message)
	}
}

func TestProseCasing_IgnoreWordsBareEntrySilencesEverywhere(t *testing.T) {
	t.Parallel()

	got := runProseCasing(t, nil, []string{"cpu"}, "The cpu usage and the api endpoint.")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1 (only api, cpu ignored): %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, `"api"`) {
		t.Errorf("remaining finding should be about api: %q", got[0].Message)
	}
}

func TestProseCasing_IgnoreWordsTypeQualifiedScopesToOneTarget(t *testing.T) {
	t.Parallel()

	r := newProseCasing(t, nil, []string{"aws_emr_cluster/ssh"})
	resourceType := &config.Type{Name: "resource"}

	silenced := r.CheckFile(check.FileCheckContext{
		Resource: "aws_emr_cluster", Type: resourceType, Path: "p",
		Content: []byte("Use ssh to connect."),
	})
	if len(silenced) != 0 {
		t.Errorf("aws_emr_cluster: got %d findings, want 0 (ignored for this target): %+v", len(silenced), silenced)
	}

	reported := r.CheckFile(check.FileCheckContext{
		Resource: "aws_other", Type: resourceType, Path: "p",
		Content: []byte("Use ssh to connect."),
	})
	if len(reported) != 1 {
		t.Errorf("aws_other: got %d findings, want 1 (not ignored for this target): %+v", len(reported), reported)
	}
}

func TestProseCasing_SeverityIsConfigurable(t *testing.T) {
	t.Parallel()

	for _, sev := range []check.Severity{check.SeverityError, check.SeverityWarning} {
		r, err := check.NewProseCasingRule(nil, nil, doc.DefaultHeadingTemplates(), false, sev)
		if err != nil {
			t.Fatalf("NewProseCasingRule: %v", err)
		}
		got := r.CheckFile(check.FileCheckContext{Resource: "aws_thing", Path: "p", Content: []byte("The api is shown.")})
		if len(got) != 1 {
			t.Fatalf("sev %v: got %d findings, want 1", sev, len(got))
		}
		if got[0].Severity != sev {
			t.Errorf("Severity = %v, want %v", got[0].Severity, sev)
		}
	}
}

func TestProseCasing_ScopingViaCheckConfig(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:          "prose_casing",
		IgnoreTargets: []string{"aws_thing"},
	}
	if cc.AppliesTo("aws_thing", "resource") {
		t.Error("aws_thing is in ignore_targets; check should not apply")
	}
	if !cc.AppliesTo("aws_other", "resource") {
		t.Error("aws_other is not ignored; check should apply")
	}
}

func TestProseCasing_EnforceCasingRejectsAlreadyLowercase(t *testing.T) {
	t.Parallel()

	_, err := check.NewProseCasingRule([]string{"arn"}, nil, doc.DefaultHeadingTemplates(), false, check.SeverityWarning)
	if err == nil {
		t.Fatal("expected an error for an enforce_casing entry equal to its own lowercase form")
	}
}

func TestProseCasing_Determinism(t *testing.T) {
	t.Parallel()

	content := "The api, the url, and the json payload are all shown on one line."
	r := newProseCasing(t, nil, nil)
	ctx := check.FileCheckContext{Resource: "aws_thing", Path: "p", Content: []byte(content)}

	first := r.CheckFile(ctx)
	for i := range 20 {
		got := r.CheckFile(ctx)
		if len(got) != len(first) {
			t.Fatalf("run %d: got %d findings, want %d (first run)", i, len(got), len(first))
		}
		for j := range got {
			if got[j].Message != first[j].Message || got[j].Line != first[j].Line {
				t.Fatalf("run %d: result %d differs: %+v vs %+v", i, j, got[j], first[j])
			}
		}
	}
}
