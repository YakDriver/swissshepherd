// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"slices"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

func TestBlocks_CombinedHeading_SharedAttributes(t *testing.T) {
	t.Parallel()

	// Reproduces: aws_appsync_channel_namespace
	// ### `publish_auth_mode` and `subscribe_auth_mode` documents both blocks.
	source := []byte("# Resource: test\n\n## Argument Reference\n\n* `name` - (Required) Name.\n* `publish_auth_mode` - (Optional) See below.\n* `subscribe_auth_mode` - (Optional) See below.\n\n### `publish_auth_mode` and `subscribe_auth_mode`\n\n* `auth_type` - (Required) Type.\n")

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatal(err)
	}

	blocks := d.ArgumentBlocks
	for _, name := range []string{"publish_auth_mode", "subscribe_auth_mode"} {
		b, ok := blocks[name]
		if !ok {
			t.Errorf("block %q not found", name)
			continue
		}
		found := false
		for _, attr := range b.Attributes {
			if attr.Name == "auth_type" {
				found = true
			}
		}
		if !found {
			t.Errorf("block %q missing attribute auth_type", name)
		}
	}
}

func TestBlocks_CombinedHeading_NestedUnderParent(t *testing.T) {
	t.Parallel()

	// Reproduces: aws_appsync_channel_namespace nested combined heading
	// #### `on_publish` and `on_subscribe` inside ### `handler_configs`
	source := []byte("# Resource: test\n\n## Argument Reference\n\n* `handler_configs` - (Optional) See below.\n\n### `handler_configs`\n\n* `on_publish` - (Optional) See below.\n* `on_subscribe` - (Optional) See below.\n\n#### `on_publish` and `on_subscribe`\n\n* `behavior` - (Required) Behavior.\n* `integration` - (Required) Integration.\n")

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatal(err)
	}

	blocks := d.ArgumentBlocks

	// handler_configs should have on_publish and on_subscribe as attributes
	hc := blocks["handler_configs"]
	if hc == nil {
		t.Fatal("block handler_configs not found")
	}
	for _, want := range []string{"on_publish", "on_subscribe"} {
		found := false
		for _, attr := range hc.Attributes {
			if attr.Name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("handler_configs missing attribute %q", want)
		}
	}

	// Both on_publish and on_subscribe should have behavior and integration
	for _, name := range []string{"on_publish", "on_subscribe"} {
		b, ok := blocks[name]
		if !ok {
			t.Errorf("block %q not found", name)
			continue
		}
		for _, want := range []string{"behavior", "integration"} {
			found := false
			for _, attr := range b.Attributes {
				if attr.Name == want {
					found = true
				}
			}
			if !found {
				t.Errorf("block %q missing attribute %q", name, want)
			}
		}
	}
}

func TestBlocks_SingleWordTitleCase_NotSwallowedByPreviousBlock(t *testing.T) {
	t.Parallel()

	// Reproduces: aws_appsync_resolver
	// ### Runtime is a single Title Case word that must start a new block,
	// not be swallowed by the preceding #### Lambda Conflict Handler Config.
	source := []byte("# Resource: test\n\n## Argument Reference\n\n* `type` - (Required) Type.\n\n### Sync Config\n\n* `conflict_detection` - (Optional) Detection.\n* `lambda_conflict_handler_config` - (Optional) See below.\n\n#### Lambda Conflict Handler Config\n\n* `lambda_conflict_handler_arn` - (Optional) ARN.\n\n### Runtime\n\n* `name` - (Required) Name.\n* `runtime_version` - (Required) Version.\n")

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatal(err)
	}

	blocks := d.ArgumentBlocks

	// lambda_conflict_handler_config must NOT contain name/runtime_version
	lc := blocks["lambda_conflict_handler_config"]
	if lc == nil {
		t.Fatal("block lambda_conflict_handler_config not found")
	}
	for _, attr := range lc.Attributes {
		if attr.Name == "name" || attr.Name == "runtime_version" {
			t.Errorf("lambda_conflict_handler_config incorrectly contains %q", attr.Name)
		}
	}

	// runtime must contain name and runtime_version
	rt := blocks["runtime"]
	if rt == nil {
		t.Fatal("block runtime not found")
	}
	for _, want := range []string{"name", "runtime_version"} {
		found := false
		for _, attr := range rt.Attributes {
			if attr.Name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("runtime missing attribute %q", want)
		}
	}
}

