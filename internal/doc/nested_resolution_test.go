// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

func blkAttrNames(b *doc.DocBlock) []string {
	if b == nil {
		return nil
	}
	names := make([]string, 0, len(b.Attributes))
	for _, a := range b.Attributes {
		names = append(names, a.Name)
	}
	return names
}

// TestNestedBlockLeadIn covers the legacy prose block introducer.
func TestNestedBlockLeadIn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"The `catalog_properties[0].data_lake_access_properties[0]` block also exports:", "catalog_properties.data_lake_access_properties", true},
		{"The `parent[0].child[0]` block supports the following:", "parent.child", true},
		{"The `foo` block supports:", "", false},      // single-level: use a heading
		{"The `foo` block is deprecated.", "", false}, // no supports/exports + colon
		{"This resource exports the following attributes:", "", false},
		{"Some prose about a block.", "", false},
	}
	for _, c := range cases {
		got, ok := doc.NestedBlockLeadIn(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("NestedBlockLeadIn(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestParse_ProseLeadInAttachesToPath: bullets after a prose lead-in are
// attributed to the dot-path block, not the root (issue #53).
func TestParse_ProseLeadInAttachesToPath(t *testing.T) {
	t.Parallel()

	md := "## Attribute Reference\n\n" +
		"* `id` - The ID.\n\n" +
		"The `catalog_properties[0].data_lake_access_properties[0]` block also exports:\n\n" +
		"* `managed_workgroup_name` - Managed workgroup name.\n" +
		"* `status_message` - Status message.\n"

	d, err := doc.ParseWithOptions([]byte(md), "t", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	b := d.AttributeBlocks["catalog_properties.data_lake_access_properties"]
	if b == nil {
		t.Fatal(`expected block keyed "catalog_properties.data_lake_access_properties"`)
	}
	got := blkAttrNames(b)
	if len(got) != 2 || got[0] != "managed_workgroup_name" {
		t.Errorf("block attrs = %v, want [managed_workgroup_name status_message]", got)
	}
	// The bullets must NOT have leaked into the root attribute block.
	for _, a := range blkAttrNames(d.AttributeBlocks[""]) {
		if a == "managed_workgroup_name" {
			t.Error("lead-in attributes leaked into the root block")
		}
	}
}

// TestParse_BlockAnchorsAndLinks records heading anchors and per-bullet link
// targets, the raw material for shared-subsection resolution (issue #51).
func TestParse_BlockAnchorsAndLinks(t *testing.T) {
	t.Parallel()

	md := "## Attribute Reference\n\n" +
		"### `endpoints` Block\n\n" +
		"* `management` - Endpoint. See [Endpoint](#endpoint).\n" +
		"* `intercluster` - Endpoint. See [Endpoint](#endpoint).\n\n" +
		"#### Endpoint\n\n" +
		"* `dns_name` - DNS name.\n" +
		"* `ip_addresses` - IP addresses.\n"

	d, err := doc.ParseWithOptions([]byte(md), "t", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	if d.BlockAnchors["endpoint"] != "endpoint" {
		t.Errorf(`BlockAnchors["endpoint"] = %q, want "endpoint"`, d.BlockAnchors["endpoint"])
	}
	eb := d.AttributeBlocks["endpoints"]
	if eb == nil {
		t.Fatal(`expected "endpoints" block`)
	}
	for _, a := range eb.Attributes {
		if a.Name == "management" && a.LinkAnchor != "endpoint" {
			t.Errorf(`management LinkAnchor = %q, want "endpoint"`, a.LinkAnchor)
		}
	}
}

// TestParse_HeadingAnchorsPreserveUnderscores guards that generated anchor
// slugs keep underscores (matching GitHub), so snake_case block headings like
// "`ec2_configuration` Block" produce "ec2_configuration-block" rather than
// dropping the underscore. Duplicate headings receive GitHub's -1/-2 suffixes.
func TestParse_HeadingAnchorsPreserveUnderscores(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `x` - (Optional) X.\n\n" +
		"### `ec2_configuration` Block\n\n" +
		"* `image_type` - (Optional) Type.\n\n" +
		"### `ec2_configuration` Block\n\n" +
		"* `image_id` - (Optional) ID.\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	if !d.HeadingAnchors["ec2_configuration-block"] {
		t.Errorf("HeadingAnchors missing underscore-preserving slug; got %v", d.HeadingAnchors)
	}
	if !d.HeadingAnchors["ec2_configuration-block-1"] {
		t.Errorf("HeadingAnchors missing duplicate-suffixed slug; got %v", d.HeadingAnchors)
	}
}

// TestParse_HeadingAnchorCollisionAvoidance guards GitHub's collision handling:
// a suffixed candidate that clashes with an explicit heading is bumped again.
// Bare headings are used so the slugs are exactly "foo", "foo", "foo-1" (no
// "-block" suffix), which forces the collision: the second "foo" becomes
// "foo-1", so the third heading — whose base slug is already "foo-1" — must
// become "foo-1-1". A per-base counter would instead emit "foo-1" twice and
// never produce "foo-1-1", so this fixture fails without collision avoidance.
func TestParse_HeadingAnchorCollisionAvoidance(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `x` - (Optional) X.\n\n" +
		"### foo\n\n* `a` - (Optional) A.\n\n" +
		"### foo\n\n* `b` - (Optional) B.\n\n" +
		"### foo-1\n\n* `c` - (Optional) C.\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"foo", "foo-1", "foo-1-1"} {
		if !d.HeadingAnchors[want] {
			t.Errorf("HeadingAnchors missing %q; got %v", want, d.HeadingAnchors)
		}
	}
}

// TestParse_InPageLinksSkipCode confirms that link-looking text inside inline
// code spans and fenced code blocks is NOT collected as an in-page link. The
// anchors rule is enabled by default, so a regression here (e.g. a Goldmark or
// extension change) would turn literal example content into false dead-anchor
// errors — this test locks in the current behavior.
func TestParse_InPageLinksSkipCode(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `a` - (Optional) Real link [ok](#argument-reference) and inline `[x](#missing-inline)`.\n\n" +
		"```\n[y](#missing-fenced)\n```\n\n" +
		"~~~markdown\n[z](#missing-tilde)\n~~~\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, l := range d.InPageLinks {
		got[l.Fragment] = true
	}
	if !got["argument-reference"] {
		t.Errorf("expected the real prose link to be collected; got %v", d.InPageLinks)
	}
	for _, bad := range []string{"missing-inline", "missing-fenced", "missing-tilde"} {
		if got[bad] {
			t.Errorf("link inside code must not be collected: %q present in %v", bad, d.InPageLinks)
		}
	}
}
