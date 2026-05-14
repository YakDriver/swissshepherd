// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// TestSchemaDocsRule_MultiLevelParentDisambiguation tests that blocks with
// the same leaf name but different parent paths are correctly disambiguated.
// Regression test for issue where s3.s3_output_format_config.aggregation_config
// and upsolver.s3_output_format_config.aggregation_config were confused.
func TestSchemaDocsRule_MultiLevelParentDisambiguation(t *testing.T) {
	t.Parallel()

	// Schema with two similar paths that differ only in the second-to-last segment
	rs := &schema.ResourceSchema{
		Blocks: map[string]*schema.Block{
			"destination.s3.format.config": {
				Path: "destination.s3.format.config",
				Attributes: []schema.Attribute{
					{Name: "common_attr", Optional: true},
					{Name: "s3_only_attr", Optional: true},
				},
			},
			"destination.upsolver.format.config": {
				Path: "destination.upsolver.format.config",
				Attributes: []schema.Attribute{
					{Name: "common_attr", Optional: true},
					{Name: "upsolver_only_attr", Optional: true},
				},
			},
		},
	}

	// Documentation with headings that include the disambiguating parent
	d := &doc.Document{
		ArgumentBlocks: map[string]*doc.DocBlock{
			"s3.format.config": {
				Name:    "s3.format.config",
				Heading: "destination s3 format config Block",
				Attributes: []doc.DocAttribute{
					{Name: "common_attr", Optional: true},
					{Name: "s3_only_attr", Optional: true},
				},
			},
			"upsolver.format.config": {
				Name:    "upsolver.format.config",
				Heading: "destination upsolver format config Block",
				Attributes: []doc.DocAttribute{
					{Name: "common_attr", Optional: true},
					{Name: "upsolver_only_attr", Optional: true},
				},
			},
		},
		AttributeBlocks: map[string]*doc.DocBlock{},
	}

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_resource", Schema: rs, Doc: d})

	if len(results) != 0 {
		t.Errorf("expected no errors, got %d:", len(results))
		for _, r := range results {
			t.Logf("  %s: %s", r.Severity, r.Message)
		}
	}
}
