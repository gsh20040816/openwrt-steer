// SPDX-License-Identifier: GPL-3.0-or-later
package capability

import "testing"

func TestParseSupportedBuild(t *testing.T) {
	report := Parse("sing-box version 1.14.0-rc.1\n\nEnvironment: go1.26 linux/amd64\nTags: with_quic,with_utls\n", []string{"tun", "dns_quic", "with_utls"})
	if !report.OK || report.Version != "1.14.0-rc.1" {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestParseRejectsVersionAndMissingTag(t *testing.T) {
	report := Parse("sing-box version 1.14.0-beta.1\nTags: with_quic\n", []string{"with_utls"})
	if report.OK || len(report.Errors) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for _, version := range []string{"1.13.99", "1.14.0-alpha.50", "1.14.0-beta.1", "1.15.0", "2.0.0"} {
		if supportedVersion(version) {
			t.Fatalf("unsupported version accepted: %s", version)
		}
	}
	for _, version := range []string{"1.14.0-beta.2", "1.14.0-rc.1", "1.14.0", "1.14.9"} {
		if !supportedVersion(version) {
			t.Fatalf("supported version rejected: %s", version)
		}
	}
}
