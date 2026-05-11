// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
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
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Block    string   `json:"block,omitempty"`
}

// Rule is the interface all checks implement.
type Rule interface {
	Name() string
	Check(resource string, rs *schema.ResourceSchema, d *doc.Document) []Result
}
