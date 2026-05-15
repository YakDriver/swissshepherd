// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package config_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/config"
)

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
