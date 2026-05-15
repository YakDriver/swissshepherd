// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// FileMatchRule validates file↔schema alignment:
//   - require_doc: every schema resource must have a doc file
//   - require_schema: every doc file must have a matching schema resource
//   - mixed_layout: can't mix website/docs/ and docs/ layouts
type FileMatchRule struct {
	RequireDoc    *bool // default true
	RequireSchema *bool // default true
	MixedLayout   *bool // default true

	IgnoreMissing []string // resources that don't need a doc file
	IgnoreExtra   []string // doc files that don't need a schema resource
}

func (r *FileMatchRule) Name() string { return "file_match" }

func (r *FileMatchRule) enabled(b *bool) bool {
	return b == nil || *b
}

// Check runs all file_match sub-checks. Called once per invocation by the Runner.
func (r *FileMatchRule) Check(cfg *config.Config, ps *schema.ProviderSchema) []Result {
	var results []Result

	if r.enabled(r.MixedLayout) {
		results = append(results, r.checkMixedLayout(cfg)...)
	}
	if r.enabled(r.RequireDoc) {
		results = append(results, r.checkRequireDoc(cfg, ps)...)
	}
	if r.enabled(r.RequireSchema) {
		results = append(results, r.checkRequireSchema(cfg, ps)...)
	}

	return results
}

// checkMixedLayout detects when both legacy (website/docs/) and registry (docs/)
// layouts have files present. Valid directories are derived from the type system.
func (r *FileMatchRule) checkMixedLayout(cfg *config.Config) []Result {
	var hasLegacy, hasRegistry bool
	for i := range cfg.Types {
		t := &cfg.Types[i]
		for _, tmpl := range t.WebsitePaths {
			pattern := strings.ReplaceAll(tmpl, "{name}", "*")
			anchor := cfg.ProviderDir
			if anchor != "" && !filepath.IsAbs(pattern) {
				pattern = filepath.Join(anchor, pattern)
			}
			matches, _ := filepath.Glob(pattern)
			if len(matches) == 0 {
				continue
			}
			rel := tmpl
			if strings.HasPrefix(rel, "website/docs/") {
				hasLegacy = true
			} else if strings.HasPrefix(rel, "docs/") {
				hasRegistry = true
			}
		}
	}
	if hasLegacy && hasRegistry {
		return []Result{{
			Rule: r.Name(), Severity: SeverityError,
			Message: "mixed documentation layouts found (both legacy website/docs/ and registry docs/); use only one",
		}}
	}
	return nil
}

// checkRequireDoc flags schema resources that have no doc file.
func (r *FileMatchRule) checkRequireDoc(cfg *config.Config, ps *schema.ProviderSchema) []Result {
	var results []Result
	for i := range cfg.Types {
		t := &cfg.Types[i]
		if t.SchemaKind == "none" {
			continue
		}
		for _, name := range ps.TargetNames(t.SchemaKind) {
			if slices.Contains(r.IgnoreMissing, name) {
				continue
			}
			if r.docExists(cfg, t, name) {
				continue
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: name, Severity: SeverityWarning,
				Message: fmt.Sprintf("no documentation file found for %s %q", t.Name, name),
			})
		}
	}
	return results
}

// checkRequireSchema flags doc files that have no matching schema resource.
func (r *FileMatchRule) checkRequireSchema(cfg *config.Config, ps *schema.ProviderSchema) []Result {
	var results []Result
	providerName := cfg.ProviderName()

	for i := range cfg.Types {
		t := &cfg.Types[i]
		if t.SchemaKind == "none" {
			continue
		}
		schemaNames := ps.TargetNames(t.SchemaKind)
		if len(schemaNames) == 0 {
			continue
		}
		nameSet := make(map[string]bool, len(schemaNames))
		for _, n := range schemaNames {
			nameSet[n] = true
		}
		// Include aliases.
		for k, v := range cfg.FileAliases {
			key := k
			if strings.Contains(key, "/") {
				parts := strings.SplitN(key, "/", 2)
				if parts[0] != t.Name {
					continue
				}
				key = parts[1]
			}
			if nameSet[key] {
				nameSet[v] = true
			}
		}

		files := discoverDocFiles(cfg, t)
		for _, file := range files {
			prefixed := fileToResourceName(file, providerName)
			bare := fileToResourceName(file, "")
			if nameSet[prefixed] || nameSet[bare] {
				continue
			}
			if slices.Contains(r.IgnoreExtra, prefixed) || slices.Contains(r.IgnoreExtra, bare) {
				continue
			}
			displayName := prefixed
			if providerName != "" && len(schemaNames) > 0 && !strings.HasPrefix(schemaNames[0], providerName+"_") {
				displayName = bare
			}
			results = append(results, Result{
				Rule: r.Name(), Path: file, Severity: SeverityError,
				Message: fmt.Sprintf("documentation file has no matching %s in schema: %s", t.Name, displayName),
			})
		}
	}
	return results
}

// docExists checks if a doc file exists for the given target.
func (r *FileMatchRule) docExists(cfg *config.Config, t *config.Type, name string) bool {
	docName := name
	if alias, ok := cfg.FileAliases[t.Name+"/"+name]; ok {
		docName = alias
	} else if alias, ok := cfg.FileAliases[name]; ok {
		docName = alias
	}
	candidates := t.ResolveDocPath(docName, cfg.ProviderName())
	for _, c := range candidates {
		full := c
		if cfg.ProviderDir != "" && !filepath.IsAbs(c) {
			full = filepath.Join(cfg.ProviderDir, c)
		}
		if fileExists(full) {
			return true
		}
	}
	return false
}

// discoverDocFiles finds all doc files for a type by globbing its website_paths.
func discoverDocFiles(cfg *config.Config, t *config.Type) []string {
	var files []string
	for _, tmpl := range t.WebsitePaths {
		pattern := strings.ReplaceAll(tmpl, "{name}", "*")
		if cfg.ProviderDir != "" && !filepath.IsAbs(pattern) {
			pattern = filepath.Join(cfg.ProviderDir, pattern)
		}
		matches, _ := filepath.Glob(pattern)
		files = append(files, matches...)
	}
	return files
}

// fileExists reports whether a path exists on disk.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// fileToResourceName converts a doc file path to the expected resource name.
func fileToResourceName(path, providerPrefix string) string {
	base := filepath.Base(path)
	if dot := strings.IndexByte(base, '.'); dot > 0 {
		base = base[:dot]
	}
	if providerPrefix == "" {
		return base
	}
	return providerPrefix + "_" + base
}
