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

func TestHeadingTemplates_Match(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		templates doc.HeadingTemplates
		heading   string
		want      string
	}{
		// {Block} Block — goldmark strips backticks so `network` Block → "network Block"
		{
			name:      "backtick_block style",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "network Block",
			want:      "network",
		},
		{
			name:      "backtick_block with underscores",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "credit_specification Block",
			want:      "credit_specification",
		},
		// {Block} alone — bare snake_case name
		{
			name:      "bare block name",
			templates: doc.HeadingTemplates{"{Block}"},
			heading:   "statement",
			want:      "statement",
		},
		{
			name:      "bare block name with underscores",
			templates: doc.HeadingTemplates{"{Block}"},
			heading:   "redis_settings",
			want:      "redis_settings",
		},
		// {Title} — title case converted to snake_case
		{
			name:      "title case",
			templates: doc.HeadingTemplates{"{Title}"},
			heading:   "Credit Specification",
			want:      "credit_specification",
		},
		{
			name:      "title case multi-word",
			templates: doc.HeadingTemplates{"{Title}"},
			heading:   "CPU Options",
			want:      "cpu_options",
		},
		// {Block} Block without backticks
		{
			name:      "block suffix no backticks",
			templates: doc.HeadingTemplates{"{Block} Block"},
			heading:   "network Block",
			want:      "network",
		},
		// Multiple templates — first match wins
		{
			name:      "multiple templates first match",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "network Block",
			want:      "network",
		},
		{
			name:      "multiple templates title case fallback",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Network Interfaces",
			want:      "network_interfaces",
		},
		{
			name:      "multiple templates bare name",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "condition",
			want:      "condition",
		},
		// Title-case headings match {Title} template (the parser only invokes
		// MatchAll inside Argument/Attribute Reference, where Example Usage
		// subheadings cannot appear).
		{
			name:      "title template snake_cases multi-word headings",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Basic Usage",
			want:      "basic_usage",
		},
		{
			name:      "title template snake_cases mixed case",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Network example",
			want:      "network_example",
		},
		// Non-matches
		{
			name:      "strict rejects bare name",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "network",
			want:      "",
		},
		{
			name:      "strict rejects title case",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "Network Interfaces",
			want:      "",
		},
		{
			name:      "strict accepts correct format",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "network_interface Block",
			want:      "network_interface",
		},
		// Title case rejected when not in templates
		{
			name:      "no title template rejects title case",
			templates: doc.HeadingTemplates{"`{Block}` Block", "{Block}"},
			heading:   "CPU Options",
			want:      "",
		},
		// Upper case rejected as {Block} (must be lowercase)
		{
			name:      "block rejects uppercase",
			templates: doc.HeadingTemplates{"{Block}"},
			heading:   "Network",
			want:      "",
		},
		// {Block} rejects punctuation and hyphens (mirrors {Path})
		{
			name:      "block rejects hyphen",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "foo-bar Block",
			want:      "",
		},
		{
			name:      "block rejects slash",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "a/b Block",
			want:      "",
		},
		{
			name:      "block rejects dot",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "foo.bar Block",
			want:      "",
		},
		{
			name:      "block accepts digits and underscores",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "config_v2 Block",
			want:      "config_v2",
		},
		// {Parent} template — single parent
		{
			name:      "parent single word",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "custom_key header Block",
			want:      "custom_key.header",
		},
		// {Parent} template — multi-word parent (uses last two parent words for disambiguation)
		{
			name:      "parent multi-word",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "customized_metric_specification metrics metric_stat Block",
			want:      "customized_metric_specification.metrics.metric_stat",
		},
		{
			name:      "parent three words",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "metric_data_query metric_stat metric Block",
			want:      "metric_data_query.metric_stat.metric",
		},
		// {Parent} template — no match without suffix
		{
			name:      "parent rejects single word",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "header Block",
			want:      "",
		},
		// {Path} template — single segment (acts like {Block})
		{
			name:      "path single segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "network Block",
			want:      "network",
		},
		// {Path} template — two segments dot-notation
		{
			name:      "path two segments",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "partition_spec.fields Block",
			want:      "partition_spec.fields",
		},
		// {Path} template — three segments dot-notation
		{
			name:      "path three segments",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "analyzer_configuration.internal_access_configuration.internal_access_analysis_rule Block",
			want:      "analyzer_configuration.internal_access_configuration.internal_access_analysis_rule",
		},
		// tfplugindocs canonical heading
		{
			name:      "tfplugindocs nested schema",
			templates: doc.HeadingTemplates{"Nested Schema for `{Path}`"},
			heading:   "Nested Schema for partition_spec.fields",
			want:      "partition_spec.fields",
		},
		{
			name:      "tfplugindocs nested schema single segment",
			templates: doc.HeadingTemplates{"Nested Schema for `{Path}`"},
			heading:   "Nested Schema for analyzer_configuration",
			want:      "analyzer_configuration",
		},
		{
			name:      "tfplugindocs nested schema deep",
			templates: doc.HeadingTemplates{"Nested Schema for `{Path}`"},
			heading:   "Nested Schema for analyzer_configuration.unused_access_configuration.analysis_rule.exclusions.resource_tags",
			want:      "analyzer_configuration.unused_access_configuration.analysis_rule.exclusions.resource_tags",
		},
		// {Path} rejects malformed dot-notation
		{
			name:      "path rejects leading dot",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   ".fields Block",
			want:      "",
		},
		{
			name:      "path rejects trailing dot",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "fields. Block",
			want:      "",
		},
		{
			name:      "path rejects double dot",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "a..b Block",
			want:      "",
		},
		{
			name:      "path rejects uppercase segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "Foo.bar Block",
			want:      "",
		},
		{
			name:      "path rejects space in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "partition spec.fields Block",
			want:      "",
		},
		{
			name:      "path rejects hyphen in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "foo-bar.fields Block",
			want:      "",
		},
		{
			name:      "path rejects slash in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "a/b.fields Block",
			want:      "",
		},
		{
			name:      "path rejects punctuation in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "foo+bar.fields Block",
			want:      "",
		},
		{
			name:      "path accepts digits",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "config_v2.field_1 Block",
			want:      "config_v2.field_1",
		},
		// Default templates accept the path forms
		{
			name:      "defaults accept dot-notation block",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "partition_spec.fields Block",
			want:      "partition_spec.fields",
		},
		{
			name:      "defaults accept tfplugindocs",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Nested Schema for partition_spec.fields",
			want:      "partition_spec.fields",
		},
		{
			name:      "defaults accept bare dot-notation",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "partition_spec.fields",
			want:      "partition_spec.fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.templates.Match(tt.heading)
			if got != tt.want {
				t.Errorf("Match(%q) = %q, want %q", tt.heading, got, tt.want)
			}
		})
	}
}

