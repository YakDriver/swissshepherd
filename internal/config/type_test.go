// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/config"
)

// TestLoad_DefaultsEmbedded confirms the embedded defaults.hcl parses on the
// cold path and produces the eight standard categories every Terraform
// provider can expect.
func TestLoad_DefaultsEmbedded(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.hcl"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{
		"resource",
		"data_source",
		"ephemeral",
		"function",
		"list_resource",
		"action",
		"guide",
		"index",
	}
	if got := cfg.TypeNames(); !slices.Equal(got, want) {
		t.Errorf("TypeNames() = %v, want %v", got, want)
	}
}

// TestLoad_DefaultResourceShape pins the most-used default (the resource
// type) so later refactors don't silently drift one of the conventions AWS
// CI depends on.
func TestLoad_DefaultResourceShape(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.hcl"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	r := cfg.GetType("resource")
	if r == nil {
		t.Fatal("default type resource missing")
	}
	if r.SchemaKind != "resource" {
		t.Errorf("SchemaKind = %q, want %q", r.SchemaKind, "resource")
	}
	if r.TitlePrefix != "Resource" {
		t.Errorf("TitlePrefix = %q, want %q", r.TitlePrefix, "Resource")
	}
	wantPaths := []string{
		"docs/resources/{name}.md",
		"website/docs/r/{name}.html.markdown",
	}
	if !slices.Equal(r.WebsitePaths, wantPaths) {
		t.Errorf("WebsitePaths = %v, want %v", r.WebsitePaths, wantPaths)
	}
	if r.RequireAttributes != config.SectionRequired {
		t.Errorf("RequireAttributes = %q, want %q", r.RequireAttributes, config.SectionRequired)
	}
	if !r.RegionAware {
		t.Error("RegionAware should be true for resource type")
	}
}

// TestLoad_UserTypeAdds confirms a user-defined type with a new name is
// appended to the defaults.
func TestLoad_UserTypeAdds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "widget" {
  schema_kind   = "none"
  website_paths = ["docs/widgets/{name}.md"]
  title_prefix  = "Widget"
}
`)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	names := cfg.TypeNames()
	if !slices.Contains(names, "widget") {
		t.Errorf("widget type should be present; got %v", names)
	}
	// Defaults are still there.
	for _, def := range []string{"resource", "data_source", "guide"} {
		if !slices.Contains(names, def) {
			t.Errorf("default type %q missing after user added a new type: %v", def, names)
		}
	}

	widget := cfg.GetType("widget")
	if widget == nil {
		t.Fatal("widget type not resolvable via GetType")
	}
	if widget.TitlePrefix != "Widget" {
		t.Errorf("TitlePrefix = %q, want %q", widget.TitlePrefix, "Widget")
	}
}

// TestLoad_UserTypeOverridesDefault confirms a user block with the same name
// as a default replaces the default wholesale — no deep-merge, no
// accidentally-inherited fields.
func TestLoad_UserTypeOverridesDefault(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "resource" {
  schema_kind   = "resource"
  website_paths = ["custom/{name}.md"]
  title_prefix  = "Custom Resource"
  # Note: deliberately omitting fields the default has (arguments_bylines,
  # region_aware, frontmatter_require, etc.) to confirm an override replaces
  # rather than deep-merges.
}
`)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	r := cfg.GetType("resource")
	if r == nil {
		t.Fatal("resource type missing")
	}
	if got, want := r.TitlePrefix, "Custom Resource"; got != want {
		t.Errorf("TitlePrefix = %q, want %q", got, want)
	}
	if len(r.WebsitePaths) != 1 || r.WebsitePaths[0] != "custom/{name}.md" {
		t.Errorf("WebsitePaths = %v, want [custom/{name}.md]", r.WebsitePaths)
	}
	if len(r.ArgumentsBylines) != 0 {
		t.Errorf("override should clear ArgumentsBylines, got %v", r.ArgumentsBylines)
	}
	if r.RegionAware {
		t.Error("override should clear RegionAware, got true")
	}
}

