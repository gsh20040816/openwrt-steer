// SPDX-License-Identifier: GPL-3.0-or-later

// Package geodata owns platform-neutral Geo database inspection and sing-box
// rule-set generation. Platform adapters provide explicit database paths.
package geodata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gsh20040816/steer/go/internal/compiler"
)

const (
	ErrorPathNotConfigured = "GEO_PATH_NOT_CONFIGURED"
	ErrorPathInvalid       = "GEO_PATH_INVALID"
	ErrorInputEmpty        = "GEO_INPUT_EMPTY"
	ErrorCategoryNotFound  = "GEO_CATEGORY_NOT_FOUND"
	ErrorToolFailed        = "GEO_TOOL_FAILED"
)

type Error struct {
	Code     string `json:"code"`
	Kind     string `json:"kind,omitempty"`
	Category string `json:"category,omitempty"`
	Path     string `json:"path,omitempty"`
	Err      error  `json:"-"`
}

func (value *Error) Error() string {
	label := map[string]string{"geosite": "GeoSite", "geoip": "GeoIP"}[value.Kind]
	if label == "" {
		label = "Geo"
	}
	switch value.Code {
	case ErrorPathNotConfigured:
		return fmt.Sprintf("%s database path is not configured", label)
	case ErrorPathInvalid:
		return fmt.Sprintf("%s database path %q is invalid: %v", label, value.Path, value.Err)
	case ErrorInputEmpty:
		return fmt.Sprintf("%s database %q is empty", label, value.Path)
	case ErrorCategoryNotFound:
		return fmt.Sprintf("%s category %q is not available in %s", label, value.Category, value.Path)
	case ErrorToolFailed:
		if value.Category != "" {
			return fmt.Sprintf("generate %s category %q from %s: %v", value.Kind, value.Category, value.Path, value.Err)
		}
		return fmt.Sprintf("inspect %s database %s: %v", value.Kind, value.Path, value.Err)
	default:
		return fmt.Sprintf("Geo operation failed: %v", value.Err)
	}
}

func (value *Error) Unwrap() error { return value.Err }

type Runner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type Options struct {
	StateDirectory string
	GeoSitePath    string
	GeoIPPath      string
	GeoViewBinary  string
	SingBoxBinary  string
}

type inputIdentity struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
}

type ruleIdentity struct {
	Kind     string `json:"kind"`
	Category string `json:"category"`
}

type generationManifest struct {
	SchemaVersion int             `json:"schema_version"`
	Inputs        []inputIdentity `json:"inputs"`
	Rules         []ruleIdentity  `json:"rules"`
}

func Catalog(ctx context.Context, runner Runner, kind, path, geoViewBinary string) ([]string, error) {
	if kind != "geosite" && kind != "geoip" {
		return nil, fmt.Errorf("Geo catalog kind must be geosite or geoip")
	}
	if err := validateInputPath(kind, path); err != nil {
		return nil, err
	}
	if geoViewBinary == "" {
		geoViewBinary = "/usr/bin/geoview"
	}
	output, err := runner.Output(ctx, geoViewBinary, "-action", "extract", "-input", path, "-type", kind)
	if err != nil {
		return nil, &Error{Code: ErrorToolFailed, Kind: kind, Path: path, Err: err}
	}
	return normalizeCatalog(output), nil
}

