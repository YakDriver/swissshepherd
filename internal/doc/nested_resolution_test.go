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
