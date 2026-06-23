// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// TestParse_TfplugindocsStyle confirms the parser keys nested-block doc
// blocks by their full dot-notation path when the heading carries the
// path explicitly. This is the tfplugindocs canonical form ("Nested
// Schema for `<path>`") and the equivalent AWS-extended forms.
func TestParse_TfplugindocsStyle(t *testing.T) {
	t.Parallel()

	// A doc that mirrors the tfplugindocs layout: every nested block is
	// at heading level 3 with the full dot-path in backticks.
	source := []byte(`# Resource: aws_test

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

### Nested Schema for ` + "`config`" + `

* ` + "`enabled`" + ` - (Required) Enable.

### Nested Schema for ` + "`config.encryption`" + `

* ` + "`kms_key_id`" + ` - (Required) KMS key.

### Nested Schema for ` + "`config.encryption.rotation`" + `

* ` + "`interval_days`" + ` - (Required) Days.

### Nested Schema for ` + "`config.logging`" + `

* ` + "`bucket`" + ` - (Required) Bucket.
`)

	d, err := doc.Parse(source, "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cases := []struct {
		key      string
		wantAttr string
	}{
		{"config", "enabled"},
		{"config.encryption", "kms_key_id"},
		{"config.encryption.rotation", "interval_days"},
		{"config.logging", "bucket"},
	}

	for _, c := range cases {
		block, ok := d.ArgumentBlocks[c.key]
		if !ok {
			t.Errorf("ArgumentBlocks[%q] missing", c.key)
			continue
		}
		var found bool
		for _, a := range block.Attributes {
			if a.Name == c.wantAttr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ArgumentBlocks[%q] has no attribute %q (got %v)", c.key, c.wantAttr, block.Attributes)
		}
	}
}

// TestParse_RepeatedLeafDistinctPaths confirms that same-name nested
// blocks under different parents stay separated when authored with
// dot-notation. The appmesh "match" case is the canonical scenario:
// `spec.http_route.match` and `spec.grpc_route.match` are distinct
// schema blocks and must remain distinct in the doc model.
func TestParse_RepeatedLeafDistinctPaths(t *testing.T) {
	t.Parallel()

	source := []byte(`# Resource: aws_appmesh_test

## Argument Reference

* ` + "`spec`" + ` - (Required) Spec.

### ` + "`spec.http_route`" + ` Block

* ` + "`name`" + ` - (Required) Name.

### ` + "`spec.http_route.match`" + ` Block

* ` + "`method`" + ` - (Optional) HTTP method.
* ` + "`scheme`" + ` - (Optional) HTTP scheme.

### ` + "`spec.grpc_route`" + ` Block

* ` + "`name`" + ` - (Required) Name.

### ` + "`spec.grpc_route.match`" + ` Block

* ` + "`service_name`" + ` - (Optional) gRPC service name.
* ` + "`port`" + ` - (Optional) Port.
`)

	d, err := doc.Parse(source, "aws_appmesh_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	httpMatch, ok := d.ArgumentBlocks["spec.http_route.match"]
	if !ok {
		t.Fatalf("ArgumentBlocks missing spec.http_route.match — got keys %v", argumentKeys(d))
	}
	grpcMatch, ok := d.ArgumentBlocks["spec.grpc_route.match"]
	if !ok {
		t.Fatalf("ArgumentBlocks missing spec.grpc_route.match — got keys %v", argumentKeys(d))
	}

	// The two match blocks must be distinct DocBlocks with their own
	// attribute lists. No merging.
	if httpMatch == grpcMatch {
		t.Fatal("http_route.match and grpc_route.match share the same DocBlock — they should be distinct")
	}

	httpAttrs := attrNames(httpMatch.Attributes)
	grpcAttrs := attrNames(grpcMatch.Attributes)

	wantHTTP := []string{"method", "scheme"}
	wantGRPC := []string{"service_name", "port"}

	if !sameStrings(httpAttrs, wantHTTP) {
		t.Errorf("spec.http_route.match attributes = %v, want %v", httpAttrs, wantHTTP)
	}
	if !sameStrings(grpcAttrs, wantGRPC) {
		t.Errorf("spec.grpc_route.match attributes = %v, want %v", grpcAttrs, wantGRPC)
	}

	// And there must NOT be a leaf-keyed "match" block — the parser
	// keyed by full path, so the leaf form should not exist.
	if _, ok := d.ArgumentBlocks["match"]; ok {
		t.Errorf("ArgumentBlocks unexpectedly contains a leaf-keyed %q block; path-keyed headings should not collapse to leaf", "match")
	}
}

// argumentKeys returns the sorted keys of d.ArgumentBlocks for debug output.
func argumentKeys(d *doc.Document) []string {
	keys := make([]string, 0, len(d.ArgumentBlocks))
	for k := range d.ArgumentBlocks {
		keys = append(keys, k)
	}
	return keys
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
