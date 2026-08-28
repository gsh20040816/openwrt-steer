// SPDX-License-Identifier: GPL-3.0-or-later

package intent

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	want := Intent{Main: Main{ID: "main", SchemaVersion: SchemaVersion, Enabled: true}, Bootstrap: Bootstrap{ID: "bootstrap"}}
	var encoded bytes.Buffer
	if err := EncodeJSON(&encoded, want); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeJSON(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Main != want.Main || got.Bootstrap != want.Bootstrap {
		t.Fatalf("round trip changed intent: %#v", got)
	}
}

func TestJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	for _, raw := range []string{
		`{"main":{"schema_version":7,"unknown":true}}`,
		`{"main":{"schema_version":7}} {}`,
	} {
		if _, err := DecodeJSON(strings.NewReader(raw)); err == nil {
			t.Fatalf("invalid canonical JSON was accepted: %s", raw)
		}
	}
}

func TestJSONDNSProfileOptionsRemainSubjectToSemanticValidation(t *testing.T) {
	value := validIntent()
	value.DNSProfiles[0].Protocol = "udp"
	value.DNSProfiles[0].TLSServerName = "stale.example"
	value.DNSProfiles[0].Path = "/dns-query"
	value.DNSProfiles[0].Insecure = true
	var encoded bytes.Buffer
	if err := EncodeJSON(&encoded, value); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeJSON(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	validation := Validate(decoded)
	for _, option := range []string{"tls_server_name", "path", "insecure"} {
		if !hasIssueForOption(validation, "UNSUPPORTED_DNS_OPTION", option) {
			t.Fatalf("raw JSON retained unsupported %q without a stable error: %#v", option, validation.Errors)
		}
	}
}

func TestJSONRoundTripPreservesTLSALPN(t *testing.T) {
	value := validIntent()
	value.Nodes[0].ALPN = []string{"h3", "h2"}
	var encoded bytes.Buffer
	if err := EncodeJSON(&encoded, value); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeJSON(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Nodes[0].ALPN, value.Nodes[0].ALPN) {
		t.Fatalf("TLS ALPN changed during JSON round trip: %#v", decoded.Nodes[0].ALPN)
	}
}
