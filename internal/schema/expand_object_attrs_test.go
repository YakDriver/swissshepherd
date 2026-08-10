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

// TestExpandObjectAttributes_ConfigUnknown verifies that the expanded block's
// ConfigUnknown flag tracks whether the object's parent is configurable
// (Optional and/or Required) — since a configurable object's cty-encoded
// fields have unknowable per-field configurability — and that it propagates to
// nested object blocks. A Computed-only parent's fields are necessarily
// read-only, so its block stays ConfigUnknown == false.
func TestExpandObjectAttributes_ConfigUnknown(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				// Configurable (Optional+Computed) object → fields unknown.
				{Name: "scaling_target", Optional: true, Computed: true, Children: []schema.Attribute{
					{Name: "max_task_count", Computed: true},
				}},
				// Optional-only object → fields unknown.
				{Name: "contacts", Optional: true, Children: []schema.Attribute{
					{Name: "email", Computed: true},
				}},
				// Computed-only output object → fields genuinely read-only,
				// with a further-nested object that must also stay known.
				{Name: "items", Computed: true, Children: []schema.Attribute{
					{Name: "arn", Computed: true},
					{Name: "dns_entry", Computed: true, Children: []schema.Attribute{
						{Name: "domain_name", Computed: true},
					}},
				}},
			}},
		},
	}
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	cases := map[string]bool{
		"scaling_target":  true,
		"contacts":        true,
		"items":           false,
		"items.dns_entry": false,
	}
	for path, want := range cases {
		b := rs.Blocks[path]
		if b == nil {
			t.Fatalf("expected block %q after expansion", path)
		}
		if b.ConfigUnknown != want {
			t.Errorf("block %q ConfigUnknown = %v, want %v", path, b.ConfigUnknown, want)
		}
	}
}

// TestExpandObjectAttributes_ConfigUnknownPropagates confirms that once an
// ancestor object is configurable, a nested Computed-only sub-object still
// inherits ConfigUnknown — a configurable block's sub-fields remain unknowable.
func TestExpandObjectAttributes_ConfigUnknownPropagates(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "config", Optional: true, Children: []schema.Attribute{
					// cty sub-object: no flags recovered, defaults Computed:true.
					{Name: "logging", Computed: true, Children: []schema.Attribute{
						{Name: "level", Computed: true},
					}},
				}},
			}},
		},
	}
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	if b := rs.Blocks["config.logging"]; b == nil || !b.ConfigUnknown {
		t.Errorf(`block "config.logging" ConfigUnknown = %v, want true (configurable ancestor)`, b != nil && b.ConfigUnknown)
	}
}
