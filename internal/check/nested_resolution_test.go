// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// TestSharedSubsection_SiblingsResolve: two structurally-identical sibling
// blocks documented once under a shared subsection, linked from both bullets,
// each satisfy Read-Only coverage (issue #51).
func TestSharedSubsection_SiblingsResolve(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "endpoints", Computed: true, Children: []schema.Attribute{
					{Name: "management", Computed: true, Children: []schema.Attribute{
						{Name: "dns_name", Computed: true},
						{Name: "ip_addresses", Computed: true},
					}},
					{Name: "intercluster", Computed: true, Children: []schema.Attribute{
						{Name: "dns_name", Computed: true},
						{Name: "ip_addresses", Computed: true},
					}},
				}},
			}},
		},
	}
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		DataSources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	md := "## Attribute Reference\n\n" +
		"* `endpoints` - Endpoints. See [`endpoints`](#endpoints-block) below.\n\n" +
		"### `endpoints` Block\n\n" +
		"* `intercluster` - Endpoint. See [Endpoint](#endpoint).\n" +
		"* `management` - Endpoint. See [Endpoint](#endpoint).\n\n" +
		"#### Endpoint\n\n" +
		"* `dns_name` - DNS name.\n" +
		"* `ip_addresses` - IP addresses.\n"

	d, err := doc.ParseWithOptions([]byte(md), "aws_test", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})
	if hasMessage(results, "should be documented") {
		t.Errorf("shared subsection should satisfy coverage for both siblings, got:\n  %s", joinMessages(results))
	}
}

// TestSharedSubsection_DifferentlyNamedSiblings: siblings with different names
// sharing one subsection (available_labels/consumed_labels -> Labels), the
// wafv2 variant from issue #51's comment.
func TestSharedSubsection_DifferentlyNamedSiblings(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "available_labels", Computed: true, Children: []schema.Attribute{{Name: "name", Computed: true}}},
				{Name: "consumed_labels", Computed: true, Children: []schema.Attribute{{Name: "name", Computed: true}}},
			}},
		},
	}
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		DataSources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	md := "## Attribute Reference\n\n" +
		"* `available_labels` - Labels. See [Labels](#labels) below.\n" +
		"* `consumed_labels` - Labels. See [Labels](#labels) below.\n\n" +
		"### Labels\n\n" +
		"* `name` - Label name.\n"

	d, err := doc.ParseWithOptions([]byte(md), "aws_test", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})
	if hasMessage(results, "should be documented") {
		t.Errorf("shared Labels subsection should satisfy coverage for both siblings, got:\n  %s", joinMessages(results))
	}
}

// TestProseLeadIn_NoCascade: a legacy prose lead-in introducing a nested
// block's read-only fields is recognized, so its bullets are covered and don't
// cascade into phantom/root/ordering findings (issue #53).
func TestProseLeadIn_NoCascade(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "catalog_properties", Optional: true, Children: []schema.Attribute{
					{Name: "data_lake_access_properties", Optional: true, Children: []schema.Attribute{
						{Name: "data_lake_access", Optional: true},
						{Name: "managed_workgroup_name", Computed: true},
						{Name: "status_message", Computed: true},
					}},
				}},
			}},
		},
	}
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	md := "## Argument Reference\n\n" +
		"* `catalog_properties` - (Optional) Config. See below.\n\n" +
		"### `catalog_properties` Block\n\n" +
		"* `data_lake_access_properties` - (Optional) Config. See below.\n\n" +
		"#### `data_lake_access_properties`\n\n" +
		"* `data_lake_access` - (Optional) Whether enabled.\n\n" +
		"## Attribute Reference\n\n" +
		"The `catalog_properties[0].data_lake_access_properties[0]` block also exports:\n\n" +
		"* `managed_workgroup_name` - Managed workgroup name.\n" +
		"* `status_message` - Status message.\n"

	d, err := doc.ParseWithOptions([]byte(md), "aws_test", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})
	for _, bad := range []string{"does not exist in schema", "should be documented", "should come before"} {
		if hasMessage(results, bad) {
			t.Errorf("prose lead-in should not cascade (%q), got:\n  %s", bad, joinMessages(results))
		}
	}
}
