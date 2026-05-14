// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// Runner orchestrates checks across every target the provider exposes.
//
// Targets are discovered by iterating the configured types (config.Config.Types)
// and asking the loaded schema for the names of each type's kind. For each
// target, Runner resolves the doc file from the type's website_paths
// templates, reads and parses it once, and invokes every Rule (schema + AST)
// and every FileRule (raw bytes) against it.
//
// The Runner no longer hard-codes "resource" vs "data source" — everything it
// does is driven by config.Type definitions. Adding a new Terraform category
// means adding a type block (and, if block-based, registering a schema
// accessor); the Runner requires no changes.
type Runner struct {
	Schema                    *schema.ProviderSchema
	Config                    *config.Config
	Rules                     []Rule
	FileRules                 []FileRule
	Logger                    *slog.Logger
	HeadingTemplates          doc.HeadingTemplates
	PreferredHeadingTemplates doc.HeadingTemplates
}

// RunAll runs every configured rule against every target of every type that
// swissshepherd can enumerate from the provider schema. Per-file load and
// parse errors are logged and skipped so a single bad doc does not bring
// down a full-provider run.
func (r *Runner) RunAll() []Result {
	return r.runFiltered("", "")
}

// RunPrefix runs every configured rule against targets whose names begin
// with prefix. If kind is non-empty, targets are restricted to the type
// with that name (typically "resource", "data_source", …). Empty kind plus
// empty prefix is equivalent to RunAll.
func (r *Runner) RunPrefix(prefix, kind string) []Result {
	return r.runFiltered(prefix, kind)
}

// RunOne runs every configured rule against a single named target. If kind
// is empty, Runner searches every configured type; an ambiguous name
// (present in multiple types) returns an error naming the candidates so the
// caller can re-invoke with an explicit --type. When kind is non-empty,
// only that type is consulted.
//
// RunOne surfaces file-read and parse errors rather than logging-and-skipping
// because it's the "I asked for this one thing" mode and silence would be
// misleading.
func (r *Runner) RunOne(name, kind string) ([]Result, error) {
	typ, err := r.resolveOne(name, kind)
	if err != nil {
		return nil, err
	}
	return r.runTarget(typ, name, false)
}

// runFiltered is the shared iteration path behind RunAll and RunPrefix. It
// walks every configured type whose kind has enumerable targets and filters
// by name prefix and/or type name. Errors are logged and dropped.
func (r *Runner) runFiltered(prefix, kind string) []Result {
	var results []Result
	for i := range r.Config.Types {
		t := &r.Config.Types[i]
		if t.SchemaKind == schema.KindNone {
			continue // content-only categories (guides, index) have no schema to enumerate
		}
		if kind != "" && t.Name != kind {
			continue
		}
		for _, name := range r.Schema.TargetNames(t.SchemaKind) {
			if prefix != "" && !strings.HasPrefix(name, prefix) {
				continue
			}
			res, _ := r.runTarget(t, name, true)
			results = append(results, res...)
		}
	}
	return results
}

