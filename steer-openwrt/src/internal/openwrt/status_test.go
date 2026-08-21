// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner map[string]string

func (runner fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	value, ok := runner[key]
	if !ok {
		return nil, fmt.Errorf("unexpected command: %s", key)
	}
	return []byte(value), nil
}

func TestStatusReportsRunningGenerationWhenCandidateIsInvalid(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "steer")
	if err := os.WriteFile(configPath, []byte("config steer 'main'\n\toption schema_version '4'\n\toption router_proxy '1'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{
		`ubus call service list {"name":"steer"}`: `{"steer":{"instances":{"sing-box":{"running":true,"pid":123}}}}`,
		"ip -json link show dev steer0":           `[]`,
		"/test/nft -j list table inet steer":      `{}`,
	}

	status := ReadStatus(context.Background(), runner, configPath, filepath.Join(directory, "run"), "/test/nft")
	if !status.CoreRunning || status.CorePID != 123 {
		t.Fatalf("running generation hidden by invalid candidate: %#v", status)
	}
	if len(status.Validation.Errors) == 0 || status.Healthy {
		t.Fatalf("invalid candidate was not reported independently: %#v", status)
	}
}

func TestStatusReportsRollbackAvailability(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "steer")
	backupPath := filepath.Join(directory, "rollback.uci")
	if err := os.WriteFile(configPath, []byte(minimalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte(minimalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	status := ReadStatusWithBackup(context.Background(), fakeRunner{}, configPath, filepath.Join(directory, "run"), "/test/nft", backupPath)
	if !status.RollbackAvailable || status.CandidateDigest == "" {
		t.Fatalf("status omitted candidate or rollback state: %#v", status)
	}
}
