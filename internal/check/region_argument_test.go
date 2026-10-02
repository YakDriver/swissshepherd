// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

func TestRegionArgumentRule_Present(t *testing.T) {
	t.Parallel()

	rule := &check.RegionArgumentRule{}
	ctx := regionCtx(true, true)
	results := rule.Check(ctx)
	if len(results) != 0 {
		t.Errorf("expected no findings when region is documented, got %d", len(results))
	}
}

func TestRegionArgumentRule_Missing(t *testing.T) {
	t.Parallel()

	rule := &check.RegionArgumentRule{}
	ctx := regionCtx(true, false)
	results := rule.Check(ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 finding when region is missing from docs, got %d", len(results))
	}
}

func TestRegionArgumentRule_NotRegionAware(t *testing.T) {
	t.Parallel()

	rule := &check.RegionArgumentRule{}
	ctx := regionCtx(true, false)
	ctx.Type.RegionAware = false
	results := rule.Check(ctx)
	if len(results) != 0 {
		t.Errorf("expected no findings for non-region-aware type, got %d", len(results))
	}
}

func TestRegionArgumentRule_Ignored(t *testing.T) {
	t.Parallel()

	rule := &check.RegionArgumentRule{IgnoreResources: []string{"aws_thing"}}
	ctx := regionCtx(true, false)
	results := rule.Check(ctx)
	if len(results) != 0 {
		t.Errorf("expected no findings for ignored resource, got %d", len(results))
	}
}

func TestRegionArgumentRule_NoRegionInSchema(t *testing.T) {
	t.Parallel()

	rule := &check.RegionArgumentRule{}
	ctx := regionCtx(false, false)
	results := rule.Check(ctx)
	if len(results) != 0 {
		t.Errorf("expected no findings when schema has no region, got %d", len(results))
	}
}

func regionCtx(schemaHasRegion, docHasRegion bool) check.CheckContext {
	var attrs []schema.Attribute
	attrs = append(attrs, schema.Attribute{Name: "name", Optional: true})
	if schemaHasRegion {
		attrs = append(attrs, schema.Attribute{Name: "region", Optional: true})
	}

	rs := &schema.ResourceSchema{
		Blocks: map[string]*schema.Block{
			"": {Attributes: attrs},
		},
	}

	var docAttrs []doc.DocAttribute
	docAttrs = append(docAttrs, doc.DocAttribute{Name: "name", Optional: true})
	if docHasRegion {
		docAttrs = append(docAttrs, doc.DocAttribute{Name: "region", Optional: true})
	}

	d := &doc.Document{
		ArgumentBlocks: map[string]*doc.DocBlock{
			"": {Attributes: docAttrs},
		},
	}

	return check.CheckContext{
		Resource: "aws_thing",
		Type:     &config.Type{RegionAware: true},
		Schema:   rs,
		Doc:      d,
	}
}