// resolveOne maps a user-provided (name, kind) pair to the single type that
// should be invoked. kind=="" triggers ambiguity detection across every
// enumerable type; a non-empty kind short-circuits to the named type.
func (r *Runner) resolveOne(name, kind string) (*config.Type, error) {
	if kind != "" {
		t := r.Config.GetType(kind)
		if t == nil {
			return nil, fmt.Errorf("unknown type %q (configured: %v)", kind, r.Config.TypeNames())
		}
		if t.SchemaKind == schema.KindNone {
			return nil, fmt.Errorf("type %q has no schema and cannot resolve individual targets", kind)
		}
		if slices.Contains(r.Schema.TargetNames(t.SchemaKind), name) {
			return t, nil
		}
		return nil, fmt.Errorf("%s %q not found in schema", kind, name)
	}

	var matches []*config.Type
	for i := range r.Config.Types {
		t := &r.Config.Types[i]
		if t.SchemaKind == schema.KindNone {
			continue
		}
		if slices.Contains(r.Schema.TargetNames(t.SchemaKind), name) {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("%q not found in any type's schema", name)
	case 1:
		return matches[0], nil
	default:
		typeNames := make([]string, len(matches))
		for i, t := range matches {
			typeNames[i] = t.Name
		}
		return nil, fmt.Errorf("%q matches multiple types (%s); re-run with --type to disambiguate",
			name, strings.Join(typeNames, ", "))
	}
}

// runTarget reads and parses the doc for a single (type, name) pair and
// runs every Rule and FileRule whose CheckConfig.AppliesTo admits this
// target. When logOnError is true, file-read and parse failures produce a
// warning log and empty results (RunAll / RunPrefix semantics); when false,
// the error is returned to the caller (RunOne).
//
// Per-rule scoping is checked before any doc work. If no rule applies to a
// target, the doc is never read — important for a large provider where
// most rules have a narrow prefix/type allowlist during a migration.
func (r *Runner) runTarget(t *config.Type, name string, logOnError bool) ([]Result, error) {
	applicableRules := r.applicableRules(name, t.Name)
	applicableFileRules := r.applicableFileRules(name, t.Name)
	if len(applicableRules) == 0 && len(applicableFileRules) == 0 {
		return nil, nil
	}

	docPath, docName, err := r.resolveDocPath(t, name)
	if err != nil {
		if logOnError {
			if !slices.Contains(r.Config.IgnoreFileMissing, name) {
				r.Logger.Warn("doc file not found", "type", t.Name, "name", name, "error", err)
			}
			return nil, nil
		}
		return nil, err
	}

	content, err := os.ReadFile(docPath)
	if err != nil {
		if logOnError {
			r.Logger.Warn("cannot read doc", "type", t.Name, "name", name, "path", docPath, "error", err)
			return nil, nil
		}
		return nil, fmt.Errorf("reading doc for %s: %w", name, err)
	}

	d, err := doc.ParseWithTemplates(content, docPath, r.HeadingTemplates)
	if err != nil {
		if logOnError {
			r.Logger.Warn("cannot parse doc", "type", t.Name, "name", name, "path", docPath, "error", err)
			return nil, nil
		}
		return nil, fmt.Errorf("parsing doc for %s: %w", name, err)
	}

	rs := r.Schema.ResourceSchemaFor(t.SchemaKind, name)

	var results []Result
	if !r.Config.ShouldIgnoreContents(name, t.Name) {
		ctx := CheckContext{Resource: name, DocName: docName, Type: t, Schema: rs, FunctionSchema: r.Schema.Functions[name], IdentitySchema: r.Schema.IdentitySchemas[name], Doc: d}
		for _, rule := range applicableRules {
			results = append(results, rule.Check(ctx)...)
		}
	}
	fctx := FileCheckContext{Resource: name, Type: t, Path: docPath, Content: content}
	for _, rule := range applicableFileRules {
		results = append(results, rule.CheckFile(fctx)...)
	}
	for i := range results {
		results[i].Path = docPath
	}
	return results, nil
}

// applicableRules filters r.Rules down to the Rules whose CheckConfig admits
// the given target. Rule order is preserved.
func (r *Runner) applicableRules(name, typeName string) []Rule {
	out := make([]Rule, 0, len(r.Rules))
	for _, rule := range r.Rules {
		if r.Config.GetCheck(rule.Name()).AppliesTo(name, typeName) {
			out = append(out, rule)
		}
	}
	return out
}

// applicableFileRules is the FileRule equivalent of applicableRules.
func (r *Runner) applicableFileRules(name, typeName string) []FileRule {
	out := make([]FileRule, 0, len(r.FileRules))
	for _, rule := range r.FileRules {
		if r.Config.GetCheck(rule.Name()).AppliesTo(name, typeName) {
			out = append(out, rule)
		}
	}
	return out
}

// resolveDocPath tries every website_paths template for the given type and
// returns the first candidate that exists on disk. When Config.ProviderDir
// is set, templates resolve relative to that directory; otherwise they
// resolve relative to the current working directory. Returns an error
// identifying every path tried when none exist.
func (r *Runner) resolveDocPath(t *config.Type, name string) (string, string, error) {
	docName := name
	if alias, ok := r.Config.FileAliases[t.Name+"/"+name]; ok {
		docName = alias
	} else if alias, ok := r.Config.FileAliases[name]; ok {
		docName = alias
	}
	candidates := t.ResolveDocPath(docName, r.Config.ProviderName())
	anchor := r.Config.ProviderDir
	tried := make([]string, 0, len(candidates))
	for _, c := range candidates {
		full := c
		if anchor != "" && !filepath.IsAbs(c) {
			full = filepath.Join(anchor, c)
		}
		tried = append(tried, full)
		if _, err := os.Stat(full); err == nil {
			return full, docName, nil
		}
	}
	return "", "", fmt.Errorf("no doc file found for %s %q (tried: %s)",
		t.Name, name, strings.Join(tried, ", "))
}
