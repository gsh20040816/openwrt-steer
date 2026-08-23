// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/geodata"
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
	if len(args) > 1 && args[0] == "-action" && args[1] == "extract" {
		return []byte("Available codes:\ncategory-example@test\ncategory-example\nprivate\nmissing\ncn\n"), nil
	}
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
	path := filepath.Join(t.TempDir(), "geosite.dat")
	if err := os.WriteFile(path, []byte("site"), 0o600); err != nil {
		t.Fatal(err)
	}
	names, err := GeoCatalog(context.Background(), catalogRunner{output: output}, "geosite", path, "/test/geoview")
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
	for name, content := range map[string]string{"geosite.dat": "site"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runner := &geoRunner{}
	rules := []compiler.GeoRuleSet{{Kind: "geosite", Category: "category-example@test", Path: filepath.Join(root, "state", "geodata", "current", "rules", "geosite-category-example@test.srs")}}
	options := GeoOptions{StateDirectory: filepath.Join(root, "state"), GeoSitePath: filepath.Join(seed, "geosite.dat"), GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
	if err := EnsureGeoRules(context.Background(), runner, rules, options); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, call := range runner.calls {
		found = found || strings.Contains(call, "-list category-example@test ")
	}
	if !found {
		t.Fatalf("geoview was not given the attribute selector: %#v", runner.calls)
	}
}

func TestEnsureGeoRulesBuildsAndReusesExactGeneration(t *testing.T) {
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"geosite.dat": "site", "geoip.dat": "ip"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ruleSets := []compiler.GeoRuleSet{
		{Kind: "geoip", Category: "private", Path: filepath.Join(root, "state", "geodata", "current", "rules", "geoip-private.srs")},
		{Kind: "geosite", Category: "category-example", Path: filepath.Join(root, "state", "geodata", "current", "rules", "geosite-category-example.srs")},
	}
	runner := &geoRunner{}
	options := GeoOptions{StateDirectory: filepath.Join(root, "state"), GeoSitePath: filepath.Join(seed, "geosite.dat"), GeoIPPath: filepath.Join(seed, "geoip.dat"), GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
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
	manifest, err := os.ReadFile(filepath.Join(firstGeneration, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), seed) || strings.Contains(string(manifest), "release") || !strings.Contains(string(manifest), `"sha256"`) {
		t.Fatalf("Geo manifest contains path/release state or lacks content hash: %s", manifest)
	}
	copyPath := filepath.Join(seed, "geosite-copy.dat")
	if err := os.WriteFile(copyPath, []byte("site"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.GeoSitePath = copyPath
	if err := EnsureGeoRules(context.Background(), runner, ruleSets, options); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != firstCallCount {
		t.Fatalf("same Geo content at a new path was rebuilt: %v", runner.calls)
	}
	info, err := os.Stat(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, []byte("site-v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(copyPath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGeoRules(context.Background(), runner, ruleSets, options); err != nil {
		t.Fatal(err)
	}
	secondGeneration, err := filepath.EvalSymlinks(current)
	if err != nil || secondGeneration == firstGeneration {
		t.Fatalf("Geo content change did not publish a new generation: first=%q second=%q err=%v", firstGeneration, secondGeneration, err)
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

func TestEnsureGeoRulesReturnsStructuredResourceErrors(t *testing.T) {
	rules := []compiler.GeoRuleSet{{Kind: "geosite", Category: "cn"}}
	state := filepath.Join(t.TempDir(), "state")
	cases := []struct {
		name    string
		path    string
		prepare func(string) error
		code    string
	}{
		{name: "not configured", code: geodata.ErrorPathNotConfigured},
		{name: "relative", path: "geosite.dat", code: geodata.ErrorPathInvalid},
		{name: "empty", path: filepath.Join(t.TempDir(), "geosite.dat"), prepare: func(path string) error { return os.WriteFile(path, nil, 0o600) }, code: geodata.ErrorInputEmpty},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.prepare != nil {
				if err := test.prepare(test.path); err != nil {
					t.Fatal(err)
				}
			}
			err := EnsureGeoRules(context.Background(), &geoRunner{}, rules, GeoOptions{StateDirectory: state, GeoSitePath: test.path})
			var geoErr *geodata.Error
			if !errors.As(err, &geoErr) || geoErr.Code != test.code || geoErr.Kind != "geosite" {
				t.Fatalf("error = %#v, want %s geosite error", err, test.code)
			}
		})
	}
}

func TestEnsureGeoRulesReportsMissingCategory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "geosite.dat")
	if err := os.WriteFile(path, []byte("site"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := EnsureGeoRules(context.Background(), &geoRunner{}, []compiler.GeoRuleSet{{Kind: "geosite", Category: "not-present"}}, GeoOptions{StateDirectory: filepath.Join(t.TempDir(), "state"), GeoSitePath: path, GeoViewBinary: "/test/geoview"})
	var geoErr *geodata.Error
	if !errors.As(err, &geoErr) || geoErr.Code != geodata.ErrorCategoryNotFound || geoErr.Category != "not-present" {
		t.Fatalf("error = %#v, want missing category", err)
	}
}

func TestEnsureGeoRulesPreservesCurrentOnConversionFailure(t *testing.T) {
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"geosite.dat": "site"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rules := []compiler.GeoRuleSet{{Kind: "geosite", Category: "missing"}}
	options := GeoOptions{StateDirectory: filepath.Join(root, "state"), GeoSitePath: filepath.Join(seed, "geosite.dat"), GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
	if err := EnsureGeoRules(context.Background(), &geoRunner{}, rules, options); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "state", "geodata", "current")
	before, err := filepath.EvalSymlinks(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "geosite.dat"), []byte("site-v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = EnsureGeoRules(context.Background(), &geoRunner{failConvert: true}, rules, options)
	var geoErr *geodata.Error
	if !errors.As(err, &geoErr) || geoErr.Code != geodata.ErrorToolFailed {
		t.Fatalf("conversion failure = %#v, want %s", err, geodata.ErrorToolFailed)
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
	for name, content := range map[string]string{"geoip.dat": "ip"} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rulePath := filepath.Join(aliasedState, "geodata", "current", "rules", "geoip-cn.srs")
	rules := []compiler.GeoRuleSet{{Kind: "geoip", Category: "cn", Path: rulePath}}
	options := GeoOptions{StateDirectory: aliasedState, GeoIPPath: filepath.Join(seed, "geoip.dat"), GeoViewBinary: "/test/geoview", SingBoxBinary: "/test/sing-box"}
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
