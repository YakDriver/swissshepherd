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

func TestArgumentsSectionRule_Ordered(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.ArgumentsSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("unexpected ordering error: %s", r.Message)
		}
	}
}

func TestArgumentsSectionRule_Unordered(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_unordered.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.ArgumentsSectionRule{}
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

func TestAttributesSectionRule_Unordered(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_unordered.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.AttributesSectionRule{}
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

func TestDescriptionStyleRule_Good(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.DescriptionStyleRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: nil, Doc: d})

	if len(results) > 0 {
		t.Errorf("expected no style errors, got %d: %v", len(results), results[0].Message)
	}
}

func TestDescriptionStyleRule_Bad(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance_bad_style.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.DescriptionStyleRule{}
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

func TestAttributesSectionRule_Correct(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.AttributesSectionRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	// arn is computed-only and is in the Attribute Reference section — should pass
	for _, r := range results {
		if r.Severity == check.SeverityError && strings.Contains(r.Message, "arn") {
			t.Errorf("unexpected error for 'arn': %s", r.Message)
		}
	}
}

func TestAttributesSectionRule_Wrong(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance_computed_wrong.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	// AttributesSectionRule: arn is computed-only but NOT in Attribute Reference
	attrRule := &check.AttributesSectionRule{}
	attrResults := attrRule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	var foundMissing bool
	for _, r := range attrResults {
		if strings.Contains(r.Message, "should be documented in Attribute Reference") {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Error("expected error about 'arn' missing from Attribute Reference section")
	}

	// ArgumentsSectionRule: arn is computed-only but IS in Argument Reference
	argRule := &check.ArgumentsSectionRule{}
	argResults := argRule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	var foundWrongSection bool
	for _, r := range argResults {
		if strings.Contains(r.Message, "should not appear in Argument Reference") {
			foundWrongSection = true
		}
	}
	if !foundWrongSection {
		t.Error("expected warning about 'arn' appearing in Argument Reference section")
	}
}
