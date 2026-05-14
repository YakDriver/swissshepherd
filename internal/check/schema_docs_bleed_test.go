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

// TestSchemaDocsRule_NoFalseComputedMisplacement verifies that computed-only
// attributes documented in the Attribute Reference section are NOT flagged as
// misplaced in the Argument Reference section, even when broad heading
// templates cause attribute-section items to bleed into ArgumentBlocks.
func TestSchemaDocsRule_NoFalseComputedMisplacement(t *testing.T) {
	t.Parallel()

	// Templates broad enough to cause bleed (includes "{Block}" which matches anything)
	templates := doc.HeadingTemplates{"`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := []byte(`# Data Source: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name of the thing.

## Attribute Reference

* ` + "`arn`" + ` - ARN of the thing.
* ` + "`created_time`" + ` - Time at which this thing was created.
* ` + "`description`" + ` - Description of the thing.
`)

	d, err := doc.ParseWithTemplates(src, "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}

	rs := &schema.ResourceSchema{
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Optional: true},
					{Name: "arn", Computed: true},
					{Name: "created_time", Computed: true},
					{Name: "description", Computed: true},
				},
			},
		},
	}

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "should not appear in Argument Reference") {
			t.Errorf("false positive: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_NoFalseLabelsWarning verifies that attributes in the
// Attribute Reference section are NOT flagged for missing (Required)/(Optional)
// labels, even when broad heading templates cause them to appear in ArgumentBlocks.
func TestSchemaDocsRule_NoFalseLabelsWarning(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := []byte(`# Data Source: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name of the thing.

## Attribute Reference

* ` + "`arn`" + ` - ARN of the thing.
* ` + "`created_date`" + ` - Creation date.
* ` + "`last_updated_date`" + ` - Last update date.
`)

	d, err := doc.ParseWithTemplates(src, "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_thing", Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "is missing (Required) or (Optional) label") {
			if strings.Contains(r.Message, "arn") || strings.Contains(r.Message, "created_date") || strings.Contains(r.Message, "last_updated_date") {
				t.Errorf("false positive label warning for attribute-section item: %s", r.Message)
			}
		}
	}
}
