// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package schema_test

import (
	"slices"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/schema"
)

func blockAttrNames(b *schema.Block) []string {
	if b == nil {
		return nil
	}
	names := make([]string, 0, len(b.Attributes))
	for _, a := range b.Attributes {
		names = append(names, a.Name)
	}
	return names
}

// TestExpandObjectAttributes turns object-typed attribute children (stored on
// Attribute.Children) into dot-path keyed blocks, recursively, without
// overwriting existing blocks.
func TestExpandObjectAttributes(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "name", Required: true},
				{Name: "items", Computed: true, Children: []schema.Attribute{
					{Name: "arn", Computed: true},
					{Name: "dns_entry", Computed: true, Children: []schema.Attribute{
						{Name: "domain_name", Computed: true},
					}},
				}},
			}},
		},
	}

	ps := &schema.ProviderSchema{
		DataSources: map[string]*schema.ResourceSchema{"aws_test": rs},
	}
	schema.ExpandObjectAttributes(ps)

	items := rs.Blocks["items"]
	if items == nil {
		t.Fatal(`expected block "items" after expansion`)
	}
	if got := blockAttrNames(items); !slices.Contains(got, "arn") || !slices.Contains(got, "dns_entry") {
		t.Errorf(`block "items" attrs = %v, want arn and dns_entry`, got)
	}
	nested := rs.Blocks["items.dns_entry"]
	if nested == nil {
		t.Fatal(`expected block "items.dns_entry" after expansion`)
	}
	if got := blockAttrNames(nested); !slices.Contains(got, "domain_name") {
		t.Errorf(`block "items.dns_entry" attrs = %v, want domain_name`, got)
	}
}

// TestExpandObjectAttributes_DoesNotOverwrite confirms a pre-existing block at
// the same path (a real nested block) is preserved.
func TestExpandObjectAttributes_DoesNotOverwrite(t *testing.T) {
	t.Parallel()

	real := &schema.Block{Path: "items", Attributes: []schema.Attribute{{Name: "real_attr", Required: true}}}
	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"":      {Path: "", Attributes: []schema.Attribute{{Name: "items", Computed: true, Children: []schema.Attribute{{Name: "arn", Computed: true}}}}},
			"items": real,
		},
	}
	schema.ExpandObjectAttributes(&schema.ProviderSchema{Resources: map[string]*schema.ResourceSchema{"aws_test": rs}})

	if rs.Blocks["items"] != real {
		t.Error(`existing "items" block must not be overwritten`)
	}
}
