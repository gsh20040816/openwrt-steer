// SPDX-License-Identifier: GPL-3.0-or-later
package capability

import "testing"

func TestParseSupportedBuild(t *testing.T) {
	report := Parse("sing-box version 1.13.19\n\nEnvironment: go1.26 linux/amd64\nTags: with_quic,with_utls\n", []string{"tun", "dns_quic", "with_utls"})
	if !report.OK || report.Version != "1.13.19" {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestParseRejectsVersionAndMissingTag(t *testing.T) {
	report := Parse("sing-box version 1.13.17\nTags: with_quic\n", []string{"with_utls"})
	if report.OK || len(report.Errors) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for _, version := range []string{"1.12.99", "1.14.0", "2.0.0"} {
		if supportedVersion(version) {
			t.Fatalf("unsupported version accepted: %s", version)
		}
	}
}

func TestParseRejectsUnknownCapability(t *testing.T) {
	report := Parse("sing-box version 1.13.19\nTags: with_quic\n", []string{"made_up"})
	if report.OK || len(report.Errors) != 1 || report.Errors[0] != "unknown runtime capability made_up" {
		t.Fatalf("unknown capability was not rejected: %#v", report)
	}
}