// TestLoad_UserTypeOverridePositionPreserved checks that overriding a
// default keeps its position in the types slice, while new types land at
// the end. Keeping order predictable simplifies diagnostics.
func TestLoad_UserTypeOverridePositionPreserved(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "widget" {
  schema_kind   = "none"
  website_paths = ["docs/widgets/{name}.md"]
}

type "resource" {
  schema_kind   = "resource"
  website_paths = ["docs/resources/{name}.md"]
}
`)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	names := cfg.TypeNames()
	if names[0] != "resource" {
		t.Errorf("resource should remain first (override keeps default position), got %q at 0", names[0])
	}
	if names[len(names)-1] != "widget" {
		t.Errorf("widget (new user type) should land last, got %q; full order: %v", names[len(names)-1], names)
	}
}

func TestLoad_InvalidType_EmptyWebsitePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "broken" {
  schema_kind   = "none"
  website_paths = []
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for type with no website_paths")
	}
	if !strings.Contains(err.Error(), "website_paths") {
		t.Errorf("error should mention website_paths; got: %v", err)
	}
}

func TestLoad_InvalidType_InvalidSectionRequirement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "broken" {
  schema_kind       = "none"
  website_paths     = ["docs/{name}.md"]
  require_attributes = "mandatory"
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid section requirement")
	}
	if !strings.Contains(err.Error(), "require_attributes") {
		t.Errorf("error should name the offending field; got: %v", err)
	}
}

func TestLoad_InvalidType_FrontmatterOverlap(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "broken" {
  schema_kind         = "none"
  website_paths       = ["docs/{name}.md"]
  frontmatter_require = ["layout"]
  frontmatter_forbid  = ["layout"]
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for frontmatter require/forbid overlap")
	}
	if !strings.Contains(err.Error(), "layout") {
		t.Errorf("error should name the overlapping field; got: %v", err)
	}
}

func TestType_ResolveDocPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		typ            config.Type
		target, prefix string
		want           []string
	}{
		"resource both layouts": {
			typ: config.Type{
				WebsitePaths: []string{
					"docs/resources/{name}.md",
					"website/docs/r/{name}.html.markdown",
				},
			},
			target: "aws_instance",
			prefix: "aws",
			want: []string{
				"docs/resources/instance.md",
				"website/docs/r/instance.html.markdown",
			},
		},
		"no prefix strip when target already bare": {
			typ:    config.Type{WebsitePaths: []string{"docs/{name}.md"}},
			target: "instance",
			prefix: "aws",
			want:   []string{"docs/instance.md"},
		},
		"fixed path (no {name}) returns template verbatim": {
			typ:    config.Type{WebsitePaths: []string{"docs/index.md"}},
			target: "aws",
			prefix: "aws",
			want:   []string{"docs/index.md"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := tt.typ.ResolveDocPath(tt.target, tt.prefix)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ResolveDocPath(%q, %q) = %v, want %v", tt.target, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestSectionRequirement_IsValid(t *testing.T) {
	t.Parallel()

	cases := map[config.SectionRequirement]bool{
		"":          true,
		"required":  true,
		"optional":  true,
		"forbidden": true,
		"REQUIRED":  false,
		"must":      false,
		"foo":       false,
	}
	for in, want := range cases {
		if got := in.IsValid(); got != want {
			t.Errorf("SectionRequirement(%q).IsValid() = %v, want %v", in, got, want)
		}
	}
}

// TestLoad_DefaultsValidateCleanly is a safety net — the embedded defaults
// must never fail their own validator, even under refactoring. If this
// breaks, we've shipped a broken defaults.hcl.
func TestLoad_DefaultsValidateCleanly(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.hcl"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, tp := range cfg.Types {
		// Validate via re-calling (Load already validated; this confirms the
		// check is idempotent and the public Validate matches).
		if err := tp.Validate(); err != nil {
			t.Errorf("default type %q failed re-validation: %v", tp.Name, err)
		}
	}
}

// Sanity: the helper used by other tests in this package is still where it
// was before phase 3. Kept here so the file stands alone during review.
var _ = os.WriteFile
