// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
)

func TestSectionPresenceRule(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		typeCfg config.Type
		source  string
		wantErr []string // substrings expected in error messages
		wantOK  bool     // true = expect zero results
	}{
		"resource with all optional sections present passes": {
			typeCfg: config.Type{
				Name:              "resource",
				RequireAttributes: config.SectionRequired,
				RequireImport:     config.SectionOptional,
				RequireTimeouts:   config.SectionOptional,
				RequireSignature:  config.SectionForbidden,
			},
			source: "# Resource: test\n\n## Argument Reference\n\n## Attribute Reference\n\n## Import\n\n## Timeouts\n",
			wantOK: true,
		},
		"resource missing required Attribute Reference fails": {
			typeCfg: config.Type{
				Name:              "resource",
				RequireAttributes: config.SectionRequired,
			},
			source:  "# Resource: test\n\n## Argument Reference\n",
			wantErr: []string{"missing required section: ## Attribute Reference"},
		},
		"data source with Import section when forbidden fails": {
			typeCfg: config.Type{
				Name:          "data_source",
				RequireImport: config.SectionForbidden,
			},
			source:  "# Data Source: test\n\n## Argument Reference\n\n## Attribute Reference\n\n## Import\n",
			wantErr: []string{`section ## Import is not allowed for type "data_source"`},
		},
		"function missing required Signature fails": {
			typeCfg: config.Type{
				Name:             "function",
				RequireSignature: config.SectionRequired,
				RequireImport:    config.SectionForbidden,
			},
			source:  "# Function: test\n\n## Arguments\n",
			wantErr: []string{"missing required section: ## Signature"},
		},
		"function with Signature passes": {
			typeCfg: config.Type{
				Name:             "function",
				RequireSignature: config.SectionRequired,
				RequireImport:    config.SectionForbidden,
			},
			source: "# Function: test\n\n## Arguments\n\n## Signature\n\n```\nfoo(bar string) string\n```\n",
			wantOK: true,
		},
		"function with Import section when forbidden fails": {
			typeCfg: config.Type{
				Name:             "function",
				RequireSignature: config.SectionRequired,
				RequireImport:    config.SectionForbidden,
			},
			source:  "# Function: test\n\n## Arguments\n\n## Signature\n\n## Import\n",
			wantErr: []string{`section ## Import is not allowed for type "function"`},
		},
		"empty requirement means optional — section absent is fine": {
			typeCfg: config.Type{
				Name:            "resource",
				RequireTimeouts: "", // empty = optional
			},
			source: "# Resource: test\n\n## Argument Reference\n",
			wantOK: true,
		},
		"empty requirement means optional — section present is fine": {
			typeCfg: config.Type{
				Name:            "resource",
				RequireTimeouts: config.SectionOptional,
			},
			source: "# Resource: test\n\n## Argument Reference\n\n## Timeouts\n",
			wantOK: true,
		},
		"multiple violations reported": {
			typeCfg: config.Type{
				Name:              "resource",
				RequireAttributes: config.SectionRequired,
				RequireImport:     config.SectionRequired,
				RequireTimeouts:   config.SectionForbidden,
			},
			source: "# Resource: test\n\n## Argument Reference\n\n## Timeouts\n",
			wantErr: []string{
				"missing required section: ## Attribute Reference",
				"missing required section: ## Import",
				`section ## Timeouts is not allowed for type "resource"`,
			},
		},
		"nil type returns no results": {
			source: "# Resource: test\n\n## Argument Reference\n",
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
