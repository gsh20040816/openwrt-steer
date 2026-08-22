// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
)

type GeoOptions struct {
	StateDirectory string
	SeedDirectory  string
	GeoViewBinary  string
	SingBoxBinary  string
}

func GeoCatalog(ctx context.Context, runner Runner, kind, seedDirectory, geoViewBinary string) ([]string, error) {
	if kind != "geosite" && kind != "geoip" {
		return nil, fmt.Errorf("Geo catalog kind must be geosite or geoip")
	}
	if seedDirectory == "" {
		seedDirectory = "/usr/share/steer/geodata-seed"
	}
	if geoViewBinary == "" {
		geoViewBinary = "/usr/bin/geoview"
	}
	output, err := runner.Output(ctx, geoViewBinary, "-action", "extract", "-input", filepath.Join(seedDirectory, kind+".dat"), "-type", kind)
	if err != nil {
		return nil, err
	}
	values := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "Available codes:" {
			values = append(values, line)
		}
	}
	sort.Strings(values)
	return values, nil
}

func EnsureGeoRules(ctx context.Context, runner Runner, ruleSets []compiler.GeoRuleSet, options GeoOptions) (returnErr error) {
	if len(ruleSets) == 0 {
		return nil
	}
	if options.StateDirectory == "" {
		options.StateDirectory = "/var/lib/steer"
	}
	if options.SeedDirectory == "" {
		options.SeedDirectory = "/usr/share/steer/geodata-seed"
	}
	if options.GeoViewBinary == "" {
		options.GeoViewBinary = "/usr/bin/geoview"
	}
	if options.SingBoxBinary == "" {
		options.SingBoxBinary = "/usr/bin/sing-box"
	}
	release, err := readRequiredFile(filepath.Join(options.SeedDirectory, "release"))
	if err != nil {
		return fmt.Errorf("read Geo package release: %w", err)
	}
	manifest, err := json.Marshal(ruleSets)
	if err != nil {
		return fmt.Errorf("encode required Geo rules: %w", err)
	}
	root := filepath.Join(options.StateDirectory, "geodata")
	current := filepath.Join(root, "current")
	if geoGenerationReady(current, release, manifest, ruleSets) {
		return pruneGeoGenerations(root, current)
	}
	for _, kind := range []string{"geosite", "geoip"} {
		if _, err := readRequiredFile(filepath.Join(options.SeedDirectory, kind+".dat")); err != nil {
			return fmt.Errorf("read Geo %s seed: %w", kind, err)
		}
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create Geo state root: %w", err)
	}
	generation, err := os.MkdirTemp(root, "generation.")
	if err != nil {
		return fmt.Errorf("create Geo generation: %w", err)
	}
	published := false
	defer func() {
		if returnErr != nil && !published {
			_ = os.RemoveAll(generation)
		}
	}()
	rulesDirectory := filepath.Join(generation, "rules")
	if err := os.Mkdir(rulesDirectory, 0o700); err != nil {
		return err
	}
	for _, ruleSet := range ruleSets {
		jsonPath := filepath.Join(generation, ruleSet.Kind+"-"+ruleSet.Category+".json")
		outputPath := filepath.Join(rulesDirectory, ruleSet.Kind+"-"+ruleSet.Category+".srs")
		seedPath := filepath.Join(options.SeedDirectory, ruleSet.Kind+".dat")
		if _, err := runner.Output(ctx, options.GeoViewBinary, "-action", "convert", "-input", seedPath, "-type", ruleSet.Kind, "-list", ruleSet.Category, "-format", "json", "-strict", "-output", jsonPath); err != nil {
			return fmt.Errorf("convert %s category %q: %w", ruleSet.Kind, ruleSet.Category, err)
		}
		if _, err := runner.Output(ctx, options.SingBoxBinary, "rule-set", "compile", "--output", outputPath, jsonPath); err != nil {
			return fmt.Errorf("compile %s category %q: %w", ruleSet.Kind, ruleSet.Category, err)
		}
		if err := os.Remove(jsonPath); err != nil {
			return fmt.Errorf("remove intermediate Geo JSON: %w", err)
		}
		if info, err := os.Stat(outputPath); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("compiled Geo rule-set is missing or empty: %s", outputPath)
		}
	}
	if err := os.WriteFile(filepath.Join(generation, "manifest.json"), append(manifest, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(generation, "package.release"), release, 0o600); err != nil {
		return err
	}
	temporary := filepath.Join(root, ".current."+strconv.Itoa(os.Getpid()))
	if err := os.Symlink(generation, temporary); err != nil {
		return fmt.Errorf("create Geo current link: %w", err)
	}
	if err := os.Rename(temporary, current); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish Geo generation: %w", err)
	}
	published = true
	return pruneGeoGenerations(root, generation)
}

func pruneGeoGenerations(root, keep string) error {
	resolvedKeep, err := filepath.EvalSymlinks(keep)
	if err != nil {
		return fmt.Errorf("resolve current Geo generation: %w", err)
	}
	resolvedKeep, err = filepath.Abs(resolvedKeep)
	if err != nil {
		return fmt.Errorf("resolve current Geo generation path: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read Geo generations: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "generation.") {
			continue
		}
		candidate, err := filepath.Abs(filepath.Join(root, entry.Name()))
		if err != nil {
			return fmt.Errorf("resolve Geo generation %q: %w", entry.Name(), err)
		}
		if filepath.Clean(candidate) == filepath.Clean(resolvedKeep) {
			continue
		}
		if err := os.RemoveAll(candidate); err != nil {
			return fmt.Errorf("remove obsolete Geo generation %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func geoGenerationReady(current string, release, manifest []byte, ruleSets []compiler.GeoRuleSet) bool {
	currentRelease, err := os.ReadFile(filepath.Join(current, "package.release"))
	if err != nil || !bytes.Equal(currentRelease, release) {
		return false
	}
	currentManifest, err := os.ReadFile(filepath.Join(current, "manifest.json"))
	if err != nil || !bytes.Equal(bytes.TrimSpace(currentManifest), manifest) {
		return false
	}
	for _, ruleSet := range ruleSets {
		info, err := os.Stat(filepath.Join(current, "rules", ruleSet.Kind+"-"+ruleSet.Category+".srs"))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func readRequiredFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}
	return content, nil
}
