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
		"website/docs/r/{name}.html.markdown",
	}
	if !slices.Equal(r.WebsitePaths, wantPaths) {
		t.Errorf("WebsitePaths = %v, want %v", r.WebsitePaths, wantPaths)
	}
	// Resource must declare its sections in the canonical order.
	wantSectionNames := []config.SectionName{
		config.SectionTitle,
		config.SectionExample,
		config.SectionArguments,
		config.SectionAttributes,
		config.SectionTimeouts,
		config.SectionImport,
		config.SectionSignature,
	}
	gotNames := make([]config.SectionName, len(r.Sections))
	for i, s := range r.Sections {
		gotNames[i] = s.SectionName()
	}
	if !slices.Equal(gotNames, wantSectionNames) {
		t.Errorf("Sections order = %v, want %v", gotNames, wantSectionNames)
	}
	// Spot-check key required and forbidden flags.
	specByName := make(map[config.SectionName]config.SectionSpec, len(r.Sections))
	for _, s := range r.Sections {
		specByName[s.SectionName()] = s
	}
	if !specByName[config.SectionAttributes].Required {
		t.Errorf("Attributes should be required for resource type")
	}
	if !specByName[config.SectionSignature].Forbidden {
		t.Errorf("Signature should be forbidden for resource type")
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

func TestLoad_InvalidType_InvalidSectionName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "broken" {
  schema_kind   = "none"
  website_paths = ["docs/{name}.md"]
  section "Not-Snake-Case" {
    required = true
  }
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid section name")
	}
	if !strings.Contains(err.Error(), "Not-Snake-Case") {
		t.Errorf("error should name the offending section; got: %v", err)
	}
}

// TestLoad_CustomSectionNameAccepted documents that any lowercase
// snake_case identifier is valid as a section name; canonical names get
// special parser support, custom names are matched by heading text.
func TestLoad_CustomSectionNameAccepted(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "ephemeral_with_notes" {
  schema_kind   = "ephemeral"
  website_paths = ["website/docs/ephemeral-resources/{name}.html.markdown"]
  title_prefix  = "Ephemeral"

  section "title"       { required = true }
  section "example"     { required = true }
  section "arguments"   { required = true }
  section "attributes"  { required = true }
  section "usage_notes" {}
}
`)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tp := cfg.GetType("ephemeral_with_notes")
	if tp == nil {
		t.Fatal("custom type not loaded")
	}
	if len(tp.Sections) != 5 {
		t.Fatalf("expected 5 sections, got %d", len(tp.Sections))
	}
	last := tp.Sections[4]
	if last.Name != "usage_notes" {
		t.Errorf("last section name = %q, want %q", last.Name, "usage_notes")
	}
	if got := last.SectionName().HeadingText(); got != "Usage Notes" {
		t.Errorf("HeadingText(usage_notes) = %q, want %q", got, "Usage Notes")
	}
}

func TestLoad_InvalidType_DuplicateSection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "broken" {
  schema_kind   = "none"
  website_paths = ["docs/{name}.md"]
  section "arguments" { required = true }
  section "arguments" {}
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for duplicate section")
	}
	if !strings.Contains(err.Error(), "more than once") {
		t.Errorf("error should explain duplicate sections; got: %v", err)
	}
}

func TestLoad_InvalidType_RequiredAndForbiddenSection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
type "broken" {
  schema_kind   = "none"
  website_paths = ["docs/{name}.md"]
  section "arguments" {
    required  = true
    forbidden = true
  }
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for both required and forbidden")
	}
	if !strings.Contains(err.Error(), "both required and forbidden") {
		t.Errorf("error should explain the conflict; got: %v", err)
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

func TestSectionName_IsValid(t *testing.T) {
	t.Parallel()

	cases := map[config.SectionName]bool{
		// Canonical names.
		config.SectionTitle:      true,
		config.SectionSignature:  true,
		config.SectionExample:    true,
		config.SectionArguments:  true,
		config.SectionAttributes: true,
		config.SectionTimeouts:   true,
		config.SectionImport:     true,
		// Custom snake_case names.
		"usage_notes":           true,
		"dependency_management": true,
		"foo123":                true,
		// Invalid: empty, uppercase, hyphens, spaces, special chars.
		"":            false,
		"TITLE":       false,
		"Title":       false,
		"foo-bar":     false,
		"foo bar":     false,
		"foo.bar":     false,
		"_underscore": true, // underscore start is fine — still snake_case
	}
	for in, want := range cases {
		if got := in.IsValid(); got != want {
			t.Errorf("SectionName(%q).IsValid() = %v, want %v", in, got, want)
		}
	}
}

func TestSectionName_IsCanonical(t *testing.T) {
	t.Parallel()

	cases := map[config.SectionName]bool{
		config.SectionTitle:      true,
		config.SectionSignature:  true,
		config.SectionExample:    true,
		config.SectionArguments:  true,
		config.SectionAttributes: true,
		config.SectionTimeouts:   true,
		config.SectionImport:     true,
		"usage_notes":            false,
		"dependency_management":  false,
		"":                       false,
	}
	for in, want := range cases {
		if got := in.IsCanonical(); got != want {
			t.Errorf("SectionName(%q).IsCanonical() = %v, want %v", in, got, want)
		}
	}
}

func TestSectionName_HeadingText(t *testing.T) {
	t.Parallel()

	cases := map[config.SectionName]string{
		config.SectionTitle:      "<title>",
		config.SectionSignature:  "Signature",
		config.SectionExample:    "Example Usage",
		config.SectionArguments:  "Argument Reference",
		config.SectionAttributes: "Attribute Reference",
		config.SectionTimeouts:   "Timeouts",
		config.SectionImport:     "Import",
		// Custom snake_case → Title Case.
		"usage_notes":           "Usage Notes",
		"dependency_management": "Dependency Management",
		"single":                "Single",
		// Edge cases: leading/trailing/double underscores must not
		// produce leading/trailing/double spaces in the heading text.
		"_underscore":   "Underscore",
		"trailing_":     "Trailing",
		"double__under": "Double Under",
	}
	for in, want := range cases {
		if got := in.HeadingText(); got != want {
			t.Errorf("SectionName(%q).HeadingText() = %q, want %q", in, got, want)
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
