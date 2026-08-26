// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportArchiveSanitizesSecretsAndReturnsNewestFirst(t *testing.T) {
	stateDirectory := t.TempDir()
	older := Report{
		Scope: "nodes", ObjectID: "node_a", Kind: "connect", TestedAt: time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC),
		Error: "temporary sing-box: password=secret outbound={private_key: hidden}", Results: []Result{{
			URL: "https://user:token@example.test/probe?token=secret&bytes=1000#private", Error: "dial tcp: password=secret",
		}},
	}
	newer := Report{
		Scope: "overview", Kind: "proxy", OK: true, TestedAt: older.TestedAt.Add(time.Hour),
		ActiveGeneration: "generation-a", ActiveDigest: "active-a", Results: []Result{{URL: "https://example.test/", OK: true, Status: 204}},
	}
	if err := SaveReport(stateDirectory, older); err != nil {
		t.Fatal(err)
	}
	if err := SaveReport(stateDirectory, newer); err != nil {
		t.Fatal(err)
	}
	diagnostics := ReadDiagnostics(stateDirectory)
	if len(diagnostics.Reports) != 2 || diagnostics.Reports[0].Kind != "proxy" {
		t.Fatalf("archive order drifted: %#v", diagnostics.Reports)
	}
	encoded := diagnostics.Reports[1].Results[0].URL + diagnostics.Reports[1].Error + diagnostics.Reports[1].Results[0].Error
	for _, secret := range []string{"user:", "token@", "secret", "private_key", "outbound="} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("sanitized archive leaked %q: %s", secret, encoded)
		}
	}
	if !strings.Contains(diagnostics.Reports[1].Results[0].URL, "/REDACTED") || strings.Contains(diagnostics.Reports[1].Results[0].URL, "1000") {
		t.Fatalf("diagnostic URL path/query values were not redacted: %q", diagnostics.Reports[1].Results[0].URL)
	}
}

func TestReportArchiveWarnsAndContinuesPastInvalidState(t *testing.T) {
	stateDirectory := t.TempDir()
	if err := SaveReport(stateDirectory, Report{Scope: "overview", Kind: "direct", TestedAt: time.Now(), Results: []Result{}}); err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(stateDirectory, "logs", "tests", "overview", "invalid.json")
	if err := os.WriteFile(invalid, []byte("{not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnostics := ReadDiagnostics(stateDirectory)
	if len(diagnostics.Reports) != 1 || len(diagnostics.Warnings) != 1 {
		t.Fatalf("invalid state hid valid reports or lacked a warning: %#v", diagnostics)
	}
}

func TestSafeErrorKeepsOnlyStableDiagnosticCategories(t *testing.T) {
	for input, expected := range map[string]string{
		"unexpected HTTP status 503":             "probe target returned HTTP 503",
		"x509: certificate is invalid":           "TLS verification failed",
		"dial tcp: connection refused":           "probe connection was refused",
		"temporary sing-box: private_key=secret": "probe failed",
	} {
		if actual := SafeError(errors.New(input)); actual != expected {
			t.Fatalf("SafeError(%q) = %q, want %q", input, actual, expected)
		}
	}
}
