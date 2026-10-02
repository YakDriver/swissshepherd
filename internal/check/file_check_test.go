// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
)

func TestFileCheckRule_SizeLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	small := filepath.Join(dir, "small.md")
	os.WriteFile(small, make([]byte, 100), 0o644)
	big := filepath.Join(dir, "big.md")
	os.WriteFile(big, make([]byte, 600000), 0o644)

	rule := &check.FileCheckRule{MaxSize: 500000}

	results := rule.CheckFile(check.FileCheckContext{Resource: "aws_small", Path: small, Content: make([]byte, 100)})
	if len(results) != 0 {
		t.Errorf("expected no findings for small file, got %d", len(results))
	}

	results = rule.CheckFile(check.FileCheckContext{Resource: "aws_big", Path: big, Content: make([]byte, 600000)})
	if len(results) != 1 {
		t.Fatalf("expected 1 finding for big file, got %d", len(results))
	}
	if results[0].Severity != check.SeverityError {
		t.Errorf("expected error severity, got %v", results[0].Severity)
	}
}

func TestFileCheckRule_Extension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	good := filepath.Join(dir, "thing.md")
	os.WriteFile(good, []byte("# hi"), 0o644)
	bad := filepath.Join(dir, "thing.txt")
	os.WriteFile(bad, []byte("# hi"), 0o644)

	rule := &check.FileCheckRule{AllowExtensions: []string{".md", ".html.markdown"}}

	results := rule.CheckFile(check.FileCheckContext{Resource: "aws_thing", Path: good, Content: []byte("# hi")})
	if len(results) != 0 {
		t.Errorf("expected no findings for .md file, got %d", len(results))
	}

	results = rule.CheckFile(check.FileCheckContext{Resource: "aws_thing", Path: bad, Content: []byte("# hi")})
	if len(results) != 1 {
		t.Fatalf("expected 1 finding for .txt file, got %d", len(results))
	}
}

func TestFileCheckRule_DefaultSize(t *testing.T) {
	t.Parallel()

	rule := &check.FileCheckRule{}
	if rule.Name() != "file_check" {
		t.Errorf("unexpected name: %s", rule.Name())
	}
}
