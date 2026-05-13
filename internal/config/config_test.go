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

// TestLoad_NoFile_ReturnsEmpty documents the graceful fallback: a missing
// config file is not an error, just a zero-value Config. Callers then rely on
// CLI flags to supply everything.
func TestLoad_NoFile_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.hcl"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil Config for missing file")
	}
	if cfg.ProviderSource != "" || len(cfg.Checks) != 0 {
		t.Errorf("expected zero-value Config, got %+v", cfg)
	}
}

// TestLoad_PathsPassThroughUnmodified pins swissshepherd's path-resolution
// contract: whatever the config says is exactly what Load returns. The OS
// interprets relative paths against the caller's CWD downstream. The most
// important regression this guards against is having Load silently rewrite
// paths by joining them to the config file's directory (the old behavior,
// removed because it produced confusing errors for configs kept in .ci/ or
// similar subdirectories).
func TestLoad_PathsPassThroughUnmodified(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sub", "swissshepherd.hcl")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const body = `
provider_source = "registry.terraform.io/hashicorp/test"
provider_dir    = "."
schema_json     = "schema.json"
`
	writeFile(t, cfgPath, body)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"provider_dir", cfg.ProviderDir, "."},
		{"schema_json", cfg.SchemaJSON, "schema.json"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q (Load must not rewrite relative paths)", tt.name, tt.got, tt.want)
		}
	}
}

// TestLoad_AllowedSubcategoriesFile_RelativeToCWD simulates the real failure
// case the user hit: config lives in a subdirectory (.ci/), the referenced
// file lives at the project root, and swissshepherd is invoked from the
// project root. The path is resolved relative to the process CWD, not to the
// config file's directory.
//
// Not parallel: t.Chdir mutates process-global state.
func TestLoad_AllowedSubcategoriesFile_RelativeToCWD(t *testing.T) {
	root := t.TempDir()

	// Layout: root/website/allowed-subcategories.txt + root/.ci/swissshepherd.hcl
	writeFile(t, filepath.Join(root, "website", "allowed-subcategories.txt"),
		"Alpha\nBravo\nCharlie\n")
	cfgPath := filepath.Join(root, ".ci", "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allowed_subcategories_file = "website/allowed-subcategories.txt"
}
`)

	// t.Chdir (Go 1.24+) scopes the directory change to this test.
	t.Chdir(root)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	fm := cfg.GetCheck("frontmatter")
	want := []string{"Alpha", "Bravo", "Charlie"}
	if !slices.Equal(fm.AllowedSubcategories, want) {
		t.Errorf("AllowedSubcategories = %v, want %v", fm.AllowedSubcategories, want)
	}
}

// TestLoad_AllowedSubcategoriesFile_Absolute confirms absolute paths aren't
// affected by the CWD and work regardless of where swissshepherd is invoked.
func TestLoad_AllowedSubcategoriesFile_Absolute(t *testing.T) {
	t.Parallel()

	listPath := filepath.Join(t.TempDir(), "allowed.txt")
	writeFile(t, listPath, "OnlyOne\n")

	cfgPath := filepath.Join(t.TempDir(), "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allowed_subcategories_file = "`+listPath+`"
}
`)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	fm := cfg.GetCheck("frontmatter")
	if !slices.Equal(fm.AllowedSubcategories, []string{"OnlyOne"}) {
		t.Errorf("AllowedSubcategories = %v, want [OnlyOne]", fm.AllowedSubcategories)
	}
}

// TestLoad_AllowedSubcategoriesFile_MissingErrors ensures the error path stays
// clear. The message must include the path so CI failures are diagnosable.
func TestLoad_AllowedSubcategoriesFile_MissingErrors(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allowed_subcategories_file = "does-not-exist.txt"
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing allowed_subcategories_file")
	}
	if !strings.Contains(err.Error(), "does-not-exist.txt") {
		t.Errorf("error message should name the missing file; got: %v", err)
	}
}

// TestLoad_AllowedSubcategoriesFileAndInlineMerge confirms inline values are
// preserved and the file-loaded values are appended, matching the behavior
// every *_file option in CheckConfig shares.
//
// Not parallel: t.Chdir mutates process-global state.
func TestLoad_AllowedSubcategoriesFileAndInlineMerge(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "allowed.txt"), "FromFile\n")
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allowed_subcategories      = ["Inline"]
  allowed_subcategories_file = "allowed.txt"
}
`)
	t.Chdir(root)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	fm := cfg.GetCheck("frontmatter")
	want := []string{"Inline", "FromFile"}
	if !slices.Equal(fm.AllowedSubcategories, want) {
		t.Errorf("AllowedSubcategories = %v, want %v", fm.AllowedSubcategories, want)
	}
}

// TestIsCheckEnabled_DefaultTrue pins the default-on semantics: a check not
// named in the config is on. A check that appears with enabled = false is off.
func TestIsCheckEnabled_DefaultTrue(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Checks: []config.CheckConfig{
			{Name: "computed_attribute", Enabled: false},
		},
	}

	tests := []struct {
		name string
		want bool
	}{
		{"completeness", true},        // not mentioned → enabled
		{"computed_attribute", false}, // explicitly disabled
		{"nonexistent_rule", true},    // future-compat: unknown → enabled
	}
	for _, tt := range tests {
		if got := cfg.IsCheckEnabled(tt.name); got != tt.want {
			t.Errorf("IsCheckEnabled(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestProviderName pulls the short name out of a provider source address.
func TestProviderName(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"registry.terraform.io/hashicorp/aws":  "aws",
		"registry.terraform.io/hashicorp/test": "test",
		"aws":                                  "aws",
		"":                                     "",
	}
	for source, want := range tests {
		cfg := &config.Config{ProviderSource: source}
		if got := cfg.ProviderName(); got != want {
			t.Errorf("ProviderName(%q) = %q, want %q", source, got, want)
		}
	}
}

// --- helpers -------------------------------------------------------------

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
