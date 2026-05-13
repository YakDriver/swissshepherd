// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// Severity indicates the severity of a check result.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	default:
		return "error"
	}
}

// Result represents a single finding from a check.
type Result struct {
	Rule     string   `json:"rule"`
	Resource string   `json:"resource"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Block    string   `json:"block,omitempty"`
}

// CheckContext carries all inputs a Rule needs to produce findings.
type CheckContext struct {
	Resource string
	Type     *config.Type
	Schema   *schema.ResourceSchema
	Doc      *doc.Document
}

// FileCheckContext carries all inputs a FileRule needs to produce findings.
type FileCheckContext struct {
	Resource string
	Type     *config.Type
	Path     string
	Content  []byte
}

// Rule is the interface all per-resource checks implement. A Rule compares a
// resource's schema against its parsed markdown document. For checks that
// operate on raw file content (YAML frontmatter, file size, line-level
// scanning, etc.) implement FileRule instead.
type Rule interface {
	Name() string
	Check(ctx CheckContext) []Result
}

// FileRule is the interface for checks that operate on the raw bytes of a
// resource documentation file, independent of the markdown AST. Runner reads
// each file exactly once per invocation and passes the content to every
// FileRule alongside the parsed Document that Rules consume.
type FileRule interface {
	Name() string
	CheckFile(ctx FileCheckContext) []Result
}
