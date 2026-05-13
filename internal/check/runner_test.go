// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// captureRule records every call so tests can assert which (resource, kind)
// pairs the Runner dispatched to. Thread-safe because Runner iterates
// sequentially but tests may run in parallel.
type captureRule struct {
	name string

	mu   sync.Mutex
	seen []captureCall
}

type captureCall struct {
	Resource  string
	HasSchema bool
	HasDoc    bool
}

func (c *captureRule) Name() string { return c.name }

func (c *captureRule) Check(resource string, rs *schema.ResourceSchema, d *doc.Document) []check.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, captureCall{
		Resource:  resource,
		HasSchema: rs != nil,
		HasDoc:    d != nil,
	})
	return nil
}

func (c *captureRule) resources() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.seen))
	for _, call := range c.seen {
		out = append(out, call.Resource)
	}
	slices.Sort(out)
	return out
}

// runnerFixture wires up a fully working Runner against a temp directory of
// doc files, with a configurable set of types and schema contents. It's the
// swiss-army-knife for every test in this file — every scenario is a few
// lines of "make this target exist with this body in this type".
type runnerFixture struct {
	root    string
	cfg     *config.Config
	ps      *schema.ProviderSchema
	capture *captureRule
	runner  *check.Runner
}

// newRunnerFixture returns a fixture with an empty schema and the embedded
// default types. Tests call addResource / addDataSource / etc. to populate
// targets, then Run, RunOne, or RunPrefix through the runner.
func newRunnerFixture(t *testing.T) *runnerFixture {
	t.Helper()

	root := t.TempDir()

	// Build a minimal HCL config that just sets provider_source + provider_dir
	// so Runner resolves doc paths relative to our temp root. Types come from
	// embedded defaults.
	cfgPath := filepath.Join(root, "swissshepherd.hcl")
	body := `
provider_source = "registry.terraform.io/hashicorp/test"
provider_dir    = "` + root + `"
schema_json     = "unused.json"
`
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	ps := &schema.ProviderSchema{
		Resources:     map[string]*schema.ResourceSchema{},
		DataSources:   map[string]*schema.ResourceSchema{},
		Ephemerals:    map[string]*schema.ResourceSchema{},
		ListResources: map[string]*schema.ResourceSchema{},
		Actions:       map[string]*schema.ResourceSchema{},
		Functions:     map[string]*schema.FunctionSchema{},
	}

	capture := &captureRule{name: "capture"}

	runner := &check.Runner{
		Schema:           ps,
		Config:           cfg,
		Rules:            []check.Rule{capture},
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		HeadingTemplates: doc.DefaultHeadingTemplates(),
	}

	return &runnerFixture{
		root:    root,
		cfg:     cfg,
		ps:      ps,
		capture: capture,
		runner:  runner,
	}
}

// addTarget registers a target with the given kind + name and writes a
// minimal doc file to the first path the configured type would resolve. The
// "test" provider_source maps to providerName "test" so doc files live at
// e.g. "<root>/docs/resources/instance.md" for "test_instance".
func (f *runnerFixture) addTarget(t *testing.T, typeName, targetName string) {
	t.Helper()

	typ := f.cfg.GetType(typeName)
	if typ == nil {
		t.Fatalf("unknown type %q", typeName)
	}

	// Write to the first website_path candidate, which must be a template
	// containing {name}. The fixture intentionally avoids non-templated
	// defaults (index) because those don't participate in target iteration.
	candidates := typ.ResolveDocPath(targetName, f.cfg.ProviderName())
	if len(candidates) == 0 {
		t.Fatalf("type %q resolves to no candidate paths for %q", typeName, targetName)
	}

	docPath := filepath.Join(f.root, candidates[0])
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(docPath), err)
	}
	body := `---
subcategory: "Test"
description: |-
  Exists for tests.
---

# ` + titlePrefix(typ) + `: ` + targetName + `

Body.
`
	if err := os.WriteFile(docPath, []byte(body), 0o600); err != nil {
		t.Fatalf("writing doc %s: %v", docPath, err)
	}

	// Register in the appropriate schema map so TargetNames / ResourceSchemaFor
	// return the expected results.
	rs := &schema.ResourceSchema{Name: targetName, Blocks: map[string]*schema.Block{}}
	switch typ.SchemaKind {
	case schema.KindResource:
		f.ps.Resources[targetName] = rs
	case schema.KindDataSource:
		f.ps.DataSources[targetName] = rs
	case schema.KindEphemeral:
		f.ps.Ephemerals[targetName] = rs
	case schema.KindListResource:
		f.ps.ListResources[targetName] = rs
	case schema.KindAction:
		f.ps.Actions[targetName] = rs
	case schema.KindFunction:
		f.ps.Functions[targetName] = &schema.FunctionSchema{Name: targetName}
	default:
		t.Fatalf("cannot register target for schema_kind %q", typ.SchemaKind)
	}
}

