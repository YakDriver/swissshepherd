// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

func TestFileMatchRule_RequireSchema(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mkDirs(t, dir, "docs/resources")
	touch(t, filepath.Join(dir, "docs/resources/instance.md"))
	touch(t, filepath.Join(dir, "docs/resources/orphan.md"))

	cfg := &config.Config{
		ProviderSource: "registry.terraform.io/hashicorp/aws",
		ProviderDir:    dir,
		Types: []config.Type{{
			Name:         "resource",
			SchemaKind:   "resource",
			WebsitePaths: []string{"docs/resources/{name}.md"},
		}},
	}
	ps := &schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{"aws_instance": {}},
	}

	rule := &check.FileMatchRule{}
	results := rule.Check(cfg, ps)

	found := false
	for _, r := range results {
		if strings.Contains(r.Message, "orphan") {
			found = true
		}
		if strings.Contains(r.Message, "instance") && strings.Contains(r.Message, "no matching") {
			t.Errorf("should not flag aws_instance: %s", r.Message)
		}
	}
	if !found {
		t.Error("expected finding for orphan doc file")
	}
}

func TestFileMatchRule_IgnoreExtra(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mkDirs(t, dir, "docs/resources")
	touch(t, filepath.Join(dir, "docs/resources/orphan.md"))

	cfg := &config.Config{
		ProviderSource: "registry.terraform.io/hashicorp/aws",
		ProviderDir:    dir,
		Types: []config.Type{{
			Name:         "resource",
			SchemaKind:   "resource",
			WebsitePaths: []string{"docs/resources/{name}.md"},
		}},
	}
	ps := &schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{},
	}

	rule := &check.FileMatchRule{IgnoreExtra: []string{"aws_orphan"}}
	results := rule.Check(cfg, ps)

	for _, r := range results {
		if strings.Contains(r.Message, "orphan") {
			t.Errorf("should not flag ignored orphan: %s", r.Message)
		}
	}
}

func TestFileMatchRule_RequireDoc(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mkDirs(t, dir, "docs/resources")
	// Only create doc for instance, not for bucket
	touch(t, filepath.Join(dir, "docs/resources/instance.md"))

	cfg := &config.Config{
		ProviderSource: "registry.terraform.io/hashicorp/aws",
		ProviderDir:    dir,
		Types: []config.Type{{
			Name:         "resource",
			SchemaKind:   "resource",
			WebsitePaths: []string{"docs/resources/{name}.md"},
		}},
	}
	ps := &schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{
			"aws_instance": {},
			"aws_bucket":   {},
		},
	}

	rule := &check.FileMatchRule{}
	results := rule.Check(cfg, ps)

	found := false
	for _, r := range results {
		if strings.Contains(r.Message, "aws_bucket") && strings.Contains(r.Message, "no documentation") {
			found = true
			if r.Severity != check.SeverityError {
				t.Errorf("missing doc file: Severity = %v, want SeverityError", r.Severity)
			}
		}
	}
	if !found {
		t.Error("expected finding for missing doc file")
	}
}

// TestFileMatchRule_RequireDoc_IgnoreMissing confirms a schema resource
// listed in ignore_missing produces no missing-doc finding, even though
// the finding is now an error.
func TestFileMatchRule_RequireDoc_IgnoreMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mkDirs(t, dir, "docs/resources")
	touch(t, filepath.Join(dir, "docs/resources/instance.md"))

	cfg := &config.Config{
		ProviderSource: "registry.terraform.io/hashicorp/aws",
		ProviderDir:    dir,
		Types: []config.Type{{
			Name:         "resource",
			SchemaKind:   "resource",
			WebsitePaths: []string{"docs/resources/{name}.md"},
		}},
	}
	ps := &schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{
			"aws_instance": {},
			"aws_bucket":   {},
		},
	}

	rule := &check.FileMatchRule{IgnoreMissing: []string{"aws_bucket"}}
	results := rule.Check(cfg, ps)

	for _, r := range results {
		if strings.Contains(r.Message, "aws_bucket") {
			t.Errorf("ignore_missing resource should not be flagged; got: %s", r.Message)
		}
	}
}

func TestFileMatchRule_MixedLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mkDirs(t, dir, "docs/resources", "website/docs/r")
	touch(t, filepath.Join(dir, "docs/resources/thing.md"))
	touch(t, filepath.Join(dir, "website/docs/r/other.html.markdown"))

	cfg := &config.Config{
		ProviderSource: "registry.terraform.io/hashicorp/aws",
		ProviderDir:    dir,
		Types: []config.Type{{
			Name:       "resource",
			SchemaKind: "resource",
			WebsitePaths: []string{
				"docs/resources/{name}.md",
				"website/docs/r/{name}.html.markdown",
			},
		}},
	}
	ps := &schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{
			"aws_thing": {},
			"aws_other": {},
		},
	}

	rule := &check.FileMatchRule{}
	results := rule.Check(cfg, ps)

	found := false
	for _, r := range results {
		if strings.Contains(r.Message, "mixed") {
			found = true
		}
	}
	if !found {
		t.Error("expected mixed layout finding")
	}
}

func mkDirs(t *testing.T, base string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(base, d), 0o755)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	os.WriteFile(path, []byte("# placeholder\n"), 0o644)
}
