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

// anchorResults runs only the anchors sub-check by disabling the others, so
// assertions are not polluted by coverage/format/label findings.
func anchorResults(t *testing.T, src string) []check.Result {
	t.Helper()
	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", anchorTemplates)
	if err != nil {
		t.Fatal(err)
	}
	off := false
	rule := &check.SchemaDocsRule{
		Coverage: &off, Ordering: &off, Description: &off, Heading: &off,
		Format: &off, Labels: &off, Byline: &off, Deprecated: &off,
	}
	return rule.Check(check.CheckContext{Resource: "aws_thing", Doc: d})
}

func anchorFindings(results []check.Result) []check.Result {
	var out []check.Result
	for _, r := range results {
		if strings.Contains(r.Message, "does not resolve to any heading") {
			out = append(out, r)
		}
	}
	return out
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

## Attribute Reference

* ` + "`arn`" + ` - ARN.
`
	findings := anchorFindings(anchorResults(t, src))
	if len(findings) != 1 {
		t.Fatalf("want 1 dead-anchor finding, got %d: %+v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Message, "#ec2_configuration") {
		t.Errorf("finding should name the dead fragment #ec2_configuration: %q", findings[0].Message)
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
	if findings := anchorFindings(anchorResults(t, src)); len(findings) != 0 {
		t.Errorf("valid link must not be flagged: %+v", findings)
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
	if findings := anchorFindings(anchorResults(t, src)); len(findings) != 0 {
		t.Errorf("section-heading link must resolve: %+v", findings)
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
	if findings := anchorFindings(anchorResults(t, src)); len(findings) != 0 {
		t.Errorf("link to duplicate-suffixed anchor must resolve: %+v", findings)
	}
}

// The same dead fragment reused on multiple bullets is reported once.
func TestAnchors_DuplicateDeadFragmentReportedOnce(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`a`" + ` - (Optional) See [x](#missing).
* ` + "`b`" + ` - (Optional) See [y](#missing).
`
	if findings := anchorFindings(anchorResults(t, src)); len(findings) != 1 {
		t.Errorf("want 1 deduped finding, got %d: %+v", len(findings), findings)
	}
}

// The sub-check is opt-out via the Anchors toggle.
func TestAnchors_DisabledToggleSuppresses(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`a`" + ` - (Optional) See [x](#missing).
`
	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", anchorTemplates)
	if err != nil {
		t.Fatal(err)
	}
	off := false
	rule := &check.SchemaDocsRule{
		Coverage: &off, Ordering: &off, Description: &off, Heading: &off,
		Format: &off, Labels: &off, Byline: &off, Deprecated: &off, Anchors: &off,
	}
	if findings := anchorFindings(rule.Check(check.CheckContext{Resource: "aws_thing", Doc: d})); len(findings) != 0 {
		t.Errorf("disabled anchors check must emit nothing: %+v", findings)
	}
}
