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

func TestSchemaDocsRule_Ordered(t *testing.T) {
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

func TestSchemaDocsRule_Unordered(t *testing.T) {
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

func TestSchemaDocsRule_UnorderedAttributes(t *testing.T) {
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

func TestSchemaDocsRule_Good(t *testing.T) {
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

func TestSchemaDocsRule_Bad(t *testing.T) {
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
