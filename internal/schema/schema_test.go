// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package schema_test

import (
	"slices"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/schema"
)

const (
	fixtureSchema   = "../../testdata/schema/test_provider.json"
	fixtureProvider = "registry.terraform.io/hashicorp/test"
)

func TestLoadFile(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile(fixtureSchema, fixtureProvider)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cases := map[string]int{
		"Resources":     len(ps.Resources),
		"DataSources":   len(ps.DataSources),
		"Ephemerals":    len(ps.Ephemerals),
		"ListResources": len(ps.ListResources),
		"Actions":       len(ps.Actions),
		"Functions":     len(ps.Functions),
	}
	for name, got := range cases {
		if got != 1 {
			t.Errorf("%s: got %d entries, want 1", name, got)
		}
	}
}

func TestLoadFile_ResourceBlocks(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile(fixtureSchema, fixtureProvider)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	rs := ps.Resources["test_instance"]
	if rs == nil {
		t.Fatal("test_instance not found in resources")
	}

	root := rs.Blocks[""]
	if root == nil {
		t.Fatal("root block not found")
	}

	attrNames := attrNameSlice(root.Attributes)
	for _, want := range []string{"name", "description", "arn", "tags", "tags_all", "id"} {
		if !slices.Contains(attrNames, want) {
			t.Errorf("root block missing attribute %q, got %v", want, attrNames)
		}
	}

	if !slices.Contains(root.ChildBlocks, "network") {
		t.Errorf("root block missing child block 'network', got %v", root.ChildBlocks)
	}

	network := rs.Blocks["network"]
	if network == nil {
		t.Fatal("network block not found")
	}

	netAttrs := attrNameSlice(network.Attributes)
	for _, want := range []string{"subnet_id", "security_groups", "private_ip"} {
		if !slices.Contains(netAttrs, want) {
			t.Errorf("network block missing attribute %q, got %v", want, netAttrs)
		}
	}

	timeouts := rs.Blocks["timeouts"]
	if timeouts == nil {
		t.Fatal("timeouts block not found")
	}

	toAttrs := attrNameSlice(timeouts.Attributes)
	if !slices.Contains(toAttrs, "create") || !slices.Contains(toAttrs, "delete") {
		t.Errorf("timeouts block missing expected attributes, got %v", toAttrs)
	}
}

