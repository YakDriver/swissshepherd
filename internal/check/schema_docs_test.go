// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// --- SchemaDocsRule magic-value config tests ---------------------------

func TestSchemaDocsRule_DefaultImplicitAttributesSkipped(t *testing.T) {
	t.Parallel()

	// "id" and "tags_all" are in DefaultImplicitAttributes and must never
	// appear as "not documented" errors even when absent from the doc.
	rs := &schema.ResourceSchema{
		Name: "test_thing",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "id", Computed: true},
					{Name: "tags_all", Computed: true},
					{Name: "name", Required: true},
				},
			},
		},
	}
	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
`), "test_thing")

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("unexpected error (id/tags_all should be implicit): %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_CustomImplicitAttributes(t *testing.T) {
	t.Parallel()

	// Override implicit list to include "region" — a provider-injected attr.
	rs := &schema.ResourceSchema{
		Name: "test_thing",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "region", Optional: true},
					{Name: "name", Required: true},
				},
			},
		},
	}
	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
`), "test_thing")

	rule := &check.SchemaDocsRule{
		IgnoreDeprecated:   true,
		ImplicitAttributes: []string{"id", "tags_all", "region"},
	}
	results := rule.Check(check.CheckContext{Resource: "test_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("region should be implicit with custom list; got: %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_CustomSkipBlocks(t *testing.T) {
	t.Parallel()

	// Override skip_blocks to also skip "network" — useful for providers
	// that document network blocks in a separate guide.
	rs := &schema.ResourceSchema{
		Name: "test_thing",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"network"},
			},
			"network": {
				Attributes: []schema.Attribute{{Name: "subnet_id", Required: true}},
			},
		},
	}
	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
`), "test_thing")

	rule := &check.SchemaDocsRule{
		IgnoreDeprecated: true,
		SkipBlocks:       []string{"timeouts", "network"},
	}
	results := rule.Check(check.CheckContext{Resource: "test_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("network block should be skipped; got: %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_DefaultSkipBlocksContainsTimeouts(t *testing.T) {
	t.Parallel()

	if !slices.Contains(check.DefaultSkipBlocks, "timeouts") {
		t.Error("DefaultSkipBlocks must contain 'timeouts'")
	}
}

// --- SchemaDocsRule magic-value config tests -----------------------

func TestSchemaDocsRule_DefaultPrefixesFire(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
`), "test")

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'The ' prefix, got %d: %v", len(results), resultMessages(results))
	}
}

func TestSchemaDocsRule_CustomBadPrefixes(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`mode`"+` - (Optional) FORBIDDEN start.
* `+"`name`"+` - (Required) The name of the thing.
`), "test")

	// Replace the default list with a custom one that only flags "FORBIDDEN".
	rule := &check.SchemaDocsRule{BadPrefixes: []string{"FORBIDDEN "}}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 1 {
		t.Fatalf("expected 1 result for custom prefix, got %d: %v", len(results), resultMessages(results))
	}
	if results[0].Message == "" {
		t.Error("result should have message set")
	}
}

func TestSchemaDocsRule_EmptyBadPrefixesMatchesNothing(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
`), "test")

	rule := &check.SchemaDocsRule{BadPrefixes: []string{}}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 0 {
		t.Errorf("empty BadPrefixes should match nothing, got %d results", len(results))
	}
}

func TestSchemaDocsRule_DefaultPrefixesMatchesTfproviderdocs(t *testing.T) {
	t.Parallel()

	// Pin the default list so a future refactor can't silently drop a prefix
	// that AWS CI depends on.
	want := []string{
		"A ", "An ", "The ", "This ", "It ",
		"Indicates ", "Specifies ", "Describes ", "Defines ",
		"Contains ", "Determines ", "Identifies ", "Represents ", "Denotes ", "Holds ", "Used ",
	}
	if !slices.Equal(check.DefaultBadDescriptionPrefixes, want) {
		t.Errorf("DefaultBadDescriptionPrefixes = %v, want %v", check.DefaultBadDescriptionPrefixes, want)
	}
}

