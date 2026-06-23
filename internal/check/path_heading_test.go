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

// TestSchemaDocsRule_PathKeyedHeadings_PreferredStyle confirms that
// path-keyed doc blocks (e.g. `spec.grpc_route.match`) participate in
// the preferred-style check rather than being silently skipped. The
// schema lookup uses the leaf name so the block is recognized as
// schema-present; the ambiguity branch is bypassed because the
// heading is already in dot-path form.
func TestSchemaDocsRule_PathKeyedHeadings_PreferredStyle(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"spec"},
			},
			"spec": {
				ChildBlocks: []string{"spec.http_route", "spec.grpc_route"},
			},
			"spec.http_route": {
				ChildBlocks: []string{"spec.http_route.match"},
			},
			"spec.http_route.match": {
				Attributes: []schema.Attribute{{Name: "method", Optional: true}},
			},
			"spec.grpc_route": {
				ChildBlocks: []string{"spec.grpc_route.match"},
			},
			"spec.grpc_route.match": {
				Attributes: []schema.Attribute{{Name: "service_name", Optional: true}},
			},
		},
	}

	// Two path-keyed headings in preferred dot-path form.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `spec` - (Required) Spec. See [`spec`](#spec) below.\n\n" +
		"### `spec` Block\n\n" +
		"* `http_route` - (Optional) HTTP. See [`spec.http_route`](#spechttp_route-block).\n" +
		"* `grpc_route` - (Optional) gRPC. See [`spec.grpc_route`](#specgrpc_route-block).\n\n" +
		"### `spec.http_route` Block\n\n" +
		"* `match` - (Optional) Match. See [`spec.http_route.match`](#spechttp_routematch-block).\n\n" +
		"### `spec.http_route.match` Block\n\n" +
		"* `method` - (Optional) Method.\n\n" +
		"### `spec.grpc_route` Block\n\n" +
		"* `match` - (Optional) Match. See [`spec.grpc_route.match`](#specgrpc_routematch-block).\n\n" +
		"### `spec.grpc_route.match` Block\n\n" +
		"* `service_name` - (Optional) Service.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Preferred templates: dot-path form first, then leaf form.
	rule := &check.SchemaDocsRule{
		Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"},
	}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	// No ambiguity warning for the path-keyed match blocks (they
	// already disambiguate by construction), no preferred-style
	// warning either (they match `{Path}` Block).
	for _, r := range results {
		if strings.Contains(r.Message, "spec.http_route.match") ||
			strings.Contains(r.Message, "spec.grpc_route.match") {
			t.Errorf("unexpected finding on path-keyed match block: %s", r.Message)
		}
		if strings.Contains(r.Message, "ambiguous") &&
			(strings.Contains(r.Message, "spec.http_route") || strings.Contains(r.Message, "spec.grpc_route")) {
			t.Errorf("unexpected ambiguity finding on path-keyed block: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_PathKeyedHeadings_BadStyleStillWarns confirms
// that path-keyed blocks aren't given a free pass on preferred-style
// when they use a non-preferred form. The previous bug was that
// path-keyed blocks were skipped entirely; now they should be checked
// like any other block.
func TestSchemaDocsRule_PathKeyedHeadings_BadStyleStillWarns(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"foo"},
			},
			"foo": {
				ChildBlocks: []string{"foo.bar"},
			},
			"foo.bar": {
				Attributes: []schema.Attribute{{Name: "qux", Optional: true}},
			},
		},
	}

	// Path-keyed heading but in bare backtick form rather than the
	// preferred `<path>` Block form.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `foo` - (Required) Foo. See [`foo`](#foo) below.\n\n" +
		"### `foo` Block\n\n" +
		"* `bar` - (Optional) Bar. See [`foo.bar`](#foobar).\n\n" +
		"### `foo.bar`\n\n" +
		"* `qux` - (Optional) Qux.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{
		Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"},
	}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	// We expect a preferred-style warning on the bare path-form
	// heading suggesting the Block-suffix form.
	var found bool
	for _, r := range results {
		if strings.Contains(r.Message, "foo.bar") &&
			strings.Contains(r.Message, "should be") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected preferred-style finding on `foo.bar` heading; got %d results", len(results))
		for _, r := range results {
			t.Logf("  result: %s", r.Message)
		}
	}
}
