// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
)

type geoRunner struct {
	calls       []string
	failConvert bool
}

func (runner *geoRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.calls = append(runner.calls, name+" "+strings.Join(args, " "))
	if runner.failConvert && strings.HasSuffix(name, "geoview") {
		return nil, fmt.Errorf("category is missing")
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

func TestEnsureGeoRulesBuildsAndReusesExactGeneration(t *testing.T) {
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"release": "fixture-r1\n", "geosite.dat": "site", "geoip.dat": "ip"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ruleSets := []compiler.GeoRuleSet{
		{Kind: "geoip", Category: "private", Path: filepath.Join(root, "state", "geodata", "current", "rules", "geoip-private.srs")},
		{Kind: "geosite", Category: "category-example", Path: filepath.Join(root, "state", "geodata", "current", "rules", "geosite-category-example.srs")},
	}
	runner := &geoRunner{}
	options := GeoOptions{StateDirectory: filepath.Join(root, "state"), SeedDirectory: seed, GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
	if err := EnsureGeoRules(context.Background(), runner, ruleSets, options); err != nil {
		t.Fatal(err)
	}
	for _, ruleSet := range ruleSets {
		if info, err := os.Stat(ruleSet.Path); err != nil || info.Size() == 0 {
			t.Fatalf("missing compiled rule-set %s: %v", ruleSet.Path, err)
		}
	}
	firstCallCount := len(runner.calls)
	if err := EnsureGeoRules(context.Background(), runner, ruleSets, options); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != firstCallCount {
		t.Fatalf("unchanged Geo generation was rebuilt: %v", runner.calls)
	}
}

func TestEnsureGeoRulesPreservesCurrentOnConversionFailure(t *testing.T) {
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"release": "fixture-r1\n", "geosite.dat": "site", "geoip.dat": "ip"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rules := []compiler.GeoRuleSet{{Kind: "geosite", Category: "missing"}}
	options := GeoOptions{StateDirectory: filepath.Join(root, "state"), SeedDirectory: seed, GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
	if err := EnsureGeoRules(context.Background(), &geoRunner{failConvert: true}, rules, options); err == nil {
		t.Fatal("missing Geo category was accepted")
	}
	if _, err := os.Lstat(filepath.Join(root, "state", "geodata", "current")); !os.IsNotExist(err) {
		t.Fatalf("failed generation became current: %v", err)
	}
}
