// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
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

	blocks := d.Blocks()
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

	blocks := d.Blocks()

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

	blocks := d.Blocks()

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

	blocks := d.Blocks()
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

	blocks := d.Blocks()

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
