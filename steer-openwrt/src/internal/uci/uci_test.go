// SPDX-License-Identifier: GPL-3.0-or-later
package uci

import (
	"strings"
	"testing"
)

func TestParseStrictDocument(t *testing.T) {
	doc, err := Parse(strings.NewReader(`
# comment
config steer 'main'
	option schema_version '4'
	list managed_zone 'lan'
	list managed_zone "guest"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 1 || doc.Sections[0].ID != "main" {
		t.Fatalf("unexpected document: %#v", doc)
	}
	if got := doc.Sections[0].Lists["managed_zone"]; len(got) != 2 || got[1] != "guest" {
		t.Fatalf("unexpected list: %#v", got)
	}
}

func TestParseRejectsAmbiguousInput(t *testing.T) {
	for name, input := range map[string]string{
		"anonymous": "config steer\n",
		"duplicate": "config steer 'main'\noption enabled '1'\noption enabled '0'\n",
		"mixed":     "config steer 'main'\noption zone 'lan'\nlist zone 'guest'\n",
		"unknown":   "config steer 'main'\ndelete enabled\n",
		"quote":     "config steer 'main\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(input)); err == nil {
				t.Fatal("expected parse failure")
			}
		})
	}
}
