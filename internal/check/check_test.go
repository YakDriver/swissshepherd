// Copyright (c) YakDriver, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

func TestCompletenessRule_Complete(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.CompletenessRule{IgnoreDeprecated: true}
	results := rule.Check("test_instance", ps.Resources["test_instance"], d)

	// Filter to errors only (warnings about timeouts block are expected since we don't doc those attrs)
	var errors []check.Result
	for _, r := range results {
		if r.Severity == check.SeverityError {
			errors = append(errors, r)
		}
	}

	// The complete doc should have no errors for root and network blocks.
	// Timeouts block attrs (create, delete) are in schema but not in the doc's ### block format,
	// so they'll show up. That's expected — timeouts are documented differently.
	for _, r := range errors {
		if r.Block != "timeouts" {
			t.Errorf("unexpected error: %s (block: %s)", r.Message, r.Block)
		}
	}
}

func TestCompletenessRule_Incomplete(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance_incomplete.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.CompletenessRule{IgnoreDeprecated: true}
	results := rule.Check("test_instance", ps.Resources["test_instance"], d)

	// Should report missing: description (root), network block entirely, timeouts block
	messages := resultMessages(results)

	// description is missing from root
	if !slices.ContainsFunc(messages, func(s string) bool {
		return strings.Contains(s, `"description"`) && strings.Contains(s, "(root)")
	}) {
		t.Error("expected error about missing 'description' in root block")
	}

	// network block is not documented at all
	if !slices.ContainsFunc(messages, func(s string) bool {
		return strings.Contains(s, `"network"`) && strings.Contains(s, "not documented")
	}) {
		t.Error("expected error about undocumented 'network' block")
	}
}

func TestCompletenessRule_SkipsImplicit(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.CompletenessRule{IgnoreDeprecated: true}
	results := rule.Check("test_instance", ps.Resources["test_instance"], d)

	// Should NOT report 'id' as missing
	for _, r := range results {
		if strings.Contains(r.Message, `"id"`) {
			t.Errorf("should not report 'id' as missing: %s", r.Message)
		}
	}
}

func resultMessages(results []check.Result) []string {
	msgs := make([]string, len(results))
	for i, r := range results {
		msgs[i] = r.Message
	}
	return msgs
}