// TestSchemaDocsRule_DefaultPrefixes_WeakVsLegitimate confirms the expanded
// default list flags weak/redundant/meta starts (This, It, Contains,
// Determines, Identifies, Represents, Denotes, Holds, Used) while leaving
// legitimate noun-phrase and boolean starts alone.
func TestSchemaDocsRule_DefaultPrefixes_WeakVsLegitimate(t *testing.T) {
	t.Parallel()

	descFlagged := func(desc string) bool {
		d, err := doc.Parse([]byte("## Argument Reference\n\n* `x` - (Optional) "+desc+"\n"), "t")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range (&check.SchemaDocsRule{}).Check(check.CheckContext{Resource: "t", Doc: d}) {
			if strings.Contains(r.Message, "should not start with") {
				return true
			}
		}
		return false
	}

	weak := []string{
		"This is the Id or ARN of the service.",
		"It is the name of the hosted zone.",
		"Contains the list of rules.",
		"Determines whether to enable the feature.",
		"Identifies the resource uniquely.",
		"Represents the current state.",
		"Denotes the type of association.",
		"Holds the encoded value.",
		"Used to select the mode.",
	}
	for _, d := range weak {
		if !descFlagged(d) {
			t.Errorf("expected weak start to be flagged: %q", d)
		}
	}

	// Legitimate starts that must NOT be flagged.
	fine := []string{
		"Whether to enable the feature.",
		"Set of ARNs to associate.",
		"List of names for the group.",
		"Map of tags to assign.",
		"Configuration block for logging.",
		"Region where this resource is managed.",
		"Name of the thing.", // starts with a real noun, not a blocked word
	}
	for _, d := range fine {
		if descFlagged(d) {
			t.Errorf("legitimate start should not be flagged: %q", d)
		}
	}
}

// --- SchemaDocsRule magic-value config tests ----------------------------

func TestSchemaDocsRule_DefaultsAllEnabled(t *testing.T) {
	t.Parallel()

	src := "# Resource: test\n\n## Argument Reference\n\n```\ncode block here\n```\n\n* `name` - (Required) Name.\n"

	d, err := doc.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}

	rule := &check.SchemaDocsRule{} // all nil → all enabled
	results := rule.Check(check.CheckContext{Resource: "test", Doc: d})

	if len(results) == 0 {
		t.Error("zero-value SchemaDocsRule should flag the code block (default enabled)")
	}
}

func TestSchemaDocsRule_DisableNoCodeBlocks(t *testing.T) {
	t.Parallel()

	f := false
	rule := &check.SchemaDocsRule{NoCodeBlocks: &f}

	src := "# Resource: test\n\n## Argument Reference\n\n```\ncode block here\n```\n\n* `name` - (Required) Name.\n"

	d, err := doc.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}

	results := rule.Check(check.CheckContext{Resource: "test", Doc: d})
	for _, r := range results {
		if r.Rule == "schema_docs" && strings.Contains(r.Message, "code block") {
			t.Errorf("NoCodeBlocks=false should suppress code-block errors; got: %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_NoCodeBlocksRecognizesTildeFence(t *testing.T) {
	t.Parallel()

	// A ~~~ fence is a code block too; NoCodeBlocks must flag it the same
	// as a ``` fence, not scan its content as attribute-list prose.
	src := "# Resource: test\n\n## Argument Reference\n\n~~~\ncode block here\n~~~\n\n* `name` - (Required) Name.\n"

	d, err := doc.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test", Doc: d})

	found := false
	for _, r := range results {
		if r.Rule == "schema_docs" && strings.Contains(r.Message, "code block") {
			found = true
		}
	}
	if !found {
		t.Error("expected a code-block finding for the ~~~ fence")
	}
}

func TestSchemaDocsRule_MismatchedFenceCharacterDoesNotCloseFence(t *testing.T) {
	t.Parallel()

	// A ~~~ fence is invisible to a check that only recognizes ```, so an
	// attribute-list-shaped line inside it (plausible as example output)
	// gets scanned as a real attribute, and the fence's own closing ~~~
	// then looks like it interrupts that fabricated list. With ~~~
	// recognized, the whole block is skipped and the only finding is the
	// expected "code block in argument/attribute section" error.
	src := "# Resource: test\n\n## Argument Reference\n\n~~~\n* `fake` - not a real attribute, just example output\n~~~\n\n* `name` - (Required) Name.\n"

	d, err := doc.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test", Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "interrupted") {
			t.Errorf("content inside the ~~~ fence must not produce an interrupted-list finding: %s", r.Message)
		}
	}
	if len(results) != 1 || !strings.Contains(results[0].Message, "code block") {
		t.Errorf("expected exactly 1 finding (the ~~~ code-block error), got: %+v", results)
	}
}

