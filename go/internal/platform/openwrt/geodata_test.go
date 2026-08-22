// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
)

type geoRunner struct {
	calls       []string
	failConvert bool
}

type catalogRunner struct {
	output []byte
}

func (runner catalogRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return runner.output, nil
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

func TestGeoCatalogNormalizesUpstreamOutput(t *testing.T) {
	output := []byte("Available codes:\nCN\n category-games \nGoogle\ncn\n\n")
	names, err := GeoCatalog(context.Background(), catalogRunner{output: output}, "geosite", t.TempDir(), "/test/geoview")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"category-games", "cn", "google"}
	if strings.Join(names, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Geo catalog = %#v, want %#v", names, want)
	}
}

func TestEnsureGeoRulesPassesGeoSiteAttributesToGeoView(t *testing.T) {
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
	runner := &geoRunner{}
	rules := []compiler.GeoRuleSet{{Kind: "geosite", Category: "category-example@test", Path: filepath.Join(root, "state", "geodata", "current", "rules", "geosite-category-example@test.srs")}}
	options := GeoOptions{StateDirectory: filepath.Join(root, "state"), SeedDirectory: seed, GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
	if err := EnsureGeoRules(context.Background(), runner, rules, options); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) < 2 || !strings.Contains(runner.calls[0], "-list category-example@test ") {
		t.Fatalf("geoview was not given the attribute selector: %#v", runner.calls)
	}
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
	obsolete := filepath.Join(root, "state", "geodata", "generation.obsolete")
	if err := os.Mkdir(obsolete, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGeoRules(context.Background(), runner, ruleSets, options); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != firstCallCount {
		t.Fatalf("unchanged Geo generation was rebuilt: %v", runner.calls)
	}
	if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
		t.Fatalf("obsolete Geo generation was not pruned: %v", err)
	}
	current := filepath.Join(root, "state", "geodata", "current")
	firstGeneration, err := filepath.EvalSymlinks(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "release"), []byte("fixture-r2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGeoRules(context.Background(), runner, ruleSets, options); err != nil {
		t.Fatal(err)
	}
	secondGeneration, err := filepath.EvalSymlinks(current)
	if err != nil || secondGeneration == firstGeneration {
		t.Fatalf("Geo release change did not publish a new generation: first=%q second=%q err=%v", firstGeneration, secondGeneration, err)
	}
	if _, err := os.Stat(firstGeneration); !os.IsNotExist(err) {
		t.Fatalf("previous current Geo generation was not pruned: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "state", "geodata"))
	if err != nil {
		t.Fatal(err)
	}
	generationCount := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "generation.") {
			generationCount++
		}
	}
	if generationCount != 1 {
		t.Fatalf("Geo generation count = %d, want 1", generationCount)
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
	if err := EnsureGeoRules(context.Background(), &geoRunner{}, rules, options); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "state", "geodata", "current")
	before, err := filepath.EvalSymlinks(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "release"), []byte("fixture-r2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGeoRules(context.Background(), &geoRunner{failConvert: true}, rules, options); err == nil {
		t.Fatal("missing Geo category was accepted")
	}
	after, err := filepath.EvalSymlinks(current)
	if err != nil || after != before {
		t.Fatalf("failed generation replaced current: before=%q after=%q err=%v", before, after, err)
	}
}

func TestEnsureGeoRulesKeepsCurrentThroughAliasedStatePath(t *testing.T) {
	root := t.TempDir()
	actualState := filepath.Join(root, "actual-state")
	if err := os.Mkdir(actualState, 0o700); err != nil {
		t.Fatal(err)
	}
	aliasedState := filepath.Join(root, "state-alias")
	if err := os.Symlink(actualState, aliasedState); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"release": "fixture-r1\n", "geosite.dat": "site", "geoip.dat": "ip"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rulePath := filepath.Join(aliasedState, "geodata", "current", "rules", "geoip-cn.srs")
	rules := []compiler.GeoRuleSet{{Kind: "geoip", Category: "cn", Path: rulePath}}
	options := GeoOptions{StateDirectory: aliasedState, SeedDirectory: seed, GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
	if err := EnsureGeoRules(context.Background(), &geoRunner{}, rules, options); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(rulePath); err != nil || info.Size() == 0 {
		t.Fatalf("current Geo generation was removed through aliased state path: info=%v err=%v", info, err)
	}
	if err := EnsureGeoRules(context.Background(), &geoRunner{}, rules, options); err != nil {
		t.Fatalf("ready aliased Geo generation could not be reused: %v", err)
	}
}
