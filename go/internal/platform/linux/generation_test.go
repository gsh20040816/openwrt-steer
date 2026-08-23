// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

type prepareRunner struct{ calls []string }

func (runner *prepareRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	runner.calls = append(runner.calls, call)
	if name == "/test/sing-box" && len(args) == 1 && args[0] == "version" {
		return []byte("sing-box version 1.13.19\nTags: with_quic,with_utls\n"), nil
	}
	return nil, nil
}

type geoPrepareRunner struct{ calls []string }

func (runner *geoPrepareRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.calls = append(runner.calls, name+" "+strings.Join(args, " "))
	if name == "/test/sing-box" && len(args) == 1 && args[0] == "version" {
		return []byte("sing-box version 1.13.19\nTags: with_quic,with_utls\n"), nil
	}
	if len(args) > 1 && args[0] == "-action" && args[1] == "extract" {
		return []byte("Available codes:\ncn\n"), nil
	}
	for index, arg := range args {
		if (arg == "-output" || arg == "--output") && index+1 < len(args) {
			if err := os.WriteFile(args[index+1], []byte("compiled\n"), 0o600); err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

func TestPrepareWritesLinuxGenerationFiles(t *testing.T) {
	root := t.TempDir()
	value := validIntent()
	backend := NewBackend(&prepareRunner{}, value, BackendOptions{RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state"), SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft"})
	compiled, err := compiler.Compile(value, backend.CompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := backend.Prepare(context.Background(), value, compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(candidate.Directory)
	for _, name := range []string{"intent.json", "sing-box.json", "platform.json", "firewall.nft"} {
		if _, err := os.Stat(filepath.Join(candidate.Directory, name)); err != nil {
			t.Fatalf("generation file %s is missing: %v", name, err)
		}
	}
	firewall, err := os.ReadFile(filepath.Join(candidate.Directory, "firewall.nft"))
	if err != nil || !strings.Contains(string(firewall), "hook output") {
		t.Fatalf("unexpected Linux firewall: %s (%v)", firewall, err)
	}
}

func TestPrepareUsesConfiguredGeoPathWithoutRequiringUnusedKind(t *testing.T) {
	root := t.TempDir()
	geoSitePath := filepath.Join(root, "geosite.dat")
	if err := os.WriteFile(geoSitePath, []byte("site"), 0o600); err != nil {
		t.Fatal(err)
	}
	value := validIntent()
	value.Rules = append([]model.Rule{{ID: "geo", Enabled: true, DomainMatch: []string{"geosite:cn"}, DNSProfile: "public", Route: "direct"}}, value.Rules...)
	runner := &geoPrepareRunner{}
	backend := NewBackend(runner, value, BackendOptions{
		RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state"),
		SingBoxBinary: "/test/sing-box", NFTBinary: "/test/nft", GeoViewBinary: "/test/geoview",
		GeoSitePath: geoSitePath,
	})
	compiled, err := compiler.Compile(value, backend.CompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := backend.Prepare(context.Background(), value, compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(candidate.Directory)
	found := false
	for _, call := range runner.calls {
		found = found || strings.Contains(call, "-input "+geoSitePath+" ")
	}
	if !found {
		t.Fatalf("configured GeoSite path was not used: %#v", runner.calls)
	}
}
