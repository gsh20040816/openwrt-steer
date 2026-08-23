// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"version"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != version {
		t.Fatalf("unexpected version output: %q", output.String())
	}
}

func TestRunRequiresExplicitPaths(t *testing.T) {
	for _, args := range [][]string{{"validate"}, {"compile"}, {"compile", "--config", "config.json"}} {
		if err := run(args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("expected explicit path error for %v", args)
		}
	}
}