func titlePrefix(t *config.Type) string {
	if t.TitlePrefix != "" {
		return t.TitlePrefix
	}
	return "Resource"
}

// --- RunAll --------------------------------------------------------------

func TestRunner_RunAll_EveryTypeVisited(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_instance")
	f.addTarget(t, "data_source", "test_instance")
	f.addTarget(t, "ephemeral", "test_secret")
	f.addTarget(t, "list_resource", "test_instances")
	f.addTarget(t, "action", "test_reboot")
	f.addTarget(t, "function", "test_format")

	f.runner.RunAll()

	want := []string{
		"test_format",
		"test_instance", // resource
		"test_instance", // data source (same name — both visited)
		"test_instances",
		"test_reboot",
		"test_secret",
	}
	if got := f.capture.resources(); !slices.Equal(got, want) {
		t.Errorf("RunAll dispatched to %v, want %v", got, want)
	}
}

// TestRunner_RunAll_NoneKindSkipped confirms types with schema_kind="none"
// (guides, index) are iterated only when directory scanning arrives in a
// future phase. For now they're silently skipped.
func TestRunner_RunAll_NoneKindSkipped(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	// Only populate a guide-like doc file; no schema side exists.
	guidePath := filepath.Join(f.root, "docs/guides/welcome.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(guidePath, []byte("---\npage_title: X\n---\n# Welcome\n"), 0o600)

	f.runner.RunAll()

	if got := f.capture.resources(); len(got) != 0 {
		t.Errorf("RunAll should skip none-kind types; got %v", got)
	}
}

// TestRunner_RunAll_MissingDocLogsAndContinues makes sure one bad target
// doesn't bring the run down. The capture rule only fires for the good one.
func TestRunner_RunAll_MissingDocLogsAndContinues(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_ok")
	// Add a second target to the schema without writing its doc file.
	f.ps.Resources["test_missing_doc"] = &schema.ResourceSchema{
		Name:   "test_missing_doc",
		Blocks: map[string]*schema.Block{},
	}

	f.runner.RunAll()

	got := f.capture.resources()
	if !slices.Equal(got, []string{"test_ok"}) {
		t.Errorf("capture = %v, want [test_ok]", got)
	}
}

// --- RunPrefix -----------------------------------------------------------

func TestRunner_RunPrefix_NameFilter(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_foo_one")
	f.addTarget(t, "resource", "test_foo_two")
	f.addTarget(t, "resource", "test_bar_three")

	f.runner.RunPrefix("test_foo_", "")

	want := []string{"test_foo_one", "test_foo_two"}
	if got := f.capture.resources(); !slices.Equal(got, want) {
		t.Errorf("RunPrefix test_foo_ = %v, want %v", got, want)
	}
}

// TestRunner_RunPrefix_TypeFilterOnly exercises the CI-gate use case: empty
// prefix but non-empty type scopes to "everything in this category". This is
// exactly what a provider migration needs to enable one type at a time.
func TestRunner_RunPrefix_TypeFilterOnly(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_instance")
	f.addTarget(t, "data_source", "test_instance")

	f.runner.RunPrefix("", "data_source")

	got := f.capture.resources()
	if !slices.Equal(got, []string{"test_instance"}) || len(got) != 1 {
		t.Errorf("RunPrefix ('', data_source) = %v, want just the data source", got)
	}
}

// TestRunner_RunPrefix_BothFilters combines both filter axes — this is the
// service-scoping case from the user's phase-3 motivation.
func TestRunner_RunPrefix_BothFilters(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_s3_bucket")
	f.addTarget(t, "resource", "test_ec2_instance")
	f.addTarget(t, "data_source", "test_s3_bucket")

	f.runner.RunPrefix("test_s3_", "resource")

	got := f.capture.resources()
	if !slices.Equal(got, []string{"test_s3_bucket"}) {
		t.Errorf("RunPrefix (test_s3_, resource) = %v, want [test_s3_bucket]", got)
	}
}

// --- RunOne --------------------------------------------------------------

func TestRunner_RunOne_Unique(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_instance")

	if _, err := f.runner.RunOne("test_instance", ""); err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if got := f.capture.resources(); !slices.Equal(got, []string{"test_instance"}) {
		t.Errorf("capture = %v, want [test_instance]", got)
	}
}

func TestRunner_RunOne_NotFound(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	_, err := f.runner.RunOne("test_nope", "")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), "test_nope") {
		t.Errorf("error should name the missing target; got: %v", err)
	}
}

// TestRunner_RunOne_AmbiguousAcrossTypes is the exact case the user flagged:
// aws_instance as both resource and data source. The error must name every
// matching type and mention --type so CI invocations can be self-correcting.
func TestRunner_RunOne_AmbiguousAcrossTypes(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_instance")
	f.addTarget(t, "data_source", "test_instance")

	_, err := f.runner.RunOne("test_instance", "")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	for _, want := range []string{"resource", "data_source", "--type"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q; got: %v", want, err)
		}
	}
}