func TestOrdering_FixtureInOrder(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("unexpected ordering error: %s", r.Message)
		}
	}
}

func TestOrdering_FixtureArgumentsUnordered(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_unordered.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})

	if len(results) == 0 {
		t.Fatal("expected ordering errors, got none")
	}

	found := false
	for _, r := range results {
		if strings.Contains(r.Message, "argument") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected an ordering error in argument section")
	}
}

func TestOrdering_FixtureAttributesUnordered(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_unordered.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})

	foundAttr := false
	for _, r := range results {
		if strings.Contains(r.Message, "attribute") {
			foundAttr = true
			break
		}
	}
	if !foundAttr {
		t.Error("expected an ordering error in attribute section")
	}
}

func TestDescription_FixtureGood(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})

	if len(results) > 0 {
		t.Errorf("expected no style errors, got %d: %v", len(results), results[0].Message)
	}
}

func TestDescription_FixtureBadPrefixes(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_bad_style.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})

	// Should flag: "The name", "A description", "Specifies the mode", "Indicates the type", "An ARN"
	if len(results) < 4 {
		t.Errorf("expected at least 4 style errors, got %d", len(results))
		for _, r := range results {
			t.Logf("  %s", r.Message)
		}
	}

	// Verify specific attributes are flagged
	flagged := make(map[string]bool)
	for _, r := range results {
		for _, name := range []string{"name", "description", "mode", "type", "arn"} {
			if strings.Contains(r.Message, `"`+name+`"`) {
				flagged[name] = true
			}
		}
	}

	for _, want := range []string{"name", "description", "mode", "type", "arn"} {
		if !flagged[want] {
			t.Errorf("expected %q to be flagged for bad description style", want)
		}
	}
}

// TestDescription_NestedObjectWeakPrefix: description style now reaches
// nested object fields — an "arn" documented as "The ARN." is flagged.
func TestDescription_NestedObjectWeakPrefix(t *testing.T) {
	t.Parallel()

	rs := objectAttrSchema()
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		DataSources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	md := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"* `items` - List of objects. Each object has the following attributes:\n" +
		"    * `arn` - The ARN value.\n" +
		"    * `foo` - Foo value.\n"

	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{
		Resource: "aws_test", Schema: rs, Doc: parseCaptured(t, md),
	})

	// Both fields covered — no missing-field error.
	if hasMessage(results, `block "items"`) && hasMessage(results, "should be documented") {
		t.Errorf("unexpected coverage error:\n  %s", joinMessages(results))
	}
	// arn's weak "The" start is flagged inside the nested block.
	if !hasMessage(results, `attribute "arn" description should not start with "The"`) {
		t.Errorf("expected weak-description finding for nested arn, got:\n  %s", joinMessages(results))
	}
}

