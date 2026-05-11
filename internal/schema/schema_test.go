// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package schema_test

import (
	"slices"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/schema"
)

func TestLoadFile(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got := len(ps.Resources); got != 1 {
		t.Fatalf("expected 1 resource, got %d", got)
	}
	if got := len(ps.DataSources); got != 1 {
		t.Fatalf("expected 1 data source, got %d", got)
	}
}

func TestLoadFile_ResourceBlocks(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	rs := ps.Resources["test_instance"]
	if rs == nil {
		t.Fatal("test_instance not found in resources")
	}

	// Root block
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

	// Network block
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

	// Timeouts block
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

	_, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

func attrNameSlice(attrs []schema.Attribute) []string {
	names := make([]string, len(attrs))
	for i, a := range attrs {
		names[i] = a.Name
	}
	return names
}
