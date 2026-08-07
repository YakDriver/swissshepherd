// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"slices"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

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
