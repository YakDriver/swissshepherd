// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// TestLabels_BudgetFixtureMisplacement drives the misplacement rule end-to-end
// on the frozen pre-fix aws_budgets_budget doc — the #60 flagship case. It pins
// the two behaviors the fixture exercises cleanly:
//
//   - Purely configurable blocks whose headings resolve to a schema block
//     (### Cost Filter → cost_filter, ### Filter Expression → filter_expression)
//     collapse to ONE "move this subsection" (ERROR), with no per-attribute noise
//     and no strip-label on their configurable fields — the core #60 fix.
//   - A prose-headed subsection that fails schema resolution (### Budget
//     Notification vs schema `notification`) falls back to the legacy
//     strip-label — the accepted, measured residual (§2, §12), never a move.
//
// (The auto_adjust_data / historical_options subsections in this doc use a
// non-canonical "`name` (Required) -" label form that the parser treats as
// malformed, so they do not drive misplacement; the mixed-block per-attribute
// behavior is covered by TestLabels_MixedBlockPerAttributeMoveKeepsComputedStrip.)
func TestLabels_BudgetFixtureMisplacement(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/budgets_budget_misplaced.html.markdown")
	if err != nil {
		t.Fatal(err)
	}

	// Schema mirrors the budgets_budget blocks the fixture documents with
	// canonical labels. filter_expression's operands are modeled as configurable
	// scalars — the test only needs them to be pure-config so the subsection
	// collapses.
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {ChildBlocks: []string{"cost_filter", "filter_expression", "notification"}},
		"cost_filter": {Path: "cost_filter", Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "values", Required: true},
		}},
		"filter_expression": {Path: "filter_expression", Attributes: []schema.Attribute{
			{Name: "and", Optional: true},
			{Name: "or", Optional: true},
			{Name: "not", Optional: true},
			{Name: "dimensions", Optional: true},
			{Name: "tags", Optional: true},
			{Name: "cost_categories", Optional: true},
		}},
		// Configurable, but documented under the prose heading "Budget
		// Notification", which does not resolve to this schema name.
		"notification": {Path: "notification", Attributes: []schema.Attribute{
			{Name: "comparison_operator", Required: true},
			{Name: "notification_type", Required: true},
			{Name: "threshold", Required: true},
			{Name: "threshold_type", Required: true},
			{Name: "subscriber_email_addresses", Optional: true},
			{Name: "subscriber_sns_topic_arns", Optional: true},
		}},
	}}

	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_budgets_budget", Schema: rs, Doc: d})

	has := func(sub string) bool { return hasMsg(results, sub) }

	// 1. Pure-config blocks collapse to one subsection move (ERROR).
	for _, block := range []string{"cost_filter", "filter_expression"} {
		msg := `block "` + block + `" is documented under Attribute Reference but is a configurable argument block in the schema; move this subsection to Argument Reference`
		if sev, ok := findSeverity(results, msg); !ok || sev != check.SeverityError {
			t.Errorf("%s collapse: ok=%v sev=%v, want ERROR; results=%+v", block, ok, sev, results)
		}
	}

	// 2. A collapsed block emits no per-attribute noise and no strip-label on its
	//    configurable fields (the collapse covers them).
	for _, bad := range []string{
		`attribute "name" in block "cost_filter" should not have`,
		`attribute "values" in block "cost_filter" should not have`,
		`in block "cost_filter" is documented under Attribute Reference but is a configurable argument in the schema`,
		`attribute "and" in block "filter_expression" should not have`,
		`in block "filter_expression" is documented under Attribute Reference but is a configurable argument in the schema`,
	} {
		if has(bad) {
			t.Errorf("collapse must suppress per-attribute/strip noise (%q): %+v", bad, results)
		}
	}

	// 3. Prose-headed unresolved subsection falls back to strip-label (accepted
	//    residual, §2/§12) — never a move.
	if !has(`attribute "comparison_operator" in block "budget_notification" should not have (Required) label`) {
		t.Errorf("prose-headed budget_notification should retain legacy strip-label: %+v", results)
	}
	if has(`in block "budget_notification" is documented under Attribute Reference but is a configurable argument`) ||
		has(`block "budget_notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("an unresolved prose heading must never produce a move: %+v", results)
	}
}