func EnsureRules(ctx context.Context, runner Runner, ruleSets []compiler.GeoRuleSet, options Options) (returnErr error) {
	rules, err := normalizeRules(ruleSets)
	if err != nil || len(rules) == 0 {
		return err
	}
	if options.StateDirectory == "" {
		options.StateDirectory = "/var/lib/steer"
	}
	if options.GeoViewBinary == "" {
		options.GeoViewBinary = "/usr/bin/geoview"
	}
	if options.SingBoxBinary == "" {
		options.SingBoxBinary = "/usr/bin/sing-box"
	}

	inputs := make([]inputIdentity, 0, 2)
	for _, kind := range requiredKinds(rules) {
		path := inputPath(options, kind)
		digest, hashErr := hashInput(kind, path)
		if hashErr != nil {
			return hashErr
		}
		inputs = append(inputs, inputIdentity{Kind: kind, SHA256: digest})
	}
	manifest, err := json.Marshal(generationManifest{SchemaVersion: 1, Inputs: inputs, Rules: rules})
	if err != nil {
		return fmt.Errorf("encode required Geo rules: %w", err)
	}
	root := filepath.Join(options.StateDirectory, "geodata")
	current := filepath.Join(root, "current")
	if generationReady(current, manifest, rules) {
		return pruneGenerations(root, current)
	}

	for _, kind := range requiredKinds(rules) {
		path := inputPath(options, kind)
		names, catalogErr := Catalog(ctx, runner, kind, path, options.GeoViewBinary)
		if catalogErr != nil {
			return catalogErr
		}
		known := make(map[string]struct{}, len(names))
		for _, name := range names {
			known[name] = struct{}{}
		}
		for _, rule := range rules {
			if rule.Kind != kind {
				continue
			}
			if _, exists := known[rule.Category]; !exists {
				return &Error{Code: ErrorCategoryNotFound, Kind: kind, Category: rule.Category, Path: path}
			}
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
	for _, rule := range rules {
		jsonPath := filepath.Join(generation, rule.Kind+"-"+rule.Category+".json")
		outputPath := filepath.Join(rulesDirectory, rule.Kind+"-"+rule.Category+".srs")
		path := inputPath(options, rule.Kind)
		if _, err := runner.Output(ctx, options.GeoViewBinary, "-action", "convert", "-input", path, "-type", rule.Kind, "-list", rule.Category, "-format", "json", "-strict", "-output", jsonPath); err != nil {
			return &Error{Code: ErrorToolFailed, Kind: rule.Kind, Category: rule.Category, Path: path, Err: err}
		}
		if _, err := runner.Output(ctx, options.SingBoxBinary, "rule-set", "compile", "--output", outputPath, jsonPath); err != nil {
			return &Error{Code: ErrorToolFailed, Kind: rule.Kind, Category: rule.Category, Path: path, Err: err}
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
	temporary := filepath.Join(root, ".current."+strconv.Itoa(os.Getpid()))
	if err := os.Symlink(generation, temporary); err != nil {
		return fmt.Errorf("create Geo current link: %w", err)
	}
	if err := os.Rename(temporary, current); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish Geo generation: %w", err)
	}
	published = true
	return pruneGenerations(root, generation)
}

func normalizeRules(ruleSets []compiler.GeoRuleSet) ([]ruleIdentity, error) {
	seen := map[ruleIdentity]struct{}{}
	for _, ruleSet := range ruleSets {
		if ruleSet.Kind != "geosite" && ruleSet.Kind != "geoip" {
			return nil, fmt.Errorf("unsupported Geo rule kind %q", ruleSet.Kind)
		}
		if strings.TrimSpace(ruleSet.Category) == "" {
			return nil, fmt.Errorf("%s rule category is empty", ruleSet.Kind)
		}
		seen[ruleIdentity{Kind: ruleSet.Kind, Category: ruleSet.Category}] = struct{}{}
	}
	rules := make([]ruleIdentity, 0, len(seen))
	for rule := range seen {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Kind == rules[j].Kind {
			return rules[i].Category < rules[j].Category
		}
		return rules[i].Kind < rules[j].Kind
	})
	return rules, nil
}

func normalizeCatalog(output []byte) []string {
	unique := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "" && line != "available codes:" {
			unique[line] = struct{}{}
		}
	}
	values := make([]string, 0, len(unique))
	for line := range unique {
		values = append(values, line)
	}
	sort.Strings(values)
	return values
}

func requiredKinds(rules []ruleIdentity) []string {
	kinds := []string{}
	for _, kind := range []string{"geoip", "geosite"} {
		for _, rule := range rules {
			if rule.Kind == kind {
				kinds = append(kinds, kind)
				break
			}
		}
	}
	return kinds
}

func inputPath(options Options, kind string) string {
	if kind == "geosite" {
		return options.GeoSitePath
	}
	return options.GeoIPPath
}

func validateInputPath(kind, path string) error {
	if path == "" {
		return &Error{Code: ErrorPathNotConfigured, Kind: kind}
	}
	if !filepath.IsAbs(path) {
		return &Error{Code: ErrorPathInvalid, Kind: kind, Path: path, Err: fmt.Errorf("path must be absolute")}
	}
	file, err := os.Open(path)
	if err != nil {
		return &Error{Code: ErrorPathInvalid, Kind: kind, Path: path, Err: err}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return &Error{Code: ErrorPathInvalid, Kind: kind, Path: path, Err: err}
	}
	if !info.Mode().IsRegular() {
		return &Error{Code: ErrorPathInvalid, Kind: kind, Path: path, Err: fmt.Errorf("not a regular file")}
	}
	if info.Size() == 0 {
		return &Error{Code: ErrorInputEmpty, Kind: kind, Path: path}
	}
	return nil
}

func hashInput(kind, path string) (string, error) {
	if err := validateInputPath(kind, path); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", &Error{Code: ErrorPathInvalid, Kind: kind, Path: path, Err: err}
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, file)
	if err != nil {
		return "", &Error{Code: ErrorPathInvalid, Kind: kind, Path: path, Err: err}
	}
	if written == 0 {
		return "", &Error{Code: ErrorInputEmpty, Kind: kind, Path: path}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func generationReady(current string, manifest []byte, rules []ruleIdentity) bool {
	currentManifest, err := os.ReadFile(filepath.Join(current, "manifest.json"))
	if err != nil || !bytes.Equal(bytes.TrimSpace(currentManifest), manifest) {
		return false
	}
	for _, rule := range rules {
		info, err := os.Stat(filepath.Join(current, "rules", rule.Kind+"-"+rule.Category+".srs"))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func pruneGenerations(root, keep string) error {
	keepInfo, err := os.Stat(keep)
	if err != nil {
		return fmt.Errorf("stat current Geo generation: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read Geo generations: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "generation.") {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		candidateInfo, err := os.Stat(candidate)
		if err != nil {
			return fmt.Errorf("stat Geo generation %q: %w", entry.Name(), err)
		}
		if os.SameFile(candidateInfo, keepInfo) {
			continue
		}
		if err := os.RemoveAll(candidate); err != nil {
			return fmt.Errorf("remove obsolete Geo generation %q: %w", entry.Name(), err)
		}
	}
	return nil
}
