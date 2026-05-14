// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

func TestSignatureSectionRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		markdown  string
		funcSch   *schema.FunctionSchema
		wantErrs  int
		wantWarns int
	}{
		{
			name:     "valid — code block with function name and params",
			markdown: "# Function: arn_parse\n\n## Signature\n\n```text\narn_parse(arn string) object\n```\n",
			funcSch:  &schema.FunctionSchema{Name: "arn_parse", ParameterNames: []string{"arn"}},
		},
		{
			name:     "valid — multiple params",
			markdown: "# Function: arn_build\n\n## Signature\n\n```text\narn_build(partition string, service string) string\n```\n",
			funcSch:  &schema.FunctionSchema{Name: "arn_build", ParameterNames: []string{"partition", "service"}},
		},
		{
			name:     "error — no code block",
			markdown: "# Function: arn_parse\n\n## Signature\n\nSome text but no code block.\n",
			wantErrs: 1,
		},
		{
			name:     "error — code block missing function name",
			markdown: "# Function: arn_parse\n\n## Signature\n\n```text\nother_func(arn string) object\n```\n",
			wantErrs: 1,
		},
		{
			name:      "warning — code block missing parameter",
			markdown:  "# Function: arn_parse\n\n## Signature\n\n```text\narn_parse(wrong string) object\n```\n",
			funcSch:   &schema.FunctionSchema{Name: "arn_parse", ParameterNames: []string{"arn"}},
			wantWarns: 1,
		},
		{
			name:      "warning — missing variadic parameter",
			markdown:  "# Function: my_func\n\n## Signature\n\n```text\nmy_func(a string) string\n```\n",
			funcSch:   &schema.FunctionSchema{Name: "my_func", ParameterNames: []string{"a"}, VariadicParameter: "extra"},
			wantWarns: 1,
		},
		{
			name:     "no section — no errors",
			markdown: "# Function: arn_parse\n\n## Arguments\n\nStuff.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.Parse([]byte(tt.markdown), "arn_parse")
			if err != nil {
				t.Fatal(err)
			}
			rule := &SignatureSectionRule{}
			ctx := CheckContext{Resource: tt.name, Doc: d, FunctionSchema: tt.funcSch}
			// Use the function name from the markdown title
			if tt.funcSch != nil {
				ctx.Resource = tt.funcSch.Name
			} else {
				ctx.Resource = "arn_parse"
			}
			results := rule.Check(ctx)

			var errs, warns int
			for _, r := range results {
				switch r.Severity {
				case SeverityError:
					errs++
				case SeverityWarning:
					warns++
				}
			}
			if errs != tt.wantErrs {
				t.Errorf("errors = %d, want %d; results: %v", errs, tt.wantErrs, results)
			}
			if warns != tt.wantWarns {
				t.Errorf("warnings = %d, want %d; results: %v", warns, tt.wantWarns, results)
			}
		})
	}
}
