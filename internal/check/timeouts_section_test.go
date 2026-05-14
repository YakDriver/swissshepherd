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

func TestTimeoutsSectionRule(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		schema  *schema.ResourceSchema
		source  string
		wantErr []string
		wantOK  bool
	}{
		"all actions match — OK": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{
					"":         {},
					"timeouts": {Attributes: []schema.Attribute{{Name: "create", Optional: true}, {Name: "delete", Optional: true}}},
				},
			},
			source: "# Resource: test\n\n## Timeouts\n\n* `create` - (Default `60m`)\n* `delete` - (Default `90m`)\n",
			wantOK: true,
		},
		"schema action not documented": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{
					"":         {},
					"timeouts": {Attributes: []schema.Attribute{{Name: "create", Optional: true}, {Name: "update", Optional: true}, {Name: "delete", Optional: true}}},
				},
			},
			source:  "# Resource: test\n\n## Timeouts\n\n* `create` - (Default `60m`)\n* `delete` - (Default `90m`)\n",
			wantErr: []string{`timeout action "update" is configured in schema but not documented`},
		},
		"documented action not in schema": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{
					"":         {},
					"timeouts": {Attributes: []schema.Attribute{{Name: "create", Optional: true}}},
				},
			},
			source:  "# Resource: test\n\n## Timeouts\n\n* `create` - (Default `60m`)\n* `delete` - (Default `90m`)\n",
			wantErr: []string{`documented timeout action "delete" does not exist in schema`},
		},
		"both directions — missing and extra": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{
					"":         {},
					"timeouts": {Attributes: []schema.Attribute{{Name: "create", Optional: true}, {Name: "read", Optional: true}}},
				},
			},
			source: "# Resource: test\n\n## Timeouts\n\n* `create` - (Default `60m`)\n* `delete` - (Default `90m`)\n",
			wantErr: []string{
				`timeout action "read" is configured in schema but not documented`,
				`documented timeout action "delete" does not exist in schema`,
			},
		},
		"nil schema — no results": {
			source: "# Resource: test\n\n## Timeouts\n\n* `create` - (Default `60m`)\n",
			wantOK: true,
		},
		"no timeouts section — no results": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{
					"":         {},
					"timeouts": {Attributes: []schema.Attribute{{Name: "create", Optional: true}}},
				},
			},
			source: "# Resource: test\n\n## Argument Reference\n",
			wantOK: true,
		},
		"no timeouts block in schema — no results": {
			schema: &schema.ResourceSchema{
				Blocks: map[string]*schema.Block{"": {}},
			},
			source: "# Resource: test\n\n## Timeouts\n\n* `create` - (Default `60m`)\n",
			wantOK: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d, err := doc.Parse([]byte(tt.source), "test")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := &check.TimeoutsSectionRule{}
			results := rule.Check(check.CheckContext{
				Resource: "test",
				Schema:   tt.schema,
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
