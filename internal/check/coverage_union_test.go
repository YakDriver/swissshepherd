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

// TestSchemaDocsRule_NestedBlock_LeafAndDotNotation_Coverage is the
// aws_xray_indexing_rule pattern: a nested schema block is documented
// under its leaf-name H3 heading in Argument Reference (input attrs)
// AND under a dot-notation reference in Attribute Reference (computed
// attrs). The full set of documented attributes for the schema's full
// path must be the union of both doc blocks; previously a single-block
// lookup picked one and silently dropped the other.
func TestSchemaDocsRule_NestedBlock_LeafAndDotNotation_Coverage(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"rule"},
			},
			"rule": {
				ChildBlocks: []string{"rule.probabilistic"},
			},
			"rule.probabilistic": {
				Attributes: []schema.Attribute{
					{Name: "desired_sampling_percentage", Required: true},
					{Name: "actual_sampling_percentage", Computed: true},
				},
			},
		},
	}

	// `desired_sampling_percentage` is documented under the
	// `### `probabilistic` Block` heading (keyed "probabilistic" in the
	// doc model). `actual_sampling_percentage` is documented as a
	// dot-notation reference in Attribute Reference (routed to a doc
	// block keyed "rule.probabilistic"). Both are valid, complementary,
	// and together they fully document the schema's "rule.probabilistic"
	// block.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `rule` - (Required) Rule. See [`rule` Block](#rule-block).\n\n" +
		"### `rule` Block\n\n" +
		"* `probabilistic` - (Optional) Probabilistic config. See [`probabilistic` Block](#probabilistic-block).\n\n" +
		"### `probabilistic` Block\n\n" +
		"* `desired_sampling_percentage` - (Required) Configured sampling percentage.\n\n" +
		"## Attribute Reference\n\n" +
		"* `rule[*].probabilistic[*].actual_sampling_percentage` - Applied sampling percentage.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "desired_sampling_percentage") ||
			strings.Contains(r.Message, "actual_sampling_percentage") {
			t.Errorf("unexpected message about probabilistic attribute: %s", r.Message)
		}
	}
}
