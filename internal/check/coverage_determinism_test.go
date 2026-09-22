// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// coverageMissingBlockMsgs returns the sorted set of "block ... is not
// documented" coverage messages for a schema+doc pair.
func coverageMissingBlockMsgs(t *testing.T, src string, rs *schema.ResourceSchema) []string {
	t.Helper()
	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", labelTemplates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
	var msgs []string
	for _, r := range results {
		if strings.Contains(r.Message, "is not documented") {
			msgs = append(msgs, r.Message)
		}
	}
	slices.Sort(msgs)
	return msgs
}

// Several undocumented blocks that share a leaf name are deduped to a single
// representative finding. Before issue #65 the representative path was chosen by
// Go's randomized map iteration over rs.Blocks, so the reported path flapped
// between runs. Coverage must now produce identical output every run.
func TestCoverage_UndocumentedSiblingBlocksDeterministic(t *testing.T) {
	t.Parallel()

	// foo.thing and bar.thing are distinct undocumented blocks that share the
	// leaf name "thing"; the leaf-keyed dedup keeps exactly one representative.
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":          {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"foo", "bar"}},
		"foo":       {Path: "foo", ChildBlocks: []string{"foo.thing"}},
		"bar":       {Path: "bar", ChildBlocks: []string{"bar.thing"}},
		"foo.thing": {Path: "foo.thing", Attributes: []schema.Attribute{{Name: "x", Optional: true}}},
		"bar.thing": {Path: "bar.thing", Attributes: []schema.Attribute{{Name: "x", Optional: true}}},
	}}

	// Doc documents name, foo, and bar, but never a "thing" subsection.
	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"### `foo` Block\n\n" +
		"* `thing` - (Optional) Thing. See below.\n\n" +
		"### `bar` Block\n\n" +
		"* `thing` - (Optional) Thing. See below.\n"

	// Go re-randomizes map iteration order on each range statement, so repeating
	// the check many times in one process exercises many orderings.
	first := coverageMissingBlockMsgs(t, src, rs)

	// Exactly one representative for the shared "thing" leaf.
	thingCount := 0
	for _, m := range first {
		if strings.Contains(m, `"thing"`) || strings.Contains(m, ".thing") {
			thingCount++
		}
	}
	if thingCount != 1 {
		t.Fatalf("expected exactly one deduped 'thing' block finding, got %d: %v", thingCount, first)
	}

	for i := 1; i < 100; i++ {
		got := coverageMissingBlockMsgs(t, src, rs)
		if !slices.Equal(got, first) {
			t.Fatalf("coverage output is nondeterministic across runs:\n run0 = %v\n run%d = %v", first, i, got)
		}
	}
}
