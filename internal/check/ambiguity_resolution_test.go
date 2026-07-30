// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// TestSchemaDocsRule_FullPathHeadings_DescendantsOwnHeadings is a
// regression test for the self-suggesting ambiguity false positive on
// aws_appmesh_gateway_route / aws_appmesh_route. A parent block
// (spec.http2_route.match) and its descendants
// (…match.header.match, …match.query_parameter.match) all share the
// leaf "match" and are structurally distinct, so the leaf is ambiguous.
// But every block has its own exact full-path heading, so the parent
// heading is NOT ambiguous.
//
// The old resolver evaluated the parent's doc key in isolation and, via
// the non-contiguous composite matcher (parts[0].parts[1].leaf), counted
// the descendants as also "resolving" to the parent key — producing an
// ambiguity warning whose suggested fix was the identical string the
// author had already written. With most-specific-match resolution, each
// descendant is owned by its own full-path heading, so the parent
// resolves to exactly one block.
func TestSchemaDocsRule_FullPathHeadings_DescendantsOwnHeadings(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"spec"},
			},
			"spec":             {ChildBlocks: []string{"spec.http2_route"}},
			"spec.http2_route": {ChildBlocks: []string{"spec.http2_route.match"}},
			// Three distinct blocks sharing the leaf "match" -> ambiguous leaf.
			"spec.http2_route.match": {
				Attributes:  []schema.Attribute{{Name: "prefix", Optional: true}},
				ChildBlocks: []string{"spec.http2_route.match.header", "spec.http2_route.match.query_parameter"},
			},
			"spec.http2_route.match.header":       {ChildBlocks: []string{"spec.http2_route.match.header.match"}},
			"spec.http2_route.match.header.match": {Attributes: []schema.Attribute{{Name: "exact", Optional: true}}},
			"spec.http2_route.match.query_parameter": {
				ChildBlocks: []string{"spec.http2_route.match.query_parameter.match"},
			},
			"spec.http2_route.match.query_parameter.match": {Attributes: []schema.Attribute{{Name: "regex", Optional: true}}},
		},
	}

	// Every match block has its own exact full-path heading.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `spec` - (Required) Spec.\n\n" +
		"### `spec.http2_route.match` Block\n\n" +
		"* `prefix` - (Optional) Prefix.\n\n" +
		"### `spec.http2_route.match.header.match` Block\n\n" +
		"* `exact` - (Optional) Exact.\n\n" +
		"### `spec.http2_route.match.query_parameter.match` Block\n\n" +
		"* `regex` - (Optional) Regex.\n"

	d, err := doc.ParseWithTemplates([]byte(markdown), "aws_test", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"}}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "is ambiguous") {
			t.Errorf("full-path heading whose descendants have their own headings must not be flagged ambiguous; got: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_IdenticalRepeatedBlocks_NotAmbiguous is a
// regression test for the order-dependent block-signature bug. Two
// blocks sharing a leaf ("config") have the identical attribute set but
// list the attributes in different declaration order. Because schema
// attributes are loaded from a Go map, declaration order is not stable;
// an unsorted signature made these compare unequal, falsely marking the
// leaf ambiguous (nondeterministically). A shared heading for two
// structurally identical blocks is unambiguous, so no warning is
// expected.
func TestSchemaDocsRule_IdenticalRepeatedBlocks_NotAmbiguous(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"config", "parent"},
			},
			// Same attribute set {auth_ttl, issuer}, different order.
			"config": {Attributes: []schema.Attribute{
				{Name: "auth_ttl", Optional: true},
				{Name: "issuer", Required: true},
			}},
			"parent": {ChildBlocks: []string{"parent.config"}},
			"parent.config": {Attributes: []schema.Attribute{
				{Name: "issuer", Required: true},
				{Name: "auth_ttl", Optional: true},
			}},
		},
	}

	// Only the bare "config" heading exists; parent.config has none, so
	// the bare key would resolve to both blocks. The blocks are
	// identical, so this is not a real ambiguity.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `config` - (Optional) Config.\n" +
		"* `parent` - (Optional) Parent.\n\n" +
		"### `config` Block\n\n" +
		"* `auth_ttl` - (Optional) TTL.\n" +
		"* `issuer` - (Required) Issuer.\n"

	d, err := doc.ParseWithTemplates([]byte(markdown), "aws_test", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"}}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "is ambiguous") {
			t.Errorf("structurally identical repeated blocks must not be flagged ambiguous; got: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_GenuineAmbiguity_StillWarns guards against
// over-suppression. Two blocks share the leaf "config" but are
// structurally DISTINCT, and only the bare top-level heading exists —
// the nested block has no dedicated heading, so the bare key genuinely
// covers both. This is a real ambiguity and must still warn.
func TestSchemaDocsRule_GenuineAmbiguity_StillWarns(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"config", "parent"},
			},
			"config": {Attributes: []schema.Attribute{{Name: "issuer", Required: true}}},
			"parent": {ChildBlocks: []string{"parent.config"}},
			// Distinct from top-level config (extra attribute).
			"parent.config": {Attributes: []schema.Attribute{
				{Name: "issuer", Required: true},
				{Name: "extra", Optional: true},
			}},
		},
	}

	// Only the bare "config" heading; parent.config has none.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `config` - (Optional) Config.\n" +
		"* `parent` - (Optional) Parent.\n\n" +
		"### `config` Block\n\n" +
		"* `issuer` - (Required) Issuer.\n"

	d, err := doc.ParseWithTemplates([]byte(markdown), "aws_test", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"}}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	var found bool
	for _, r := range results {
		if strings.Contains(r.Message, "is ambiguous") && strings.Contains(r.Message, `block "config"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("genuine ambiguity (distinct blocks, only bare heading) must still warn; got %d results", len(results))
		for _, r := range results {
			t.Logf("  result: %s", r.Message)
		}
	}
}
