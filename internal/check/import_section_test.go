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

func TestImportSectionRule(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		source   string
		identity *schema.IdentitySchema
		wantErr  []string
		wantOK   bool
	}{
		"valid import section — no identity": {
			source: "# Resource: aws_thing\n\n## Import\n\nIn Terraform v1.5.0 and later, use an [`import` block](https://developer.hashicorp.com/terraform/language/import) to import things using their ID. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"thing-123\"\n}\n```\n\nUsing `terraform import`, import things using their ID. For example:\n\n```console\n% terraform import aws_thing.example thing-123\n```\n",
			wantOK: true,
		},
		"passive voice — can be imported": {
			source:  "# Resource: aws_thing\n\n## Import\n\nThings can be imported using their ID. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"thing-123\"\n}\n```\n",
			wantErr: []string{"should not include \"can be imported\""},
		},
		"e.g. usage": {
			source:  "# Resource: aws_thing\n\n## Import\n\nImport using the ID (e.g. thing-123). For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"thing-123\"\n}\n```\n",
			wantErr: []string{`should not include "e.g."`},
		},
		"no code blocks": {
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things using their ID. For example:\n",
			wantErr: []string{"should have at least one code block"},
		},
		"cannot import — no code blocks OK": {
			source: "# Resource: aws_thing\n\n## Import\n\nYou cannot import this resource.\n",
			wantOK: true,
		},
		"first block not terraform import": {
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```console\n% terraform import aws_thing.example thing-123\n```\n",
			wantErr: []string{"first import code block should be"},
		},
		"code block missing resource name": {
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```terraform\nimport {\n  to = aws_other.example\n  id = \"thing-123\"\n}\n```\n",
			wantErr: []string{"should contain resource name"},
		},
		"invalid code block language": {
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"thing-123\"\n}\n```\n\n```bash\nterraform import aws_thing.example thing-123\n```\n",
			wantErr: []string{"should be 'terraform' or 'console'"},
		},
		"console block without % prefix": {
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"thing-123\"\n}\n```\n\n```console\nterraform import aws_thing.example thing-123\n```\n",
			wantErr: []string{"should begin with '% '"},
		},
		"terraform block after console": {
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"thing-123\"\n}\n```\n\n```console\n% terraform import aws_thing.example thing-123\n```\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"other\"\n}\n```\n",
			wantErr: []string{"terraform import blocks should appear before console"},
		},
		"identity — valid section": {
			identity: &schema.IdentitySchema{
				Attributes: []schema.IdentityAttribute{
					{Name: "arn", Type: "string", Required: true},
				},
			},
			source: "# Resource: aws_thing\n\n## Import\n\nIn Terraform v1.12.0 and later, the [`import` block](https://developer.hashicorp.com/terraform/language/import) can be used with the `identity` attribute. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  identity = {\n    \"arn\" = \"arn:aws:thing:us-east-1:123456789012:thing/example\"\n  }\n}\n\nresource \"aws_thing\" \"example\" {\n  ### Configuration omitted for brevity ###\n}\n```\n\n### Identity Schema\n\n#### Required\n\n* `arn` (String) ARN of the thing.\n\nIn Terraform v1.5.0 and later, use an [`import` block](https://developer.hashicorp.com/terraform/language/import) to import things. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"arn:aws:thing:us-east-1:123456789012:thing/example\"\n}\n```\n\nUsing `terraform import`, import things. For example:\n\n```console\n% terraform import aws_thing.example arn:aws:thing:us-east-1:123456789012:thing/example\n```\n",
			wantOK: true,
		},
		"identity — missing identity in first block": {
			identity: &schema.IdentitySchema{
				Attributes: []schema.IdentityAttribute{
					{Name: "arn", Type: "string", Required: true},
				},
			},
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  id = \"thing-123\"\n}\n```\n\n### Identity Schema\n\n#### Required\n\n* `arn` (String) ARN.\n\n```console\n% terraform import aws_thing.example thing-123\n```\n",
			wantErr: []string{"first import block should use identity"},
		},
		"identity — missing Identity Schema subsection": {
			identity: &schema.IdentitySchema{
				Attributes: []schema.IdentityAttribute{
					{Name: "arn", Type: "string", Required: true},
				},
			},
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  identity = {\n    \"arn\" = \"x\"\n  }\n}\n```\n\n```console\n% terraform import aws_thing.example thing-123\n```\n",
			wantErr: []string{"should include ### Identity Schema subsection"},
		},
		"identity — schema attr not documented": {
			identity: &schema.IdentitySchema{
				Attributes: []schema.IdentityAttribute{
					{Name: "arn", Type: "string", Required: true},
					{Name: "region", Type: "string", Required: false},
				},
			},
			source:  "# Resource: aws_thing\n\n## Import\n\nImport things. For example:\n\n```terraform\nimport {\n  to = aws_thing.example\n  identity = {\n    \"arn\" = \"x\"\n  }\n}\n```\n\n### Identity Schema\n\n#### Required\n\n* `arn` (String) ARN.\n\n```console\n% terraform import aws_thing.example thing-123\n```\n",
			wantErr: []string{`identity attribute "region" is in schema but not documented`},
		},
		"no import section — no results": {
			source: "# Resource: aws_thing\n\n## Argument Reference\n",
			wantOK: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d, err := doc.Parse([]byte(tt.source), "aws_thing")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := &check.ImportSectionRule{RequireIdentitySection: true}
			results := rule.Check(check.CheckContext{
				Resource:       "aws_thing",
				Doc:            d,
				IdentitySchema: tt.identity,
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
