// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
)

func TestParseSeverity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		def  check.Severity
		want check.Severity
	}{
		{"error", check.SeverityWarning, check.SeverityError},
		{"warning", check.SeverityError, check.SeverityWarning},
		{"warn", check.SeverityError, check.SeverityWarning},
		{"  ERROR  ", check.SeverityWarning, check.SeverityError},
		{"", check.SeverityWarning, check.SeverityWarning},      // empty → default
		{"bogus", check.SeverityWarning, check.SeverityWarning}, // invalid → default
		{"", check.SeverityError, check.SeverityError},
	}
	for _, tc := range cases {
		if got := check.ParseSeverity(tc.in, tc.def); got != tc.want {
			t.Errorf("ParseSeverity(%q, %v) = %v, want %v", tc.in, tc.def, got, tc.want)
		}
	}
}

// TestOverrideSeverity guards that a check block's severity replaces the
// severity of every finding from that check, in both directions and on
// checks that mix the two, and leaves other checks' findings alone (#119).
func TestOverrideSeverity(t *testing.T) {
	t.Parallel()

	results := func() []check.Result {
		return []check.Result{
			{Rule: "schema_docs", Severity: check.SeverityError, Message: "e"},
			{Rule: "schema_docs", Severity: check.SeverityWarning, Message: "w"},
			{Rule: "title_section", Severity: check.SeverityError, Message: "t"},
			{Rule: "file_match", Severity: check.SeverityError, Message: "f"},
		}
	}
	tests := []struct {
		name   string
		checks []config.CheckConfig
		want   []check.Severity
	}{
		{
			name: "no severity set",
			want: []check.Severity{check.SeverityError, check.SeverityWarning, check.SeverityError, check.SeverityError},
		},
		{
			name:   "warning demotes every finding of a mixed check",
			checks: []config.CheckConfig{{Name: "schema_docs", Severity: "warning"}},
			want:   []check.Severity{check.SeverityWarning, check.SeverityWarning, check.SeverityError, check.SeverityError},
		},
		{
			name:   "error promotes every finding of a mixed check",
			checks: []config.CheckConfig{{Name: "schema_docs", Severity: "error"}},
			want:   []check.Severity{check.SeverityError, check.SeverityError, check.SeverityError, check.SeverityError},
		},
		{
			name: "per check",
			checks: []config.CheckConfig{
				{Name: "title_section", Severity: "warning"},
				{Name: "file_match", Severity: "Warn"},
			},
			want: []check.Severity{check.SeverityError, check.SeverityWarning, check.SeverityWarning, check.SeverityWarning},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := check.OverrideSeverity(&config.Config{Checks: tt.checks}, results())
			for i, r := range got {
				if r.Severity != tt.want[i] {
					t.Errorf("result %d (%s %q): severity = %v, want %v", i, r.Rule, r.Message, r.Severity, tt.want[i])
				}
			}
		})
	}
}