// TestHeading_PathKeyedPreferredStyle confirms that
// path-keyed doc blocks (e.g. `spec.grpc_route.match`) participate in
// the preferred-style check rather than being silently skipped. The
// schema lookup uses the leaf name so the block is recognized as
// schema-present; the ambiguity branch is bypassed because the
// heading is already in dot-path form.
func TestHeading_PathKeyedPreferredStyle(t *testing.T) {
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

func TestHeading_PathKeyedBadStyleStillWarns(t *testing.T) {
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
	// heading suggesting the `<full.path>` Block form. The suggestion
	// must contain the full dot-path "foo.bar", NOT just the leaf
	// "bar" (which would drop the disambiguator and reintroduce the
	// ambiguity this PR fixes).
	var found bool
	for _, r := range results {
		if !strings.Contains(r.Message, "foo.bar") || !strings.Contains(r.Message, "should be") {
			continue
		}
		// The suggested text is the `should be %q` portion; the
		// message contains "foo.bar" twice already (block name and
		// heading text). Look specifically for the suggested form
		// to confirm it preserves the full path.
		if !strings.Contains(r.Message, "`foo.bar` Block") {
			t.Errorf("preferred-style suggestion drops dot-path; expected \"`foo.bar` Block\" in suggestion, got: %s", r.Message)
		}
		found = true
		break
	}
	if !found {
		t.Errorf("expected preferred-style finding on `foo.bar` heading; got %d results", len(results))
		for _, r := range results {
			t.Logf("  result: %s", r.Message)
		}
	}
}

// TestDescription_OneFindingPerBullet: findings are deduped by bullet, not
// by section key and field name (#86). Same-named bullets in the two
// reference sections, or twice under one key, are separate defects; one
// bullet credited to every alias of a combined heading is one.
func TestDescription_OneFindingPerBullet(t *testing.T) {
	t.Parallel()

	off := false
	testCases := map[string]struct {
		md   string
		want []string // finding messages, in order
	}{
		"same name in both reference sections": {
			md: "## Argument Reference\n\n* `id` - (Optional) The ID to use.\n\n## Attribute Reference\n\n* `id` - The ID.\n",
			want: []string{
				`attribute "id" description should not start with "The" (block "(root)") line 3`,
				`attribute "id" description should not start with "The" (block "(root)") line 7`,
			},
		},
		"same name twice under one key": {
			md: "## Argument Reference\n\n* `a` - (Optional) A.\n\n### `a` Block\n\n* `x` - (Optional) The X.\n\n### `a` Block\n\n* `x` - (Optional) The other X.\n",
			want: []string{
				`attribute "x" description should not start with "The" (block "a") line 7`,
				`attribute "x" description should not start with "The" (block "a") line 11`,
			},
		},
		"combined heading": {
			md: "## Argument Reference\n\n* `b` - (Optional) B.\n* `a` - (Optional) A.\n\n### `b` and `a`\n\n* `x` - (Optional) The X.\n",
			want: []string{
				`attribute "x" description should not start with "The" (block "a") line 8`,
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.ParseWithTemplates([]byte(tc.md), "aws_thing", doc.HeadingTemplates{"`{Block}` Block", "`{Block}`"})
			if err != nil {
				t.Fatal(err)
			}
			rule := check.SchemaDocsRule{Coverage: &off, Ordering: &off, Format: &off, Byline: &off, Heading: &off, Labels: &off, Deprecated: &off}
			// Run repeatedly: which alias names the finding must not depend
			// on map order.
			for range 20 {
				var got []string
				for _, r := range rule.Check(check.CheckContext{Resource: "aws_thing", Doc: d}) {
					got = append(got, fmt.Sprintf("%s line %d", r.Message, r.Line))
				}
				if !slices.Equal(got, tc.want) {
					t.Fatalf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(tc.want, "\n  "))
				}
			}
		})
	}
}

// TestDescription_OrphanedBullets: bullets that belong to no section still
// get the description check, which doesn't depend on the block. The finding
// names the heading instead. A prose-introduced list belongs to the block it
// names, so its finding names that block (#77).
func TestDescription_OrphanedBullets(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                       {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"action"}},
		"action":                 {ChildBlocks: []string{"action.cloudwatch_logs"}},
		"action.cloudwatch_logs": {Attributes: optional("role_arn")},
	}}
	testCases := map[string]struct {
		body string
		want string
	}{
		"under an unparseable heading": {
			body: "### Waiting for Capacity\n\n* `role_arn` - (Optional) The role.\n",
			want: `attribute "role_arn" description should not start with "The" (under heading "Waiting for Capacity")`,
		},
		"in a prose-introduced list": {
			body: "* `name` - (Required) Name.\n\nThe `cloudwatch_logs` object takes the following arguments:\n\n* `role_arn` - (Optional) The role.\n",
			want: `attribute "role_arn" description should not start with "The" (block "cloudwatch_logs")`,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.ParseWithTemplates([]byte("## Argument Reference\n\n"+tc.body), "aws_thing", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
			if err != nil {
				t.Fatal(err)
			}
			results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
			if !hasMessage(results, tc.want) {
				t.Errorf("missing %q in:\n  %s", tc.want, joinMessages(results))
			}
		})
	}
}