// TestRunner_RunOne_TypeDisambiguates is the recovery path: the same
// situation as ambiguous, but with --type set, picks exactly one target.
func TestRunner_RunOne_TypeDisambiguates(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_instance")
	f.addTarget(t, "data_source", "test_instance")

	if _, err := f.runner.RunOne("test_instance", "data_source"); err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	// The capture rule only saw one invocation — the data source's.
	if got := f.capture.resources(); len(got) != 1 {
		t.Errorf("expected exactly one invocation, got %v", got)
	}
}

func TestRunner_RunOne_UnknownType(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_instance")

	_, err := f.runner.RunOne("test_instance", "widget")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "widget") {
		t.Errorf("error should name the unknown type; got: %v", err)
	}
}

// TestRunner_RunOne_MissingDocErrorsLoudly — contrary to RunAll, RunOne
// should fail loudly when its single target has no doc. The message must
// name every path tried so the user knows where to put the file.
func TestRunner_RunOne_MissingDocErrorsLoudly(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	// Register the target in the schema but don't create its doc.
	f.ps.Resources["test_no_doc"] = &schema.ResourceSchema{
		Name:   "test_no_doc",
		Blocks: map[string]*schema.Block{},
	}

	_, err := f.runner.RunOne("test_no_doc", "")
	if err == nil {
		t.Fatal("expected error for missing doc")
	}
	// Should mention one of the templates it tried.
	if !strings.Contains(err.Error(), "no_doc") {
		t.Errorf("error should name the target; got: %v", err)
	}
}

// --- Schema nil safety (regression) --------------------------------------

// TestRunner_FunctionTargetGetsNilSchema pins the contract that rules
// tolerate nil rs: the three rules that dereference rs (Completeness,
// ComputedAttribute, HeadingStyle) check for nil up front, and the Runner
// passes nil for non-block-schema kinds (function, none). If a future rule
// author forgets the guard, this test trips.
func TestRunner_FunctionTargetGetsNilSchema(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "function", "test_format")

	// Add every production rule — if any panics on nil rs, the test fails.
	f.runner.Rules = append(f.runner.Rules,
		&check.CompletenessRule{IgnoreDeprecated: true},
		&check.OrderingRule{},
		&check.DescriptionStyleRule{},
		&check.ComputedAttributeRule{},
		&check.HeadingStyleRule{Preferred: doc.HeadingTemplates{"`{Block}` Block"}},
		&check.TitleSectionRule{AllowedPrefixes: []string{"Function"}},
	)

	// Must not panic. Result content doesn't matter — just survival.
	_ = f.runner.RunAll()
}

// TestRunner_BlockTargetGetsSchemaPopulated confirms the happy path: for
// every block-kind target, rs is non-nil and the right record is passed.
func TestRunner_BlockTargetGetsSchemaPopulated(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t)
	f.addTarget(t, "resource", "test_instance")

	f.runner.RunAll()

	f.capture.mu.Lock()
	defer f.capture.mu.Unlock()
	if len(f.capture.seen) != 1 {
		t.Fatalf("expected one call, got %d", len(f.capture.seen))
	}
	if !f.capture.seen[0].HasSchema {
		t.Error("block-kind target should pass a non-nil ResourceSchema")
	}
	if !f.capture.seen[0].HasDoc {
		t.Error("target should pass a parsed Document")
	}
}

// --- Doc path resolution --------------------------------------------------

// TestRunner_ResolvesFirstExistingTemplate verifies the "try each
// website_paths entry in order" contract. The resource type's defaults list
// registry style first, legacy style second. Dropping the registry file in
// place and omitting the legacy one should still work; likewise the reverse.
func TestRunner_ResolvesFirstExistingTemplate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		relPath string
	}{
		"registry style wins when present": {"docs/resources/instance.md"},
		"legacy style picked up too":       {"website/docs/r/instance.html.markdown"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newRunnerFixture(t)

			// Write a doc to the chosen layout and register the schema entry.
			docPath := filepath.Join(f.root, tt.relPath)
			if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			body := "---\npage_title: x\n---\n# Resource: test_instance\nBody.\n"
			if err := os.WriteFile(docPath, []byte(body), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			f.ps.Resources["test_instance"] = &schema.ResourceSchema{
				Name:   "test_instance",
				Blocks: map[string]*schema.Block{},
			}

			f.runner.RunAll()

			got := f.capture.resources()
			if !slices.Equal(got, []string{"test_instance"}) {
				t.Errorf("capture = %v, want [test_instance] (layout %q)", got, tt.relPath)
			}
		})
	}
}
