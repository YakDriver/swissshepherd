// Copyright IBM Corp. 2019, 2026
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

// TestParse_FrontmatterDoesNotProduceSetextHeadings is a regression test for
// the case where Goldmark, lacking a meta extension, treats the closing "---"
// of YAML frontmatter as a setext H2 underline for the preceding paragraph.
// Stripping the frontmatter region before parsing keeps this from happening.
func TestParse_FrontmatterDoesNotProduceSetextHeadings(t *testing.T) {
	t.Parallel()

	source := []byte(`---
subcategory: "Transcribe"
layout: "aws"
page_title: "AWS: aws_transcribe_start_transcription_job"
description: |-
  Starts an Amazon Transcribe transcription job.
---

# Action: aws_transcribe_start_transcription_job

## Argument Reference

* ` + "`name`" + ` - (Required) Job name.
`)

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if d.Sections.Title == nil {
		t.Fatal("Title section should be discovered")
	}
	if d.Sections.Arguments == nil {
		t.Fatal("Argument Reference section should be discovered")
	}
	if got := len(d.Sections.UnknownHeadings); got != 0 {
		var texts []string
		for _, h := range d.Sections.UnknownHeadings {
			texts = append(texts, h.Text)
		}
		t.Errorf("expected 0 unknown headings (frontmatter must not produce setext H2s), got %d: %v", got, texts)
	}
}

// TestParse_MalformedFrontmatter_DoesNotCorruptBody is a regression test for
// the case where YAML frontmatter is opened but never properly closed, and
// the body contains a "\n---" sequence (e.g. as a thematic break or in a
// quoted code block). The frontmatter stripper must not match a body
// "\n---" as the closer and silently blank out real content.
func TestParse_MalformedFrontmatter_DoesNotCorruptBody(t *testing.T) {
	t.Parallel()

	// Opening "---" with no matching closing line. The body contains a
	// "---" thematic break in the middle of real content. A buggy stripper
	// would match "\n---" anywhere in the file and blank out the title and
	// the first H2.
	source := []byte("---\n" +
		"subcategory: \"Foo\"\n" +
		// no closing "---" line
		"\n" +
		"# Resource: aws_test\n\n" +
		"## Example Usage\n\n" +
		"Some lead-in text.\n\n" +
		"---\n" + // body thematic break
		"\n" +
		"## Argument Reference\n\n" +
		"* `id` - (Optional) The ID.\n")

	d, err := doc.Parse(source, "test")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if d.Sections.Title == nil {
		t.Error("Title section should be discovered (body must not be corrupted)")
	}
	if d.Sections.Example == nil {
		t.Error("Example Usage section should be discovered")
	}
	if d.Sections.Arguments == nil {
		t.Error("Argument Reference section should be discovered")
	}
}
