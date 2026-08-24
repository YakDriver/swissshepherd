// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
)

// The multi-entry weak-config gloss map, mirroring
// terraform-provider-aws/.ci/swissshepherd-weak.hcl. Used to exercise the
// combined pre-filter gate with realistic entry counts.
var weakGlosses = map[string]string{
	"Amazon Machine Image": "AMI", "Amazon Resource Name": "ARN", "Amazon Resource Names": "ARNs",
	"Application Programming Interface": "API", "Central Processing Unit": "CPU",
	"Command-Line Interface": "CLI", "Domain Name System": "DNS", "Elastic Compute Cloud": "EC2",
	"Extensible Markup Language": "XML", "Graphics Processing Unit": "GPU",
	"HyperText Markup Language": "HTML", "Hypertext Transfer Protocol": "HTTP",
	"Internet Protocol": "IP", "JavaScript Object Notation": "JSON", "Key Management Service": "KMS",
	"Relational Database Service": "RDS", "Simple Storage Service": "S3", "Software Development Kit": "SDK",
	"Structured Query Language": "SQL", "Transmission Control Protocol": "TCP",
	"Transport Layer Security": "TLS", "Unicode Transformation Format": "UTF",
	"Uniform Resource Identifier": "URI", "Uniform Resource Locator": "URL",
	"Universal Serial Bus": "USB", "Virtual Private Cloud": "VPC", "Virtual Private Network": "VPN",
	"YAML Ain't Markup Language": "YAML",
}

func gloss(glosses map[string]string, content string) []check.Result {
	return check.NewGlossRule(glosses, false, check.SeverityWarning).CheckFile(check.FileCheckContext{
		Resource: "aws_thing", Path: "p", Content: []byte(content),
	})
}

// TestGlossRule_GateEquivalence is a regression guard for the combined
// pre-filter optimization: gating (fast reject) must not change which lines
// produce findings. Lines with no phrase are skipped; lines with a phrase in
// prose are reported; lines where the only phrase sits in code/URL are gated
// in but masked back out to zero findings. Verified against the full weak
// gloss map so alternation ordering cannot mask an entry.
func TestGlossRule_GateEquivalence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"no phrase at all", "This line mentions an ARN and a VPC, both fine.", 0},
		{"prose phrase", "The Amazon Resource Name (ARN) of the thing.", 1},
		{"plural distinct entry", "A list of Amazon Resource Names here.", 2}, // matches both singular and plural entries
		{"phrase only in code", "Set `Domain Name System` in the field.", 0},
		{"phrase only in url", "See [x](https://example.com/Virtual-Private-Cloud).", 0},
		{"mixed: code masked, prose live", "`Simple Storage Service` but Transport Layer Security too.", 1},
		{"multiple distinct phrases one line", "The Internet Protocol and the Key Management Service.", 2},
		{"near-miss substring", "The Resource Name and the Storage Service (no vendor prefix).", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := gloss(weakGlosses, tc.content); len(got) != tc.want {
				t.Errorf("content %q: got %d findings, want %d: %+v", tc.content, len(got), tc.want, got)
			}
		})
	}
}
