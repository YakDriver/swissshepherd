// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// TestParse_LinkFragmentDecodingOrder guards that in-page link fragments are
// decoded in Goldmark's rendered-href order (UnescapePunctuations,
// ResolveNumericReferences, ResolveEntityNames) rather than resolving named
// entities first. For "#x&amp;#95;y" Goldmark renders the literal target
// "x&#95;y"; resolving the named entity first would instead recursively decode
// the exposed "&#95;" to "_", yielding "x_y" and wrongly accepting a dead link.
func TestParse_LinkFragmentDecodingOrder(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `a` - (Optional) See [x](#x&amp;#95;y).\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}

	var got string
	for _, l := range d.InPageLinks {
		got = l.Fragment
	}
	if got != "x&#95;y" {
		t.Errorf("fragment = %q, want %q (must not over-decode to \"x_y\")", got, "x&#95;y")
	}
	if got == "x_y" {
		t.Error("named entities resolved before numeric references: fragment over-decoded to \"x_y\"")
	}
}