func TestBlocks_CombinedHeading_ThreeBlocks(t *testing.T) {
	t.Parallel()

	// Edge case: three blocks in one heading with comma-and pattern.
	source := []byte("# Resource: test\n\n## Argument Reference\n\n* `a` - (Optional) See below.\n* `b` - (Optional) See below.\n* `c` - (Optional) See below.\n\n### `a`, `b`, and `c`\n\n* `shared` - (Required) Shared attr.\n")

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatal(err)
	}

	blocks := d.ArgumentBlocks
	for _, name := range []string{"a", "b", "c"} {
		b, ok := blocks[name]
		if !ok {
			t.Errorf("block %q not found", name)
			continue
		}
		found := false
		for _, attr := range b.Attributes {
			if attr.Name == "shared" {
				found = true
			}
		}
		if !found {
			t.Errorf("block %q missing attribute shared", name)
		}
	}
}

func TestBlocks_CombinedHeading_DoesNotPolluteNextBlock(t *testing.T) {
	t.Parallel()

	// Ensure attributes after a combined heading don't leak into a subsequent block.
	source := []byte("# Resource: test\n\n## Argument Reference\n\n* `x` - (Optional) See below.\n* `y` - (Optional) See below.\n* `z` - (Optional) See below.\n\n### `x` and `y`\n\n* `shared` - (Required) Shared.\n\n### `z`\n\n* `unique` - (Required) Unique.\n")

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatal(err)
	}

	blocks := d.ArgumentBlocks

	// x and y should have "shared" but NOT "unique"
	for _, name := range []string{"x", "y"} {
		b := blocks[name]
		if b == nil {
			t.Errorf("block %q not found", name)
			continue
		}
		for _, attr := range b.Attributes {
			if attr.Name == "unique" {
				t.Errorf("block %q incorrectly contains unique", name)
			}
		}
	}

	// z should have "unique" but NOT "shared"
	z := blocks["z"]
	if z == nil {
		t.Fatal("block z not found")
	}
	for _, attr := range z.Attributes {
		if attr.Name == "shared" {
			t.Error("block z incorrectly contains shared")
		}
	}
	found := false
	for _, attr := range z.Attributes {
		if attr.Name == "unique" {
			found = true
		}
	}
	if !found {
		t.Error("block z missing attribute unique")
	}
}

func TestBlocks_UsageBasedPricingTerm(t *testing.T) {
	t.Parallel()

	source := []byte("# Data Source: test\n\n## Argument Reference\n\n* `id` - (Optional) The ID.\n\n## Attribute Reference\n\n* `usage_based_pricing_term` - Details about the pricing terms. See [`usage_based_pricing_term`](#usage_based_pricing_term).\n\n### `usage_based_pricing_term` Block\n\n* `rate_card` - Details about a usage price for each dimension. See [`rate_card`](#rate_card).\n\n### `rate_card` Block\n\n* `description` - Description of the price rate.\n* `dimension` - Dimension for the price rate.\n* `price` - Single-dimensional rate information.\n* `unit` - Unit associated with the price.\n")

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatal(err)
	}

	blocks := d.AttributeBlocks

	// Validate usage_based_pricing_term block
	usageTermBlock := blocks["usage_based_pricing_term"]
	if usageTermBlock == nil {
		t.Fatal("block usage_based_pricing_term not found")
	}
	for _, want := range []string{"rate_card"} {
		found := false
		for _, attr := range usageTermBlock.Attributes {
			if attr.Name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("block usage_based_pricing_term missing attribute %q", want)
		}
	}
}

