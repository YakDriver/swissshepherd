// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
)

var anchorTemplates = doc.HeadingTemplates{"`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

func anchorResults(t *testing.T, src string) []check.Result {
	t.Helper()
	return anchorResultsSev(t, src, check.SeverityError)
}

func anchorResultsSev(t *testing.T, src string, sev check.Severity) []check.Result {
	t.Helper()
	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", anchorTemplates)
	if err != nil {
		t.Fatal(err)
	}
	return check.NewAnchorsRule(sev).Check(check.CheckContext{Resource: "aws_thing", Doc: d})
}

// A "See below" link left pointing at the old slug after a heading was renamed
// to "`name` Block" (slug "name-block") is a dead link and must be flagged.
func TestAnchors_DeadLinkAfterHeadingRename(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`ec2_configuration`" + ` - (Optional) Config. See [` + "`ec2_configuration`" + `](#ec2_configuration) below.

### ` + "`ec2_configuration`" + ` Block

* ` + "`image_type`" + ` - (Optional) Image type.
`
	results := anchorResults(t, src)
	if len(results) != 1 {
		t.Fatalf("want 1 dead-anchor finding, got %d: %+v", len(results), results)
	}
	if !strings.Contains(results[0].Message, "#ec2_configuration") {
		t.Errorf("finding should name the dead fragment: %q", results[0].Message)
	}
	if results[0].Severity != check.SeverityError {
		t.Errorf("default severity should be error, got %v", results[0].Severity)
	}
}

// A link whose fragment matches the generated heading slug ("#name-block")
// resolves and must not be flagged.
func TestAnchors_ValidLinkResolves(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`ec2_configuration`" + ` - (Optional) Config. See [` + "`ec2_configuration`" + `](#ec2_configuration-block) below.

### ` + "`ec2_configuration`" + ` Block

* ` + "`image_type`" + ` - (Optional) Image type.
`
	if results := anchorResults(t, src); len(results) != 0 {
		t.Errorf("valid link must not be flagged: %+v", results)
	}
}

// Fragments must be validated everywhere in the document, not just in
// Argument/Attribute list bullets. A dead link in an Example Usage paragraph
// and in a callout must both be caught.
func TestAnchors_ValidatedOutsideArgAttrLists(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Example Usage

See the [network configuration](#missing-network) for details.

-> **Note:** Review the [tags section](#missing-tags) before applying.

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
`
	results := anchorResults(t, src)
	frags := map[string]bool{}
	for _, r := range results {
		for _, f := range []string{"#missing-network", "#missing-tags"} {
			if strings.Contains(r.Message, f) {
				frags[f] = true
			}
		}
	}
	if !frags["#missing-network"] || !frags["#missing-tags"] {
		t.Errorf("expected dead links in Example Usage prose and callout to be caught; got: %+v", results)
	}
}

// Links to top-level section headings (e.g. "#attribute-reference") resolve.
func TestAnchors_SectionHeadingLinkResolves(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`size`" + ` - (Optional) See the [attributes](#attribute-reference) section.

## Attribute Reference

* ` + "`arn`" + ` - ARN.
`
	if results := anchorResults(t, src); len(results) != 0 {
		t.Errorf("section-heading link must resolve: %+v", results)
	}
}

// GitHub appends -1, -2 to duplicate heading slugs; a link to the suffixed
// anchor must resolve.
func TestAnchors_DuplicateHeadingSuffixResolves(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`grpc`" + ` - (Optional) See [second](#grpc-block-1).

### ` + "`grpc`" + ` Block

* ` + "`a`" + ` - (Optional) A.

### ` + "`grpc`" + ` Block

* ` + "`b`" + ` - (Optional) B.
`
	if results := anchorResults(t, src); len(results) != 0 {
		t.Errorf("link to duplicate-suffixed anchor must resolve: %+v", results)
	}
}

// The same dead fragment reused across the document is reported once.
func TestAnchors_DuplicateDeadFragmentReportedOnce(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`a`" + ` - (Optional) See [x](#missing).
* ` + "`b`" + ` - (Optional) See [y](#missing).
`
	if results := anchorResults(t, src); len(results) != 1 {
		t.Errorf("want 1 deduped finding, got %d: %+v", len(results), results)
	}
}

// Severity is configurable; a warning-configured rule emits warnings.
func TestAnchors_SeverityConfigurable(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`a`" + ` - (Optional) See [x](#missing).
`
	results := anchorResultsSev(t, src, check.SeverityWarning)
	if len(results) != 1 {
		t.Fatalf("want 1 finding, got %d", len(results))
	}
	if results[0].Severity != check.SeverityWarning {
		t.Errorf("severity = %v, want warning", results[0].Severity)
	}
}

// Multiple in-page links in a single bullet are each validated — the whole-AST
// collection is not limited to the first link per bullet.
func TestAnchors_MultipleLinksPerBulletAllChecked(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`a`" + ` - (Optional) See [A](#missing-a) and [B](#missing-b).
`
	results := anchorResults(t, src)
	got := map[string]bool{}
	for _, r := range results {
		for _, f := range []string{"#missing-a", "#missing-b"} {
			if strings.Contains(r.Message, f) {
				got[f] = true
			}
		}
	}
	if !got["#missing-a"] || !got["#missing-b"] {
		t.Errorf("both dead fragments in one bullet must be reported; got: %+v", results)
	}
}

// A link to a non-ASCII heading anchor resolves (Unicode-aware slug).
func TestAnchors_UnicodeHeadingLinkResolves(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Über Configuration

See the [über section](#über-configuration).

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
`
	if results := anchorResults(t, src); len(results) != 0 {
		t.Errorf("link to Unicode heading must resolve: %+v", results)
	}
}

// A link to a heading nested inside a list item resolves (heading slugs are
// gathered by a full-tree walk).
func TestAnchors_LinkToHeadingInsideListResolves(t *testing.T) {
	t.Parallel()

	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* see [nested](#nested-heading)\n\n    ### Nested Heading\n\n    detail\n"
	if results := anchorResults(t, src); len(results) != 0 {
		t.Errorf("link to heading nested in a list must resolve: %+v", results)
	}
}

// External and cross-file links are out of scope (only "#..." fragments count).
func TestAnchors_ExternalAndCrossFileLinksIgnored(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`a`" + ` - (Optional) See [aws](https://aws.amazon.com/vpc) and [other](other.html#frag).
`
	if results := anchorResults(t, src); len(results) != 0 {
		t.Errorf("external/cross-file links must be ignored: %+v", results)
	}
}
