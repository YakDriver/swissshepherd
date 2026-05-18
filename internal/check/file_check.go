// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// FileCheckRule validates file-level properties: size limit, extension, and link style.
type FileCheckRule struct {
	MaxSize                 int64    // bytes; 0 means use default (500000)
	AllowExtensions         []string // e.g. [".md", ".html.markdown"]; empty means no check
	AllowRegistryExtensions []string // stricter set for registry layout; empty means no check
	InlineLinks             *bool    // when true, flag reference-style links
}

func (r *FileCheckRule) Name() string { return "file_check" }

func (r *FileCheckRule) maxSize() int64 {
	if r.MaxSize > 0 {
		return r.MaxSize
	}
	return 500000 // 500KB Terraform Registry limit
}

func (r *FileCheckRule) CheckFile(ctx FileCheckContext) []Result {
	var results []Result

	// Size check.
	fi, err := os.Stat(ctx.Path)
	if err == nil && fi.Size() >= r.maxSize() {
		results = append(results, Result{
			Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
			Message: fmt.Sprintf("file size %d exceeds maximum %d bytes", fi.Size(), r.maxSize()),
		})
	}

	// Extension check.
	exts := r.AllowExtensions
	if len(r.AllowRegistryExtensions) > 0 && strings.HasPrefix(ctx.Path, "docs/") {
		exts = r.AllowRegistryExtensions
	}
	if len(exts) > 0 {
		valid := false
		for _, ext := range exts {
			if strings.HasSuffix(ctx.Path, ext) {
				valid = true
				break
			}
		}
		if !valid {
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
				Message: fmt.Sprintf("file extension not valid, expected one of: %v", exts),
			})
		}
	}

	// Link style check.
	if r.InlineLinks != nil && *r.InlineLinks {
		results = append(results, r.checkLinkStyle(ctx)...)
	}

	return results
}

// checkLinkStyle flags reference-style link definitions (lines matching
// `[ref]: url`). These should be converted to inline `[text](url)` style.
func (r *FileCheckRule) checkLinkStyle(ctx FileCheckContext) []Result {
	var results []Result
	for i, line := range bytes.Split(ctx.Content, []byte("\n")) {
		if isRefLinkDef(line) {
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Line:    i + 1,
				Message: "reference-style link definition; use inline [text](url) style instead",
			})
		}
	}
	return results
}

// isRefLinkDef detects lines like `[label]: url` (reference link definitions).
func isRefLinkDef(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return false
	}
	closeBracket := bytes.IndexByte(trimmed, ']')
	if closeBracket < 2 {
		return false
	}
	// Must be followed by ": " (not "](..." which is an inline link)
	rest := trimmed[closeBracket+1:]
	return len(rest) >= 2 && rest[0] == ':' && rest[1] == ' '
}
