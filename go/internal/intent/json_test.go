// SPDX-License-Identifier: GPL-3.0-or-later

package intent

import (
	"bytes"
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
