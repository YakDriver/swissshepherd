// Copyright (c) YakDriver, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"slices"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

func TestParseFile_Complete(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Root argument block should have arguments
	root := d.ArgumentBlocks[""]
	if root == nil {
		t.Fatal("root argument block not found")
	}

	rootNames := docAttrNames(root.Attributes)
	for _, want := range []string{"name", "description", "network", "tags"} {
		if !slices.Contains(rootNames, want) {
			t.Errorf("root block missing attribute %q, got %v", want, rootNames)
		}
	}

	// Check required/optional
	for _, attr := range root.Attributes {
		switch attr.Name {
		case "name":
			if !attr.Required {
				t.Error("name should be Required")
			}
		case "description":
			if !attr.Optional {
				t.Error("description should be Optional")
			}
		}
	}

	// Network block in arguments
	network := d.ArgumentBlocks["network"]
	if network == nil {
		t.Fatal("network argument block not found")
	}

	netNames := docAttrNames(network.Attributes)
	for _, want := range []string{"subnet_id", "security_groups"} {
		if !slices.Contains(netNames, want) {
			t.Errorf("network block missing attribute %q, got %v", want, netNames)
		}
	}

	// Attribute Reference section
	attrRoot := d.AttributeBlocks[""]
	if attrRoot == nil {
		t.Fatal("root attribute block not found")
	}

	attrNames := docAttrNames(attrRoot.Attributes)
	if !slices.Contains(attrNames, "arn") {
		t.Errorf("attribute section missing 'arn', got %v", attrNames)
	}
}

func TestParseFile_Descriptions(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	root := d.ArgumentBlocks[""]
	for _, attr := range root.Attributes {
		if attr.Name == "name" {
			if attr.Description == "" {
				t.Error("name attribute should have a description")
			}
			if attr.Description != "Name of the instance." {
				t.Errorf("unexpected description: %q", attr.Description)
			}
			break
		}
	}
}

func TestParseFile_DataSource(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/d/instance.html.markdown")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	root := d.ArgumentBlocks[""]
	if root == nil {
		t.Fatal("root argument block not found")
	}

	rootNames := docAttrNames(root.Attributes)
	if !slices.Contains(rootNames, "name") {
		t.Errorf("root block missing 'name', got %v", rootNames)
	}

	attrRoot := d.AttributeBlocks[""]
	if attrRoot == nil {
		t.Fatal("root attribute block not found")
	}
	attrNames := docAttrNames(attrRoot.Attributes)
	if !slices.Contains(attrNames, "arn") {
		t.Errorf("attribute section missing 'arn', got %v", attrNames)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	t.Parallel()

	_, err := doc.ParseFile("../../testdata/docs/r/nonexistent.html.markdown")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func docAttrNames(attrs []doc.DocAttribute) []string {
	names := make([]string, len(attrs))
	for i, a := range attrs {
		names[i] = a.Name
	}
	return names
}
