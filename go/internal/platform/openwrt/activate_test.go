// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/openwrt-steer/go/internal/generation"
)

type activationRunner struct{ calls []string }

func (runner *activationRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, call)
	switch {
	case call == "/test/nft -j list tables":
		return []byte(`{"nftables":[{"metainfo":{"version":"1"}}]}`), nil
	case call == "ip -json -4 rule show", call == "ip -json -6 rule show":
		return []byte(`[{"priority":32766,"table":254}]`), nil
	case call == "ip -json -4 route show table all", call == "ip -json -6 route show table all":
		return []byte(`[{"dst":"default","table":254}]`), nil
	case strings.Contains(call, "route flush table 2023"), strings.Contains(call, "route replace local"), strings.Contains(call, "rule add priority 8999"), strings.HasPrefix(call, "/test/nft -f "):
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", call)
	}
}

func TestActivatePublishesOnlyAfterPlatformResources(t *testing.T) {
	root := t.TempDir()
	generationDirectory := filepath.Join(root, "generations", "candidate")
	if err := os.MkdirAll(generationDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generationDirectory, "firewall.nft"), []byte("table inet steer {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{Resources: Resources{MACMark: 0x2026, MACTable: 2023, MACPriority: 8999, MACBindings: []MACBinding{{Address: "02:00:00:00:00:10", TProxyPort: 20000, DNSPort: 20001}}}}
	runner := &activationRunner{}
	candidate := generation.Candidate{Directory: generationDirectory}
	if err := ActivateGeneration(context.Background(), runner, candidate, plan, root, "/test/nft"); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil {
		t.Fatal(err)
	}
	if target != generationDirectory {
		t.Fatalf("unexpected current target: %s", target)
	}
	calls := strings.Join(runner.calls, "\n")
	if strings.Index(calls, "/test/nft -f") > strings.Index(calls, "rule add priority") {
		t.Fatalf("MAC route was loaded before nft table:\n%s", calls)
	}
}

type cleanupRunner struct{ calls []string }

func (runner *cleanupRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, call)
	switch {
	case call == "/test/nft -j list tables":
		return []byte(`{"nftables":[]}`), nil
	case call == "ip -json -4 rule show", call == "ip -json -6 rule show":
		return []byte(`[{"priority":8999,"table":2023,"fwmark":"0x2024/0xffffffff","iif":"br-lan"},{"priority":8999,"table":2023,"fwmark":"0x2026/0xffffffff"}]`), nil
	case call == "ip -json -4 route show table all", call == "ip -json -6 route show table all":
		return []byte(`[{"dst":"default","table":2023}]`), nil
	case strings.Contains(call, "rule del priority 8999"), strings.Contains(call, "route flush table 2023"):
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", call)
	}
}

func TestCleanupRemovesLegacyScopedAndCurrentGlobalMACRules(t *testing.T) {
	runner := &cleanupRunner{}
	plan := Plan{Resources: Resources{MACMark: 0x2026, MACTable: 2023, MACPriority: 8999}}
	if err := CleanupPlatform(context.Background(), runner, plan, "/test/nft"); err != nil {
		t.Fatal(err)
	}
	calls := strings.Join(runner.calls, "\n")
	if !strings.Contains(calls, "rule del priority 8999 iif br-lan fwmark 0x2024 lookup 2023") {
		t.Fatalf("legacy scoped rule was not removed:\n%s", calls)
	}
	if !strings.Contains(calls, "rule del priority 8999 fwmark 0x2026 lookup 2023") {
		t.Fatalf("current global rule was not removed:\n%s", calls)
	}
}
