// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

func TestExampleSectionRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		markdown  string
		langs     []string
		wantErrs  int
		wantWarns int
	}{
		{
			name:     "valid — terraform code block with resource name",
			markdown: "# Resource: aws_thing\n\n## Example Usage\n\n```terraform\nresource \"aws_thing\" \"example\" {}\n```\n",
		},
		{
			name:     "valid — hcl code block",
			markdown: "# Resource: aws_thing\n\n## Example Usage\n\n```hcl\nresource \"aws_thing\" \"example\" {}\n```\n",
		},
		{
			name:      "error — only non-allowed language",
			markdown:  "# Resource: aws_thing\n\n## Example Usage\n\n```python\nimport aws_thing\n```\n",
			wantErrs:  1,
			wantWarns: 1,
		},
		{
			name:      "warning — missing resource name",
			markdown:  "# Resource: aws_thing\n\n## Example Usage\n\n```terraform\nresource \"aws_other\" \"example\" {}\n```\n",
			wantWarns: 1,
		},
		{
			name:      "error — wrong language with custom allowed",
			markdown:  "# Resource: aws_thing\n\n## Example Usage\n\n```terraform\nresource \"aws_thing\" \"example\" {}\n```\n",
			langs:     []string{"hcl"},
			wantErrs:  1,
			wantWarns: 1,
		},
		{
			name:     "no section — no errors",
			markdown: "# Resource: aws_thing\n\n## Argument Reference\n\nStuff.\n",
		},
		{
			name:     "valid — no language on code block (allowed)",
			markdown: "# Resource: aws_thing\n\n## Example Usage\n\n```\nresource \"aws_thing\" \"example\" {}\n```\n",
		},
		{
			name:     "error — wrong heading text",
			markdown: "# Resource: aws_thing\n\n## Examples\n\n```terraform\nresource \"aws_thing\" \"example\" {}\n```\n",
			wantErrs: 1,
		},
		{
			name:     "valid — supplementary json block alongside terraform",
			markdown: "# Resource: aws_thing\n\n## Example Usage\n\n```terraform\nresource \"aws_thing\" \"example\" {}\n```\n\n```json\n{\"key\": \"value\"}\n```\n",
		},
		{
			name:     "valid — supplementary console block alongside terraform",
			markdown: "# Resource: aws_thing\n\n## Example Usage\n\n```terraform\nresource \"aws_thing\" \"example\" {}\n```\n\n```console\n% aws s3 ls\n```\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.Parse([]byte(tt.markdown), "aws_thing")
			if err != nil {
				t.Fatal(err)
			}
			rule := &ExampleSectionRule{AllowedLanguages: tt.langs}
			ctx := CheckContext{Resource: "aws_thing", Doc: d}
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