func TestLoadFile_ProviderNotFound(t *testing.T) {
	t.Parallel()

	_, err := schema.LoadFile(fixtureSchema, "registry.terraform.io/hashicorp/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

// TestLoadFile_AllKindsFlattened confirms that ephemeral resources, list
// resources, and actions — all block-based categories — flatten into the
// same ResourceSchema shape as regular resources. This is the contract the
// runner will lean on in chunk 3.3 when it iterates types uniformly.
func TestLoadFile_AllKindsFlattened(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile(fixtureSchema, fixtureProvider)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tests := map[string]struct {
		rs           *schema.ResourceSchema
		wantAttrs    []string
		wantNotAttrs []string
	}{
		"ephemeral test_secret": {
			rs:        ps.Ephemerals["test_secret"],
			wantAttrs: []string{"name", "value"},
		},
		"list_resource test_instances": {
			rs:        ps.ListResources["test_instances"],
			wantAttrs: []string{"filter", "names"},
		},
		"action test_reboot": {
			rs:        ps.Actions["test_reboot"],
			wantAttrs: []string{"instance_id", "force"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if tt.rs == nil {
				t.Fatal("target missing from loaded schema")
			}
			root := tt.rs.Blocks[""]
			if root == nil {
				t.Fatal("root block missing")
			}
			got := attrNameSlice(root.Attributes)
			for _, want := range tt.wantAttrs {
				if !slices.Contains(got, want) {
					t.Errorf("root block missing attribute %q, got %v", want, got)
				}
			}
		})
	}
}

// TestLoadFile_FunctionShape confirms the minimal FunctionSchema fields
// populate correctly — including a variadic parameter, since tfjson models
// that distinctly from positional parameters.
func TestLoadFile_FunctionShape(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile(fixtureSchema, fixtureProvider)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	fn := ps.Function("test_format")
	if fn == nil {
		t.Fatal("test_format function not found")
	}
	if fn.Description != "Format a string with named arguments." {
		t.Errorf("Description = %q", fn.Description)
	}
	wantParams := []string{"template", "count"}
	if !slices.Equal(fn.ParameterNames, wantParams) {
		t.Errorf("ParameterNames = %v, want %v", fn.ParameterNames, wantParams)
	}
	if fn.VariadicParameter != "args" {
		t.Errorf("VariadicParameter = %q, want %q", fn.VariadicParameter, "args")
	}
}

// TestTargetNames pins the ProviderSchema.TargetNames accessor that chunk
// 3.3's runner will use to iterate targets by kind. Every built-in block
// kind gets exercised plus the two kinds that intentionally return nil.
func TestTargetNames(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile(fixtureSchema, fixtureProvider)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tests := map[string]struct {
		kind string
		want []string
	}{
		"resource":            {schema.KindResource, []string{"test_instance"}},
		"data_source":         {schema.KindDataSource, []string{"test_instance"}},
		"ephemeral":           {schema.KindEphemeral, []string{"test_secret"}},
		"list_resource":       {schema.KindListResource, []string{"test_instances"}},
		"action":              {schema.KindAction, []string{"test_reboot"}},
		"function":            {schema.KindFunction, []string{"test_format"}},
		"none returns nil":    {schema.KindNone, nil},
		"unknown returns nil": {"widget", nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := ps.TargetNames(tt.kind)
			if !slices.Equal(got, tt.want) {
				t.Errorf("TargetNames(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

// TestTargetNames_SortedOutput documents the ordering contract. Map
// iteration in Go is explicitly randomized; stable output here saves
// every caller from sorting on their own.
func TestTargetNames_SortedOutput(t *testing.T) {
	t.Parallel()

	ps := &schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{
			"zzz_last":   {Name: "zzz_last"},
			"aaa_first":  {Name: "aaa_first"},
			"mmm_middle": {Name: "mmm_middle"},
		},
	}
	got := ps.TargetNames(schema.KindResource)
	want := []string{"aaa_first", "mmm_middle", "zzz_last"}
	if !slices.Equal(got, want) {
		t.Errorf("TargetNames(resource) = %v, want %v", got, want)
	}
}

// TestResourceSchemaFor exercises the generic kind-indexed accessor rules
// will lean on: it returns the right record for a known (kind, name) pair
// and nil for every "this shouldn't exist" path without panicking.
func TestResourceSchemaFor(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile(fixtureSchema, fixtureProvider)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tests := map[string]struct {
		kind     string
		name     string
		wantName string // empty means expect nil
	}{
		"resource hit":         {schema.KindResource, "test_instance", "test_instance"},
		"data_source hit":      {schema.KindDataSource, "test_instance", "test_instance"},
		"ephemeral hit":        {schema.KindEphemeral, "test_secret", "test_secret"},
		"list_resource hit":    {schema.KindListResource, "test_instances", "test_instances"},
		"action hit":           {schema.KindAction, "test_reboot", "test_reboot"},
		"function returns nil": {schema.KindFunction, "test_format", ""},
		"none returns nil":     {schema.KindNone, "whatever", ""},
		"unknown kind nil":     {"widget", "anything", ""},
		"missing name nil":     {schema.KindResource, "nonexistent", ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rs := ps.ResourceSchemaFor(tt.kind, tt.name)
			if tt.wantName == "" {
				if rs != nil {
					t.Errorf("expected nil, got %+v", rs)
				}
				return
			}
			if rs == nil {
				t.Fatal("expected non-nil schema")
			}
			if rs.Name != tt.wantName {
				t.Errorf("schema.Name = %q, want %q", rs.Name, tt.wantName)
			}
		})
	}
}

// TestBlockKindsCoversEveryBlockKind is a tripwire: if someone adds a new
// kind constant and forgets to include it in BlockKinds (or intentionally
// excludes a non-block kind), this test forces them to re-state the intent.
func TestBlockKindsCoversEveryBlockKind(t *testing.T) {
	t.Parallel()

	// Kinds that DO have block schemas.
	wantBlock := []string{
		schema.KindResource,
		schema.KindDataSource,
		schema.KindEphemeral,
		schema.KindListResource,
		schema.KindAction,
	}
	slices.Sort(wantBlock)

	got := slices.Clone(schema.BlockKinds)
	slices.Sort(got)

	if !slices.Equal(got, wantBlock) {
		t.Errorf("BlockKinds = %v, want %v", got, wantBlock)
	}

	// Kinds that MUST NOT appear in BlockKinds.
	forbidden := []string{schema.KindFunction, schema.KindNone}
	for _, k := range forbidden {
		if slices.Contains(schema.BlockKinds, k) {
			t.Errorf("BlockKinds must not include %q", k)
		}
	}
}

func attrNameSlice(attrs []schema.Attribute) []string {
	names := make([]string, len(attrs))
	for i, a := range attrs {
		names[i] = a.Name
	}
	return names
}

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
