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

// TestLoad_AllowSubcategoriesFile_RelativeToCWD simulates the real failure
// case the user hit: config lives in a subdirectory (.ci/), the referenced
// file lives at the project root, and swissshepherd is invoked from the
// project root. The path is resolved relative to the process CWD, not to the
// config file's directory.
//
// Not parallel: t.Chdir mutates process-global state.
func TestLoad_AllowSubcategoriesFile_RelativeToCWD(t *testing.T) {
	root := t.TempDir()

	// Layout: root/website/allowed-subcategories.txt + root/.ci/swissshepherd.hcl
	writeFile(t, filepath.Join(root, "website", "allowed-subcategories.txt"),
		"Alpha\nBravo\nCharlie\n")
	cfgPath := filepath.Join(root, ".ci", "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allow_subcategories_file = "website/allowed-subcategories.txt"
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
	if !slices.Equal(fm.AllowSubcategories, want) {
		t.Errorf("AllowSubcategories = %v, want %v", fm.AllowSubcategories, want)
	}
}

// TestLoad_AllowSubcategoriesFile_Absolute confirms absolute paths aren't
// affected by the CWD and work regardless of where swissshepherd is invoked.
func TestLoad_AllowSubcategoriesFile_Absolute(t *testing.T) {
	t.Parallel()

	listPath := filepath.Join(t.TempDir(), "allowed.txt")
	writeFile(t, listPath, "OnlyOne\n")

	cfgPath := filepath.Join(t.TempDir(), "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allow_subcategories_file = "`+listPath+`"
}
`)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	fm := cfg.GetCheck("frontmatter")
	if !slices.Equal(fm.AllowSubcategories, []string{"OnlyOne"}) {
		t.Errorf("AllowSubcategories = %v, want [OnlyOne]", fm.AllowSubcategories)
	}
}

// TestLoad_AllowSubcategoriesFile_MissingErrors ensures the error path stays
// clear. The message must include the path so CI failures are diagnosable.
func TestLoad_AllowSubcategoriesFile_MissingErrors(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allow_subcategories_file = "does-not-exist.txt"
}
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing allow_subcategories_file")
	}
	if !strings.Contains(err.Error(), "does-not-exist.txt") {
		t.Errorf("error message should name the missing file; got: %v", err)
	}
}

// TestLoad_AllowSubcategoriesFileAndInlineMerge confirms inline values are
// preserved and the file-loaded values are appended, matching the behavior
// every *_file option in CheckConfig shares.
//
// Not parallel: t.Chdir mutates process-global state.
func TestLoad_AllowSubcategoriesFileAndInlineMerge(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "allowed.txt"), "FromFile\n")
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	writeFile(t, cfgPath, `
check "frontmatter" {
  enabled                    = true
  allow_subcategories      = ["Inline"]
  allow_subcategories_file = "allowed.txt"
}
`)
	t.Chdir(root)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	fm := cfg.GetCheck("frontmatter")
	want := []string{"Inline", "FromFile"}
	if !slices.Equal(fm.AllowSubcategories, want) {
		t.Errorf("AllowSubcategories = %v, want %v", fm.AllowSubcategories, want)
	}
}

// TestIsCheckEnabled_DefaultTrue pins the default-on semantics: a check not
// named in the config is on. A check that appears with enabled = false is off.
func TestIsCheckEnabled_DefaultTrue(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Checks: []config.CheckConfig{
			{Name: "attributes_section", Enabled: false},
		},
	}

	tests := []struct {
		name string
		want bool
	}{
		{"arguments_section", true},   // not mentioned → enabled
		{"attributes_section", false}, // explicitly disabled
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

// TestCheckConfig_AppliesTo_EmptyMatchesEverything pins the default-on
// semantic: a CheckConfig with no path-scoping lists admits every target,
// matching the "check applies everywhere" baseline that existed before
// phase 3.
func TestCheckConfig_AppliesTo_EmptyMatchesEverything(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{Name: "ordering"}

	for _, tc := range []struct{ name, typeName string }{
		{"aws_s3_bucket", "resource"},
		{"aws_s3_bucket", "data_source"},
		{"aws_format", "function"},
		{"", ""},
	} {
		if !cc.AppliesTo(tc.name, tc.typeName) {
			t.Errorf("AppliesTo(%q, %q) = false, want true (empty CheckConfig should admit everything)", tc.name, tc.typeName)
		}
	}
}

// TestCheckConfig_AppliesTo_Types covers the type-axis allowlist:
// populated Types scopes the check to listed type names.
func TestCheckConfig_AppliesTo_Types(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:  "ordering",
		Types: []string{"resource", "data_source"},
	}

	tests := map[string]struct {
		name     string
		typeName string
		want     bool
	}{
		"resource included":     {"aws_s3_bucket", "resource", true},
		"data_source included":  {"aws_s3_bucket", "data_source", true},
		"ephemeral excluded":    {"aws_secret", "ephemeral", false},
		"function excluded":     {"aws_format", "function", false},
		"list_resource missing": {"aws_instances", "list_resource", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := cc.AppliesTo(tt.name, tt.typeName); got != tt.want {
				t.Errorf("AppliesTo(%q, %q) = %v, want %v", tt.name, tt.typeName, got, tt.want)
			}
		})
	}
}

// TestCheckConfig_AppliesTo_Prefixes covers the prefix-axis allowlist. An
// arbitrary type passes as long as its name has one of the listed prefixes.
func TestCheckConfig_AppliesTo_Prefixes(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:     "ordering",
		Prefixes: []string{"aws_s3", "aws_appflow"},
	}

	tests := map[string]struct {
		name string
		want bool
	}{
		"prefix aws_s3 matches":      {"aws_s3_bucket", true},
		"prefix aws_s3 sub-resource": {"aws_s3_bucket_policy", true},
		"prefix aws_appflow matches": {"aws_appflow_flow", true},
		"no prefix match":            {"aws_ec2_instance", false},
		"exact prefix length name":   {"aws_s3", true},
		"empty name never matches":   {"", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := cc.AppliesTo(tt.name, "resource"); got != tt.want {
				t.Errorf("AppliesTo(%q, resource) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestCheckConfig_AppliesTo_Targets covers the exact-name allowlist.
func TestCheckConfig_AppliesTo_Targets(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:    "ordering",
		Targets: []string{"aws_instance", "aws_vpc"},
	}

	tests := map[string]struct {
		name string
		want bool
	}{
		"listed target":           {"aws_instance", true},
		"second listed target":    {"aws_vpc", true},
		"unlisted target":         {"aws_s3_bucket", false},
		"prefix of listed target": {"aws_", false},
		"extension of listed":     {"aws_instance_extra", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := cc.AppliesTo(tt.name, "resource"); got != tt.want {
				t.Errorf("AppliesTo(%q, resource) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestCheckConfig_AppliesTo_PrefixesAndTargetsAreOred confirms the two
// name-axis lists compose via OR — either list matching is enough.
func TestCheckConfig_AppliesTo_PrefixesAndTargetsAreOred(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:     "ordering",
		Prefixes: []string{"aws_s3"},
		Targets:  []string{"aws_instance"},
	}

	tests := map[string]struct {
		name string
		want bool
	}{
		"prefix match":       {"aws_s3_bucket", true},
		"exact target match": {"aws_instance", true},
		"neither match":      {"aws_vpc", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := cc.AppliesTo(tt.name, "resource"); got != tt.want {
				t.Errorf("AppliesTo(%q, resource) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestCheckConfig_AppliesTo_TypesAndNameAreAnded is the composition test —
// when both the type axis and at least one name axis is populated, a target
// must satisfy BOTH. This is the "migrate aws_s3 resources only, leave data
// sources for later" pattern the user called out.
func TestCheckConfig_AppliesTo_TypesAndNameAreAnded(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:     "ordering",
		Types:    []string{"resource"},
		Prefixes: []string{"aws_s3"},
	}

	tests := map[string]struct {
		name     string
		typeName string
		want     bool
	}{
		"resource + prefix match":       {"aws_s3_bucket", "resource", true},
		"resource + no prefix match":    {"aws_ec2_instance", "resource", false},
		"data source + prefix match":    {"aws_s3_bucket", "data_source", false},
		"data source + no prefix match": {"aws_ec2_instance", "data_source", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := cc.AppliesTo(tt.name, tt.typeName); got != tt.want {
				t.Errorf("AppliesTo(%q, %q) = %v, want %v", tt.name, tt.typeName, got, tt.want)
			}
		})
	}
}

// TestCheckConfig_AppliesTo_IgnoreTargetsWins locks down the deny-wins
// semantic: IgnoreTargets excludes a name even when every allowlist would
// include it.
func TestCheckConfig_AppliesTo_IgnoreTargetsWins(t *testing.T) {
	t.Parallel()

	cc := config.CheckConfig{
		Name:          "ordering",
		Prefixes:      []string{"aws_s3"},
		Targets:       []string{"aws_s3_bucket"},
		IgnoreTargets: []string{"aws_s3_bucket"},
	}

	if cc.AppliesTo("aws_s3_bucket", "resource") {
		t.Error("IgnoreTargets must win over allowlists, but AppliesTo returned true")
	}
	// Other targets under the prefix still match.
	if !cc.AppliesTo("aws_s3_bucket_policy", "resource") {
		t.Error("ignored target should not affect other names")
	}
}

// TestLoad_IgnoreTargetsFile confirms the file form loads into IgnoreTargets
// alongside any inline list, matching every other *_file option's behavior.
//
// Not parallel: t.Chdir mutates process-global state.
func TestLoad_IgnoreTargetsFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/ignored.txt", "# comment line\naws_legacy_one\naws_legacy_two\n\n")
	cfgPath := root + "/swissshepherd.hcl"
	writeFile(t, cfgPath, `
check "ordering" {
  enabled               = true
  ignore_targets       = ["aws_inline_ignore"]
  ignore_targets_file  = "ignored.txt"
}
`)

	t.Chdir(root)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cc := cfg.GetCheck("ordering")
	want := []string{"aws_inline_ignore", "aws_legacy_one", "aws_legacy_two"}
	if len(cc.IgnoreTargets) != len(want) {
		t.Fatalf("IgnoreTargets = %v, want %v", cc.IgnoreTargets, want)
	}
	for i, v := range want {
		if cc.IgnoreTargets[i] != v {
			t.Errorf("IgnoreTargets[%d] = %q, want %q", i, cc.IgnoreTargets[i], v)
		}
	}
	// Cross-check: AppliesTo actually uses the loaded list.
	if cc.AppliesTo("aws_legacy_one", "resource") {
		t.Error("file-loaded ignored target should exclude AppliesTo")
	}
}

// TestLoad_SkipBlocksFile confirms skip_blocks_file merges into SkipBlocks
// after the inline list, and that a missing file is an error.
//
// Not parallel: t.Chdir mutates process-global state.
func TestLoad_SkipBlocksFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/skip.txt", "# WAFv2 statement nesting\nrule.statement.and_statement\n\nrule.statement.or_statement\n")
	writeFile(t, root+"/ok.hcl", `
check "schema_docs" {
  enabled          = true
  skip_blocks      = ["timeouts"]
  skip_blocks_file = "skip.txt"
}
`)
	writeFile(t, root+"/missing.hcl", `
check "schema_docs" {
  enabled          = true
  skip_blocks_file = "nope.txt"
}
`)
	t.Chdir(root)

	cfg, err := config.Load("ok.hcl")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{"timeouts", "rule.statement.and_statement", "rule.statement.or_statement"}
	if got := cfg.GetCheck("schema_docs").SkipBlocks; !slices.Equal(got, want) {
		t.Errorf("SkipBlocks = %q, want %q", got, want)
	}
	// A file listing nothing still replaces the default, as an empty
	// inline list would.
	writeFile(t, root+"/empty.txt", "# nothing skipped\n\n")
	writeFile(t, root+"/empty.hcl", `
check "schema_docs" {
  enabled          = true
  skip_blocks_file = "empty.txt"
}
`)
	cfg, err = config.Load("empty.hcl")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.GetCheck("schema_docs").SkipBlocks; got == nil || len(got) != 0 {
		t.Errorf("SkipBlocks from an empty file = %#v, want a non-nil empty slice", got)
	}
	if _, err := config.Load("missing.hcl"); err == nil || !strings.Contains(err.Error(), "nope.txt") {
		t.Errorf("Load() with a missing skip_blocks_file: error = %v, want one naming nope.txt", err)
	}
}

func TestCheckConfig_AppliesTo_QualifiedTargets(t *testing.T) {
	t.Parallel()
	cc := config.CheckConfig{Targets: []string{"data_source/aws_thing"}}

	if !cc.AppliesTo("aws_thing", "data_source") {
		t.Error("qualified target should match when type matches")
	}
	if cc.AppliesTo("aws_thing", "resource") {
		t.Error("qualified target should not match different type")
	}
}

func TestCheckConfig_AppliesTo_QualifiedPrefixes(t *testing.T) {
	t.Parallel()
	cc := config.CheckConfig{Prefixes: []string{"data_source/aws_s3"}}

	if !cc.AppliesTo("aws_s3_bucket", "data_source") {
		t.Error("qualified prefix should match when type and prefix match")
	}
	if cc.AppliesTo("aws_s3_bucket", "resource") {
		t.Error("qualified prefix should not match different type")
	}
	if cc.AppliesTo("aws_ec2_instance", "data_source") {
		t.Error("qualified prefix should not match different name")
	}
}

func TestCheckConfig_AppliesTo_QualifiedIgnoreTargets(t *testing.T) {
	t.Parallel()
	cc := config.CheckConfig{IgnoreTargets: []string{"data_source/aws_thing"}}

	if cc.AppliesTo("aws_thing", "data_source") {
		t.Error("qualified ignore_target should exclude matching type")
	}
	if !cc.AppliesTo("aws_thing", "resource") {
		t.Error("qualified ignore_target should not exclude different type")
	}
}

func TestCheckConfig_AppliesTo_QualifiedIgnorePrefixes(t *testing.T) {
	t.Parallel()
	cc := config.CheckConfig{IgnorePrefixes: []string{"resource/aws_legacy"}}

	if cc.AppliesTo("aws_legacy_thing", "resource") {
		t.Error("qualified ignore_prefix should exclude matching type+prefix")
	}
	if !cc.AppliesTo("aws_legacy_thing", "data_source") {
		t.Error("qualified ignore_prefix should not exclude different type")
	}
}

// TestLoad_Overrides covers override blocks: targets_file merges into
// targets, and an override that would be ignored or ambiguous is an error.
//
// Not parallel: t.Chdir mutates process-global state.
func TestLoad_Overrides(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/quicksight.txt", "# reuse graphs\nresource/aws_quicksight_template\n")
	t.Chdir(root)

	testCases := map[string]struct {
		hcl     string
		wantErr string
	}{
		"targets and targets_file": {
			hcl: `check "schema_docs" {
  override {
    targets      = ["resource/aws_quicksight_analysis"]
    targets_file = "quicksight.txt"
    coverage     = false
  }
}`,
		},
		"outside schema_docs": {
			hcl:     `check "ordering" {` + "\n  override {\n    targets = [\"aws_x\"]\n    coverage = false\n  }\n}",
			wantErr: `check "ordering": override blocks are only supported in check "schema_docs"`,
		},
		"no targets": {
			hcl:     `check "schema_docs" {` + "\n  override {\n    coverage = false\n  }\n}",
			wantErr: `override 1 lists no targets`,
		},
		"no toggle": {
			hcl:     `check "schema_docs" {` + "\n  override {\n    targets = [\"aws_x\"]\n  }\n}",
			wantErr: `override 1 sets no sub-check`,
		},
		"bare and qualified overlap": {
			hcl: `check "schema_docs" {
  override {
    targets  = ["aws_x"]
    coverage = false
  }
  override {
    targets = ["resource/aws_x"]
    labels  = false
  }
}`,
			wantErr: `target "resource/aws_x" is matched by overrides 1 and 2`,
		},
		"same name, different types": {
			hcl: `check "schema_docs" {
  override {
    targets  = ["data_source/aws_x"]
    coverage = false
  }
  override {
    targets = ["resource/aws_x"]
    labels  = false
  }
}`,
		},
		"missing targets_file": {
			hcl:     `check "schema_docs" {` + "\n  override {\n    targets_file = \"nope.txt\"\n    coverage = false\n  }\n}",
			wantErr: "nope.txt",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, strings.ReplaceAll(name, " ", "_")+".hcl")
			writeFile(t, path, tc.hcl)
			cfg, err := config.Load(path)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Load() error = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if name != "targets and targets_file" {
				return
			}
			o := cfg.GetCheck("schema_docs").Overrides
			want := []string{"resource/aws_quicksight_analysis", "resource/aws_quicksight_template"}
			if len(o) != 1 || !slices.Equal(o[0].Targets, want) || o[0].Coverage == nil || *o[0].Coverage {
				t.Errorf("Overrides = %+v, want one with targets %q and coverage = false", o, want)
			}
			if !o[0].Matches("aws_quicksight_template", "resource") || o[0].Matches("aws_quicksight_template", "data_source") {
				t.Error("Matches: a qualified target must match only its type")
			}
		})
	}
}

// TestLoad_DuplicateCheck: two check blocks with one name are a load error
// naming both lines (#96). For schema_docs the message points at override
// blocks, which is usually what the second block was reaching for.
func TestLoad_DuplicateCheck(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		hcl     string
		wantErr string
	}{
		"schema_docs twice": {
			hcl:     "check \"schema_docs\" {\n  coverage = true\n}\n\ncheck \"ordering\" {\n  enabled = true\n}\n\ncheck \"schema_docs\" {\n  labels = false\n}\n",
			wantErr: `check "schema_docs" is defined twice (lines 1 and 9); use one block with override blocks for per-target sub-check settings`,
		},
		"another check twice": {
			hcl:     "check \"ordering\" {\n  enabled = true\n}\ncheck \"ordering\" {\n  enabled = false\n}\n",
			wantErr: `check "ordering" is defined twice (lines 1 and 4); merge them into one block`,
		},
		"distinct names": {
			hcl: "check \"schema_docs\" {\n  coverage = true\n}\ncheck \"ordering\" {\n  enabled = true\n}\n",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "swissshepherd.hcl")
			writeFile(t, path, tc.hcl)
			_, err := config.Load(path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Load() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != "config "+path+": "+tc.wantErr {
				t.Fatalf("Load() error = %v, want %q", err, "config "+path+": "+tc.wantErr)
			}
		})
	}
}

func TestLoad_ProseCasingValidation(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		hcl     string
		wantErr string
	}{
		"already lowercase entry": {
			hcl:     "check \"prose_casing\" {\n  enabled = true\n  enforce_casing = [\"arn\"]\n}\n",
			wantErr: `check "prose_casing": enforce_casing entry "arn" is already lowercase; it would suggest itself`,
		},
		"two entries differ only in case": {
			hcl:     "check \"prose_casing\" {\n  enabled = true\n  enforce_casing = [\"ARN\", \"Arn\"]\n}\n",
			wantErr: `check "prose_casing": enforce_casing entries "ARN" and "Arn" differ only in case`,
		},
		"two entries differ only in case once trimmed": {
			// Untrimmed, " Arn " != "arn" lowercased, so a comparison that
			// skipped trimming would miss this collision and let it reach
			// NewProseCasingRule, which trims before merging and would
			// silently let the second entry overwrite the first instead of
			// erroring.
			hcl:     "check \"prose_casing\" {\n  enabled = true\n  enforce_casing = [\"ARN\", \" Arn \"]\n}\n",
			wantErr: `check "prose_casing": enforce_casing entries "ARN" and "Arn" differ only in case`,
		},
		"two entries differ only in spacing": {
			hcl:     "check \"prose_casing\" {\n  enabled = true\n  enforce_casing = [\"AutoScaling\", \"Auto Scaling\"]\n}\n",
			wantErr: `check "prose_casing": enforce_casing entries "AutoScaling" and "Auto Scaling" differ only in case and spacing, so they match the same text; keep one`,
		},
		"repeated inner spaces collapse before comparing": {
			hcl:     "check \"prose_casing\" {\n  enabled = true\n  enforce_casing = [\"API Gateway\", \"API  gateway\"]\n}\n",
			wantErr: `check "prose_casing": enforce_casing entries "API Gateway" and "API gateway" differ only in case`,
		},
		"already lowercase multi-word entry": {
			hcl:     "check \"prose_casing\" {\n  enabled = true\n  enforce_casing = [\"auto scaling\"]\n}\n",
			wantErr: `check "prose_casing": enforce_casing entry "auto scaling" is already lowercase; it would suggest itself`,
		},
		"valid entries": {
			hcl: "check \"prose_casing\" {\n  enabled = true\n  enforce_casing = [\"ARN\", \"DynamoDB\", \"API Gateway\", \"Auto Scaling group\"]\n}\n",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "swissshepherd.hcl")
			writeFile(t, path, tc.hcl)
			_, err := config.Load(path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Load() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != "config "+path+": "+tc.wantErr {
				t.Fatalf("Load() error = %v, want %q", err, "config "+path+": "+tc.wantErr)
			}
		})
	}
}
