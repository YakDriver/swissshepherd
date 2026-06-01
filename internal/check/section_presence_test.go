// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// resourceSpec returns a fresh canonical resource section spec for use in
// tests. Order: title, example, arguments, attributes, timeouts (optional),
// import (optional), signature (forbidden).
func resourceSpec() []config.SectionSpec {
	return []config.SectionSpec{
		{Name: "title", Required: true},
		{Name: "example", Required: true},
		{Name: "arguments", Required: true},
		{Name: "attributes", Required: true},
		{Name: "timeouts"},
		{Name: "import"},
		{Name: "signature", Forbidden: true},
	}
}

func TestSectionPresenceRule_Presence(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		typeCfg config.Type
		source  string
		wantErr []string
		wantOK  bool
	}{
		"resource with all sections present passes": {
			typeCfg: config.Type{Name: "resource", Sections: resourceSpec()},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n```terraform\n```\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n\n" +
				"## Timeouts\n\n" +
				"## Import\n",
			wantOK: true,
		},
		"resource missing required Attribute Reference fails": {
			typeCfg: config.Type{Name: "resource", Sections: resourceSpec()},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n",
			wantErr: []string{"missing required section: ## Attribute Reference"},
		},
		"resource missing Example Usage fails": {
			typeCfg: config.Type{Name: "resource", Sections: resourceSpec()},
			source: "# Resource: test\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n",
			wantErr: []string{"missing required section: ## Example Usage"},
		},
		"resource with forbidden Signature section fails": {
			typeCfg: config.Type{Name: "resource", Sections: resourceSpec()},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n\n" +
				"## Signature\n",
			wantErr: []string{`section ## Signature is not allowed for type "resource"`},
		},
		"data source with Import section when forbidden fails": {
			typeCfg: config.Type{
				Name: "data_source",
				Sections: []config.SectionSpec{
					{Name: "title", Required: true},
					{Name: "example", Required: true},
					{Name: "arguments", Required: true},
					{Name: "attributes", Required: true},
					{Name: "import", Forbidden: true},
				},
			},
			source: "# Data Source: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n\n" +
				"## Import\n",
			wantErr: []string{`section ## Import is not allowed for type "data_source"`},
		},
		"function missing required Signature fails": {
			typeCfg: config.Type{
				Name: "function",
				Sections: []config.SectionSpec{
					{Name: "title", Required: true},
					{Name: "example", Required: true},
					{Name: "signature", Required: true},
					{Name: "arguments", Required: true},
				},
			},
			source: "# Function: test\n\n" +
				"## Example Usage\n\n" +
				"## Arguments\n",
			wantErr: []string{"missing required section: ## Signature"},
		},
		"type with no sections is skipped": {
			typeCfg: config.Type{Name: "guide"},
			source:  "# Guide\n\n## Random heading\n",
			wantOK:  true,
		},
		"nil type returns no results": {
			source: "# Resource: test\n\n## Argument Reference\n",
			wantOK: true,
		},
		"multiple violations reported": {
			typeCfg: config.Type{Name: "resource", Sections: resourceSpec()},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Signature\n",
			wantErr: []string{
				"missing required section: ## Attribute Reference",
				`section ## Signature is not allowed for type "resource"`,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d, err := doc.Parse([]byte(tt.source), "test")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			var tp *config.Type
			if tt.typeCfg.Name != "" {
				tp = &tt.typeCfg
			}

			rule := &check.SectionPresenceRule{}
			results := rule.Check(check.CheckContext{
				Resource: "test",
				Type:     tp,
				Doc:      d,
			})

			if tt.wantOK {
				if len(results) != 0 {
					var msgs []string
					for _, r := range results {
						msgs = append(msgs, r.Message)
					}
					t.Errorf("expected 0 results, got %d:\n  %s", len(results), strings.Join(msgs, "\n  "))
				}
				return
			}

			for _, want := range tt.wantErr {
				found := false
				for _, r := range results {
					if strings.Contains(r.Message, want) {
						found = true
						break
					}
				}
				if !found {
					var msgs []string
					for _, r := range results {
						msgs = append(msgs, r.Message)
					}
					t.Errorf("expected message containing %q, got:\n  %s", want, strings.Join(msgs, "\n  "))
				}
			}
		})
	}
}

func TestSectionPresenceRule_Order(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		source     string
		wantOrders []string
	}{
		"in-order resource passes": {
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n\n" +
				"## Timeouts\n\n" +
				"## Import\n",
		},
		"argument before example flagged": {
			source: "# Resource: test\n\n" +
				"## Argument Reference\n\n" +
				"## Example Usage\n\n" +
				"## Attribute Reference\n",
			wantOrders: []string{
				"section ## Argument Reference appears before ## Example Usage; expected the reverse order",
			},
		},
		"timeouts before attribute flagged": {
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Timeouts\n\n" +
				"## Attribute Reference\n",
			wantOrders: []string{
				"section ## Timeouts appears before ## Attribute Reference; expected the reverse order",
			},
		},
		"import before timeouts flagged": {
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n\n" +
				"## Import\n\n" +
				"## Timeouts\n",
			wantOrders: []string{
				"section ## Import appears before ## Timeouts; expected the reverse order",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d, err := doc.Parse([]byte(tt.source), "test")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			tp := &config.Type{Name: "resource", Sections: resourceSpec()}
			rule := &check.SectionPresenceRule{}
			results := rule.Check(check.CheckContext{
				Resource: "test",
				Type:     tp,
				Doc:      d,
			})

			// Filter to order messages only.
			var orderMsgs []string
			for _, r := range results {
				if strings.Contains(r.Message, "appears before") {
					orderMsgs = append(orderMsgs, r.Message)
				}
			}

			if len(tt.wantOrders) == 0 {
				if len(orderMsgs) != 0 {
					t.Errorf("expected no order errors, got:\n  %s", strings.Join(orderMsgs, "\n  "))
				}
				return
			}

			for _, want := range tt.wantOrders {
				found := false
				for _, m := range orderMsgs {
					if strings.Contains(m, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected order message containing %q, got:\n  %s",
						want, strings.Join(orderMsgs, "\n  "))
				}
			}
		})
	}
}