// TestBlocks_UnparseableHeadingOrphansBullets: bullets under an H3+ heading
// that matches no heading template belong to no section. Before #77 they were
// credited to the previous section, producing phantom fields there. A dot-path
// reference bullet names its own path and is still routed.
func TestBlocks_UnparseableHeadingOrphansBullets(t *testing.T) {
	t.Parallel()

	source := []byte("# Resource: test\n\n## Argument Reference\n\n" +
		"* `source` - (Required) Source.\n\n" +
		"### `source` Block\n\n" +
		"* `location` - (Required) Location.\n\n" +
		"### source: auth\n\n" +
		"* `resource` - (Optional) Resource.\n" +
		"* `source[*].auth[*].type` - (Required) Type.\n\n" +
		"### `other` Block\n\n" +
		"* `name` - (Optional) Name.\n")

	d, err := doc.ParseWithTemplates(source, "test", doc.HeadingTemplates{"`{Block}` Block"})
	if err != nil {
		t.Fatal(err)
	}

	names := func(key string) []string {
		var out []string
		if b := d.ArgumentBlocks[key]; b != nil {
			for _, a := range b.Attributes {
				out = append(out, a.Name)
			}
		}
		return out
	}
	testCases := map[string]struct {
		key  string
		want []string
	}{
		"previous section keeps only its own bullet": {key: "source", want: []string{"location"}},
		"dot-path reference is still routed":         {key: "source.auth", want: []string{"type"}},
		"next parsed heading starts a section again": {key: "other", want: []string{"name"}},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := names(tc.key); !slices.Equal(got, tc.want) {
				t.Errorf("ArgumentBlocks[%q] = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

// TestBlocks_ProseLeadInCandidates: after a section's own bullets, a
// colon-terminated paragraph naming something in backticks may introduce a
// list for another block. The parser records it with the list's bullets and
// leaves the bullets in the section; only the schema can say whether they
// belong to another block (#77).
func TestBlocks_ProseLeadInCandidates(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		body        string
		wantRoot    []string
		wantBullets []string // bullets of the recorded candidate; nil for none
	}{
		"prose naming another block": {
			body:        "* `name` - (Required) Name.\n\nThe `cloudwatch_logs` object takes the following arguments:\n\n* `role_arn` - (Required) Role.\n",
			wantRoot:    []string{"name", "role_arn"},
			wantBullets: []string{"role_arn"},
		},
		"byline before the first list": {
			body:     "The following arguments are required:\n\n* `name` - (Required) Name.\n",
			wantRoot: []string{"name"},
		},
		"second byline without a block name": {
			body:     "* `name` - (Required) Name.\n\nThe following arguments are optional:\n\n* `tags` - (Optional) Tags.\n",
			wantRoot: []string{"name", "tags"},
		},
		"prose without a trailing colon": {
			body:     "* `name` - (Required) Name.\n\nSee `cloudwatch_logs` below.\n\n* `tags` - (Optional) Tags.\n",
			wantRoot: []string{"name", "tags"},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.Parse([]byte("# Resource: test\n\n## Argument Reference\n\n"+tc.body), "test")
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, a := range d.ArgumentBlocks[""].Attributes {
				got = append(got, a.Name)
			}
			if !slices.Equal(got, tc.wantRoot) {
				t.Errorf("root bullets = %v, want %v", got, tc.wantRoot)
			}
			var bullets []string
			for _, pl := range d.ProseLeadIns {
				for _, a := range pl.Bullets {
					bullets = append(bullets, a.Name)
				}
			}
			if !slices.Equal(bullets, tc.wantBullets) {
				t.Errorf("candidate bullets = %v, want %v", bullets, tc.wantBullets)
			}
		})
	}
}

// TestBlocks_ProseContinuingCurrentBlockIsNotOrphaned: prose that names the
// current section's own block continues that section.
func TestBlocks_ProseContinuingCurrentBlockIsNotOrphaned(t *testing.T) {
	t.Parallel()

	d, err := doc.Parse([]byte("# Resource: test\n\n## Argument Reference\n\n"+
		"### `rule` Block\n\n* `a` - (Optional) A.\n\nThe `rule` block also supports:\n\n* `b` - (Optional) B.\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range d.ArgumentBlocks["rule"].Attributes {
		got = append(got, a.Name)
	}
	if !slices.Equal(got, []string{"a", "b"}) || len(d.ProseLeadIns) != 0 {
		t.Errorf("rule bullets = %v, prose lead-ins = %+v; want [a b] and none", got, d.ProseLeadIns)
	}
}

// TestBlocks_DuplicateHeadingOccurrences: headings that normalize to the same
// key are kept as separate occurrences, each with its own bullets, so coverage
// can judge them one by one (#77). Content routed from elsewhere on the page
// joins the merged entry but no occurrence.
func TestBlocks_DuplicateHeadingOccurrences(t *testing.T) {
	t.Parallel()

	d, err := doc.Parse([]byte("# Resource: test\n\n## Argument Reference\n\n"+
		"* `z[*].routed` - (Optional) Routed.\n\n"+
		"### `z` Block\n\n* `a1` - (Optional) A1.\n\n"+
		"### `z` Block\n\n* `a4` - (Optional) A4.\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	var got [][]string
	for _, o := range d.ArgumentBlocks["z"].Occurrences {
		var names []string
		for _, a := range o.Attributes {
			names = append(names, a.Name)
		}
		got = append(got, names)
	}
	want := [][]string{{"a1"}, {"a4"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
}
