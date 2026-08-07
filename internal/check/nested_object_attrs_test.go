// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// objectAttrSchema builds a data source whose computed `items` attribute is an
// object-typed list: items => {arn, foo}. Children live on Attribute.Children,
// exactly as the loader produces them for list(object({...})).
func objectAttrSchema() *schema.ResourceSchema {
	return &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "name", Required: true},
				{Name: "items", Computed: true, Children: []schema.Attribute{
					{Name: "arn", Computed: true},
					{Name: "foo", Computed: true},
				}},
			}},
		},
	}
}

func parseCaptured(t *testing.T, md string) *doc.Document {
	t.Helper()
	d, err := doc.ParseWithOptions([]byte(md), "aws_test",
		doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return d
}

func hasMessage(results []check.Result, substr string) bool {
	for _, r := range results {
		if strings.Contains(r.Message, substr) {
			return true
		}
	}
	return false
}

// TestNestedObject_On_MissingField_Errors: with the schema expanded and the
// doc parsed with nested capture, an undocumented object field is flagged.
func TestNestedObject_On_MissingField_Errors(t *testing.T) {
	t.Parallel()

	rs := objectAttrSchema()
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		DataSources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	// `items` documented inline with arn only — foo is missing.
	md := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"* `items` - List of objects. Each object has the following attributes:\n" +
		"    * `arn` - ARN value.\n"

	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{
		Resource: "aws_test", Schema: rs, Doc: parseCaptured(t, md),
	})

	want := `Read-Only attribute "foo" in block "items" should be documented`
	if !hasMessage(results, want) {
		t.Errorf("expected %q, got:\n  %s", want, joinMessages(results))
	}
}

// TestNestedObject_On_WeakDescription_Flagged: description style now reaches
// nested object fields — an "arn" documented as "The ARN." is flagged.
func TestNestedObject_On_WeakDescription_Flagged(t *testing.T) {
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

// TestNestedObject_Off_NoFindings: without expansion and without nested
// capture (the default), object fields are neither required nor style-checked.
func TestNestedObject_Off_NoFindings(t *testing.T) {
	t.Parallel()

	rs := objectAttrSchema() // NOT expanded

	// Default parse: no nested capture. arn has a weak "The" start and foo is
	// absent — neither should produce a finding.
	md := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"* `items` - List of objects. Each object has the following attributes:\n" +
		"    * `arn` - The ARN value.\n"

	d, err := doc.Parse([]byte(md), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{
		Resource: "aws_test", Schema: rs, Doc: d,
	})

	if hasMessage(results, `block "items"`) {
		t.Errorf("did not expect nested-block findings with toggle off:\n  %s", joinMessages(results))
	}
	if hasMessage(results, `"foo"`) {
		t.Errorf("did not expect a foo finding with toggle off:\n  %s", joinMessages(results))
	}
	if hasMessage(results, `attribute "arn" description should not start`) {
		t.Errorf("did not expect nested arn description finding with toggle off:\n  %s", joinMessages(results))
	}
}
