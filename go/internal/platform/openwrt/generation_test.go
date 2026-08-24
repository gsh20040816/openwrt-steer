// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

type prepareRunner struct {
	calls []string
}

func (runner *prepareRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, call)
	switch {
	case strings.HasSuffix(call, "sing-box version"):
		return []byte("sing-box version 1.14.0-rc.1\nTags: with_quic,with_utls\n"), nil
	case strings.Contains(call, "sing-box check -c") || strings.Contains(call, "nft -c -f"):
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", call)
	}
}

func TestPreparePerformsAllPreMutationChecks(t *testing.T) {
	root := t.TempDir()
	value, err := DecodeBytes([]byte(minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	runner := &prepareRunner{}
	backend := NewBackend(runner, value, BackendOptions{
		RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state"),
		SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft",
	})
	compiled := compiler.Compile(value, backend.CompilerOptions())
	candidate, err := backend.Prepare(context.Background(), value, compiled)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"intent.json", "sing-box.json", "platform.json", "firewall.nft"} {
		if _, err := os.Stat(filepath.Join(candidate.Directory, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	joined := strings.Join(runner.calls, "\n")
	for _, required := range []string{"sing-box version", "sing-box check -c", "nft -c -f"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing check %q in %s", required, joined)
		}
	}
}

func TestBackendDefaultsToPackageOwnedGeoSeed(t *testing.T) {
	backend := NewBackend(nil, model.Intent{}, BackendOptions{})
	if backend.options.GeoDataDirectory != "/usr/share/steer/geodata-seed" {
		t.Fatalf("unexpected OpenWrt Geo seed directory: %q", backend.options.GeoDataDirectory)
	}
}