func TestParseWithTemplates_Strict(t *testing.T) {
	t.Parallel()

	// The test fixture uses "### `network` Block" which goldmark renders as "network Block"
	// With strict backtick_block template, this should match
	strict := doc.HeadingTemplates{"`{Block}` Block"}

	d, err := doc.ParseFileWithTemplates("../../testdata/docs/r/instance.html.markdown", strict)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, ok := d.ArgumentBlocks["network"]; !ok {
		t.Error("strict template should match 'network Block' heading")
	}
}

func TestParseWithTemplates_StrictRejectsBareName(t *testing.T) {
	t.Parallel()

	// Create a doc with bare name heading style (### statement)
	source := []byte(`# Resource: test_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

### statement

* ` + "`sid`" + ` - (Optional) Statement ID.
`)

	// Strict backtick_block should NOT match "statement" (bare name)
	strict := doc.HeadingTemplates{"`{Block}` Block"}
	d, err := doc.ParseWithTemplates(source, "test", strict)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, ok := d.ArgumentBlocks["statement"]; ok {
		t.Error("strict backtick_block template should NOT match bare 'statement' heading")
	}

	// But default templates should match it
	d2, err := doc.ParseWithTemplates(source, "test", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, ok := d2.ArgumentBlocks["statement"]; !ok {
		t.Error("default templates should match bare 'statement' heading")
	}
}

// TestParse_TfplugindocsStyle confirms the parser keys nested-block doc
// blocks by their full dot-notation path when the heading carries the
// path explicitly. This is the tfplugindocs canonical form ("Nested
// Schema for `<path>`") and the equivalent AWS-extended forms.
func TestParse_TfplugindocsStyle(t *testing.T) {
	t.Parallel()

	// A doc that mirrors the tfplugindocs layout: every nested block is
	// at heading level 3 with the full dot-path in backticks.
	source := []byte(`# Resource: aws_test

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

### Nested Schema for ` + "`config`" + `

* ` + "`enabled`" + ` - (Required) Enable.

### Nested Schema for ` + "`config.encryption`" + `

* ` + "`kms_key_id`" + ` - (Required) KMS key.

### Nested Schema for ` + "`config.encryption.rotation`" + `

* ` + "`interval_days`" + ` - (Required) Days.

### Nested Schema for ` + "`config.logging`" + `

* ` + "`bucket`" + ` - (Required) Bucket.
`)

	d, err := doc.Parse(source, "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cases := []struct {
		key      string
		wantAttr string
	}{
		{"config", "enabled"},
		{"config.encryption", "kms_key_id"},
		{"config.encryption.rotation", "interval_days"},
		{"config.logging", "bucket"},
	}

	for _, c := range cases {
		block, ok := d.ArgumentBlocks[c.key]
		if !ok {
			t.Errorf("ArgumentBlocks[%q] missing", c.key)
			continue
		}
		var found bool
		for _, a := range block.Attributes {
			if a.Name == c.wantAttr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ArgumentBlocks[%q] has no attribute %q (got %v)", c.key, c.wantAttr, block.Attributes)
		}
	}
}

// TestParse_RepeatedLeafDistinctPaths confirms that same-name nested
// blocks under different parents stay separated when authored with
// dot-notation. The appmesh "match" case is the canonical scenario:
// `spec.http_route.match` and `spec.grpc_route.match` are distinct
// schema blocks and must remain distinct in the doc model.
func TestParse_RepeatedLeafDistinctPaths(t *testing.T) {
	t.Parallel()

	source := []byte(`# Resource: aws_appmesh_test

## Argument Reference

* ` + "`spec`" + ` - (Required) Spec.

### ` + "`spec.http_route`" + ` Block

* ` + "`name`" + ` - (Required) Name.

### ` + "`spec.http_route.match`" + ` Block

* ` + "`method`" + ` - (Optional) HTTP method.
* ` + "`scheme`" + ` - (Optional) HTTP scheme.

### ` + "`spec.grpc_route`" + ` Block

* ` + "`name`" + ` - (Required) Name.

### ` + "`spec.grpc_route.match`" + ` Block

* ` + "`service_name`" + ` - (Optional) gRPC service name.
* ` + "`port`" + ` - (Optional) Port.
`)

	d, err := doc.Parse(source, "aws_appmesh_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	httpMatch, ok := d.ArgumentBlocks["spec.http_route.match"]
	if !ok {
		t.Fatalf("ArgumentBlocks missing spec.http_route.match — got keys %v", argumentKeys(d))
	}
	grpcMatch, ok := d.ArgumentBlocks["spec.grpc_route.match"]
	if !ok {
		t.Fatalf("ArgumentBlocks missing spec.grpc_route.match — got keys %v", argumentKeys(d))
	}

	// The two match blocks must be distinct DocBlocks with their own
	// attribute lists. No merging.
	if httpMatch == grpcMatch {
		t.Fatal("http_route.match and grpc_route.match share the same DocBlock — they should be distinct")
	}

	httpAttrs := attrNames(httpMatch.Attributes)
	grpcAttrs := attrNames(grpcMatch.Attributes)

	wantHTTP := []string{"method", "scheme"}
	wantGRPC := []string{"service_name", "port"}

	if !sameStrings(httpAttrs, wantHTTP) {
		t.Errorf("spec.http_route.match attributes = %v, want %v", httpAttrs, wantHTTP)
	}
	if !sameStrings(grpcAttrs, wantGRPC) {
		t.Errorf("spec.grpc_route.match attributes = %v, want %v", grpcAttrs, wantGRPC)
	}

	// And there must NOT be a leaf-keyed "match" block — the parser
	// keyed by full path, so the leaf form should not exist.
	if _, ok := d.ArgumentBlocks["match"]; ok {
		t.Errorf("ArgumentBlocks unexpectedly contains a leaf-keyed %q block; path-keyed headings should not collapse to leaf", "match")
	}
}

// argumentKeys returns the sorted keys of d.ArgumentBlocks for debug output.
func argumentKeys(d *doc.Document) []string {
	keys := make([]string, 0, len(d.ArgumentBlocks))
	for k := range d.ArgumentBlocks {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func docBlockAttrNames(b *doc.DocBlock) []string {
	if b == nil {
		return nil
	}
	names := make([]string, 0, len(b.Attributes))
	for _, a := range b.Attributes {
		names = append(names, a.Name)
	}
	return names
}

func hasAttr(b *doc.DocBlock, name string) bool {
	return slices.Contains(docBlockAttrNames(b), name)
}

const nestedItemsDoc = "## Attribute Reference\n\n" +
	"* `items` - List of objects. Each object has the following attributes:\n" +
	"    * `arn` - The ARN.\n" +
	"    * `dns_entry` - DNS block.\n" +
	"        * `domain_name` - The domain name.\n"

// TestParse_NestedCaptureOff confirms the default ignores inline-indented
// nested sub-bullets: no dot-path blocks are created.
func TestParse_NestedCaptureOff(t *testing.T) {
	t.Parallel()

	d, err := doc.Parse([]byte(nestedItemsDoc), "t")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.AttributeBlocks["items"]; ok {
		t.Error(`"items" block should not exist with capture off`)
	}
	if _, ok := d.AttributeBlocks["items.dns_entry"]; ok {
		t.Error(`"items.dns_entry" block should not exist with capture off`)
	}
	// The top-level `items` attribute is still recorded in the root block.
	if !hasAttr(d.AttributeBlocks[""], "items") {
		t.Error(`root block should still contain "items"`)
	}
}

// TestParse_NestedCaptureOn confirms inline-indented nested attributes are
// captured into dot-path keyed blocks at every depth.
func TestParse_NestedCaptureOn(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseWithOptions([]byte(nestedItemsDoc), "t",
		doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}

	items := d.AttributeBlocks["items"]
	if items == nil {
		t.Fatalf(`"items" block missing; blocks: %v`, attrBlockKeys(d))
	}
	if !hasAttr(items, "arn") || !hasAttr(items, "dns_entry") {
		t.Errorf(`"items" block should contain arn and dns_entry, got %v`, docBlockAttrNames(items))
	}

	dns := d.AttributeBlocks["items.dns_entry"]
	if dns == nil {
		t.Fatalf(`"items.dns_entry" block missing; blocks: %v`, attrBlockKeys(d))
	}
	if !hasAttr(dns, "domain_name") {
		t.Errorf(`"items.dns_entry" should contain domain_name, got %v`, docBlockAttrNames(dns))
	}
}

func blkAttrNames(b *doc.DocBlock) []string {
	if b == nil {
		return nil
	}
	names := make([]string, 0, len(b.Attributes))
	for _, a := range b.Attributes {
		names = append(names, a.Name)
	}
	return names
}

// TestNestedBlockLeadIn covers the legacy prose block introducer.
func TestNestedBlockLeadIn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"The `catalog_properties[0].data_lake_access_properties[0]` block also exports:", "catalog_properties.data_lake_access_properties", true},
		{"The `parent[0].child[0]` block supports the following:", "parent.child", true},
		{"The `foo` block supports:", "", false},      // single-level: use a heading
		{"The `foo` block is deprecated.", "", false}, // no supports/exports + colon
		{"This resource exports the following attributes:", "", false},
		{"Some prose about a block.", "", false},
	}
	for _, c := range cases {
		got, ok := doc.NestedBlockLeadIn(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("NestedBlockLeadIn(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestParse_ProseLeadInAttachesToPath: bullets after a prose lead-in are
// attributed to the dot-path block, not the root (issue #53).
func TestParse_ProseLeadInAttachesToPath(t *testing.T) {
	t.Parallel()

	md := "## Attribute Reference\n\n" +
		"* `id` - The ID.\n\n" +
		"The `catalog_properties[0].data_lake_access_properties[0]` block also exports:\n\n" +
		"* `managed_workgroup_name` - Managed workgroup name.\n" +
		"* `status_message` - Status message.\n"

	d, err := doc.ParseWithOptions([]byte(md), "t", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	b := d.AttributeBlocks["catalog_properties.data_lake_access_properties"]
	if b == nil {
		t.Fatal(`expected block keyed "catalog_properties.data_lake_access_properties"`)
	}
	got := blkAttrNames(b)
	if len(got) != 2 || got[0] != "managed_workgroup_name" {
		t.Errorf("block attrs = %v, want [managed_workgroup_name status_message]", got)
	}
	// The bullets must NOT have leaked into the root attribute block.
	for _, a := range blkAttrNames(d.AttributeBlocks[""]) {
		if a == "managed_workgroup_name" {
			t.Error("lead-in attributes leaked into the root block")
		}
	}
}