// TestSchemaDocsRule_Overrides: an override replaces sub-check toggles for
// the targets it lists, before any sub-check runs (#75). A computed-only
// argument labeled (Optional) is left by labels to coverage; with coverage
// switched off by an override, labels must report it instead.
func TestSchemaDocsRule_Overrides(t *testing.T) {
	t.Parallel()

	const md = "## Argument Reference\n\n* `name` - (Required) Name.\n* `arn` - (Optional) ARN.\n\n## Attribute Reference\n\n* `id` - ID.\n"
	rs := &schema.ResourceSchema{Name: "aws_thing", Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}, {Name: "arn", Computed: true}, {Name: "id", Computed: true}}},
	}}
	const coverageMsg = `Read-Only attribute "arn" should be documented in Attribute Reference section`
	const labelsMsg = `argument "arn" is labeled (Optional) but is computed-only in the schema; move it to Attribute Reference and remove the label`
	off, on := false, true

	testCases := map[string]struct {
		target, typ string
		overrides   []config.Override
		want        []string
		wantNot     []string
	}{
		"no override": {
			target: "aws_thing", typ: "resource",
			want:    []string{coverageMsg},
			wantNot: []string{labelsMsg},
		},
		"coverage off for this target": {
			target: "aws_thing", typ: "resource",
			overrides: []config.Override{{Targets: []string{"aws_thing"}, Coverage: &off}},
			want:      []string{labelsMsg},
			wantNot:   []string{coverageMsg},
		},
		"qualified target of another type": {
			target: "aws_thing", typ: "resource",
			overrides: []config.Override{{Targets: []string{"data_source/aws_thing"}, Coverage: &off}},
			want:      []string{coverageMsg},
			wantNot:   []string{labelsMsg},
		},
		"qualified target of this type": {
			target: "aws_thing", typ: "resource",
			overrides: []config.Override{{Targets: []string{"resource/aws_thing"}, Coverage: &off}},
			want:      []string{labelsMsg},
			wantNot:   []string{coverageMsg},
		},
		"another target": {
			target: "aws_other", typ: "resource",
			overrides: []config.Override{{Targets: []string{"aws_thing"}, Coverage: &off}},
			want:      []string{coverageMsg},
		},
		// An unset toggle keeps the check's value; a set one can re-enable.
		"unset toggle keeps the check's value": {
			target: "aws_thing", typ: "resource",
			overrides: []config.Override{{Targets: []string{"aws_thing"}, Ordering: &on}},
			want:      []string{coverageMsg},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.Parse([]byte(md), tc.target)
			if err != nil {
				t.Fatal(err)
			}
			rule := &check.SchemaDocsRule{Description: &off, Format: &off, Byline: &off, Heading: &off, Overrides: tc.overrides}
			results := rule.Check(check.CheckContext{Resource: tc.target, Type: &config.Type{Name: tc.typ}, Schema: rs, Doc: d})
			for _, w := range tc.want {
				if !hasMessage(results, w) {
					t.Errorf("missing %q in:\n  %s", w, joinMessages(results))
				}
			}
			for _, w := range tc.wantNot {
				if hasMessage(results, w) {
					t.Errorf("unexpected %q in:\n  %s", w, joinMessages(results))
				}
			}
			if rule.Coverage != nil {
				t.Error("Check modified the rule's own toggles")
			}
		})
	}
}