func TestSectionPresenceRule_OrderDisabled(t *testing.T) {
	t.Parallel()

	source := "# Resource: test\n\n" +
		"## Argument Reference\n\n" +
		"## Example Usage\n\n" +
		"## Attribute Reference\n"

	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	tp := &config.Type{Name: "resource", Sections: resourceSpec()}
	disabled := false
	rule := &check.SectionPresenceRule{EnforceOrder: &disabled}
	results := rule.Check(check.CheckContext{
		Resource: "test",
		Type:     tp,
		Doc:      d,
	})

	for _, r := range results {
		if strings.Contains(r.Message, "appears before") {
			t.Errorf("expected no order errors with EnforceOrder=false, got: %s", r.Message)
		}
	}
}

func TestSectionPresenceRule_Unknown(t *testing.T) {
	t.Parallel()

	source := "# Resource: test\n\n" +
		"## Example Usage\n\n" +
		"## Argument Reference\n\n" +
		"## Attribute Reference\n\n" +
		"## My wild heading\n\n" +
		"some content\n\n" +
		"## Another mystery section\n"

	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	tp := &config.Type{Name: "resource", Sections: resourceSpec()}
	rule := &check.SectionPresenceRule{}
	results := rule.Check(check.CheckContext{
		Resource: "test",
		Type:     tp,
		Doc:      d,
	})

	wantUnknown := []string{
		"unknown level-2 section: ## My wild heading",
		"unknown level-2 section: ## Another mystery section",
	}
	for _, want := range wantUnknown {
		found := false
		for _, r := range results {
			if strings.Contains(r.Message, want) {
				found = true
				break
			}
		}
		if !found {
			var msgs []string
			for _, r := range results {
				msgs = append(msgs, r.Message)
			}
			t.Errorf("expected message containing %q, got:\n  %s",
				want, strings.Join(msgs, "\n  "))
		}
	}
}

func TestSectionPresenceRule_AllowUnknownSections(t *testing.T) {
	t.Parallel()

	source := "# Resource: test\n\n" +
		"## Example Usage\n\n" +
		"## Argument Reference\n\n" +
		"## Attribute Reference\n\n" +
		"## My wild heading\n"

	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	tp := &config.Type{Name: "resource", Sections: resourceSpec()}
	allow := true
	rule := &check.SectionPresenceRule{AllowUnknownSections: &allow}
	results := rule.Check(check.CheckContext{
		Resource: "test",
		Type:     tp,
		Doc:      d,
	})

	for _, r := range results {
		if strings.Contains(r.Message, "unknown level-2 section") {
			t.Errorf("expected no unknown-section errors with AllowUnknownSections=true, got: %s", r.Message)
		}
	}
}

func TestSectionPresenceRule_SchemaTimeouts(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		schema  *schema.ResourceSchema
		source  string
		wantErr string
	}{
		"timeouts section present and schema has timeouts — OK": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{
					"":         {},
					"timeouts": {Attributes: []schema.Attribute{{Name: "create", Optional: true}}},
				},
			},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n\n" +
				"## Timeouts\n\n" +
				"* `create` - (Default `60m`)\n",
		},
		"timeouts section present but schema has no timeouts — error": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{"": {}},
			},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n\n" +
				"## Timeouts\n\n" +
				"* `create` - (Default `60m`)\n",
			wantErr: "schema does not configure timeouts",
		},
		"no timeouts section but schema has timeouts — error": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{
					"":         {},
					"timeouts": {Attributes: []schema.Attribute{{Name: "create", Optional: true}}},
				},
			},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n",
			wantErr: "schema configures timeouts ('create') but ## Timeouts section is missing",
		},
		"no timeouts section and schema has no timeouts — OK": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{"": {}},
			},
			source: "# Resource: test\n\n" +
				"## Example Usage\n\n" +
				"## Argument Reference\n\n" +
				"## Attribute Reference\n",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d, err := doc.Parse([]byte(tt.source), "test")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			tp := &config.Type{Name: "resource", Sections: resourceSpec()}
			rule := &check.SectionPresenceRule{}
			results := rule.Check(check.CheckContext{
				Resource: "test",
				Type:     tp,
				Schema:   tt.schema,
				Doc:      d,
			})

			if tt.wantErr == "" {
				for _, r := range results {
					if strings.Contains(r.Message, "imeout") {
						t.Errorf("unexpected timeouts error: %s", r.Message)
					}
				}
				return
			}
			found := false
			for _, r := range results {
				if strings.Contains(r.Message, tt.wantErr) {
					found = true
				}
			}
			if !found {
				var msgs []string
				for _, r := range results {
					msgs = append(msgs, r.Message)
				}
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, msgs)
			}
		})
	}
}
