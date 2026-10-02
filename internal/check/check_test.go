// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
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
