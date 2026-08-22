// SPDX-License-Identifier: GPL-3.0-or-later
package main

// CLI tests lock the OpenWrt command surface and Apply serialization.

import (
	"strings"
	"testing"
	"time"
)

func TestApplyLockSerializesTransactions(t *testing.T) {
	runDirectory := t.TempDir()
	first, err := acquireApplyLock(runDirectory)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		second, err := acquireApplyLock(runDirectory)
		acquired <- err
		if err == nil {
			<-release
			second.Close()
		}
	}()
	select {
	case err := <-acquired:
		t.Fatalf("second Apply did not wait for the first lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatal(err)
		}
		close(release)
	case <-time.After(time.Second):
		t.Fatal("second Apply did not acquire the released lock")
	}
}

func TestSubscriptionSubcommandParsesTrailingFlags(t *testing.T) {
	err := runSubscription([]string{"status", "--config", "/definitely/missing/steer"})
	if err == nil || !strings.Contains(err.Error(), "read UCI for subscription status") {
		t.Fatalf("trailing flags were not parsed by the status subcommand: %v", err)
	}
}

func TestProbeParsesRouteAndRejectsAmbiguousTargets(t *testing.T) {
	err := runProbe([]string{"--kind", "speedtest", "--route", "route_a", "--config", "/definitely/missing/steer"})
	if err == nil || !strings.Contains(err.Error(), "read UCI for route test") {
		t.Fatalf("route test flags were not parsed: %v", err)
	}
	err = runProbe([]string{"--kind", "speedtest", "--node", "node_a", "--route", "route_a"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("ambiguous node and route test was accepted: %v", err)
	}
}

func TestUsageExposesOnlyFrozenPublicCommands(t *testing.T) {
	message := usage().Error()
	for _, command := range []string{"version", "validate", "apply", "health", "status", "probe", "subscription", "geo-catalog", "cleanup"} {
		if !strings.Contains(message, command) {
			t.Fatalf("public command %q is missing from usage: %s", command, message)
		}
	}
	for _, removed := range []string{"compile", "plan", "prepare", "capabilities", "rollback", "_start"} {
		if strings.Contains(message, removed) {
			t.Fatalf("non-public command %q leaked into usage: %s", removed, message)
		}
	}
}
