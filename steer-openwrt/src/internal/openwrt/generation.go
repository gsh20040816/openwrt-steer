// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/capability"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

type PrepareOptions struct {
	ConfigPath     string
	RunDirectory   string
	StateDirectory string
	SingBoxBinary  string
	NFTBinary      string
	SeedDirectory  string
	GeoViewBinary  string
}

type Generation struct {
	Directory   string          `json:"directory"`
	Bundle      compiler.Bundle `json:"bundle"`
	Environment Environment     `json:"environment"`
	Firewall    string          `json:"-"`
}

func PrepareGeneration(ctx context.Context, runner Runner, options PrepareOptions) (generation Generation, returnErr error) {
	options = normalizePrepareOptions(options)

	configBytes, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		return Generation{}, fmt.Errorf("read candidate UCI: %w", err)
	}
	intent, err := DecodeBytes(configBytes)
	if err != nil {
		return Generation{}, err
	}
	bundle := compiler.CompileWithOptions(intent, compiler.Options{StateDirectory: options.StateDirectory})
	if !bundle.Validation.OK {
		return Generation{}, ValidationError{Validation: bundle.Validation}
	}

	versionOutput, err := runner.Output(ctx, options.SingBoxBinary, "version")
	if err != nil {
		return Generation{}, fmt.Errorf("inspect sing-box: %w", err)
	}
	capabilityReport := capability.Parse(string(versionOutput), bundle.Plan.RequiredCapabilities)
	if !capabilityReport.OK {
		return Generation{}, fmt.Errorf("sing-box capability check failed: %v", capabilityReport.Errors)
	}
	environment, err := ResolveEnvironment(ctx, runner, intent.Main.ManagedZones)
	if err != nil {
		return Generation{}, err
	}
	firewall, err := RenderFirewall(bundle.Plan, environment.ManagedDevices)
	if err != nil {
		return Generation{}, err
	}
	if err := EnsureGeoRules(ctx, runner, bundle.Plan.GeoRuleSets, GeoOptions{StateDirectory: options.StateDirectory, SeedDirectory: options.SeedDirectory, GeoViewBinary: options.GeoViewBinary, SingBoxBinary: options.SingBoxBinary}); err != nil {
		return Generation{}, err
	}
	for _, ruleSet := range bundle.Plan.GeoRuleSets {
		info, statErr := os.Stat(ruleSet.Path)
		if statErr != nil || !info.Mode().IsRegular() {
			return Generation{}, fmt.Errorf("required Geo rule-set is unavailable: %s", ruleSet.Path)
		}
	}
	if err := os.MkdirAll(filepath.Join(options.RunDirectory, "generations"), 0o700); err != nil {
		return Generation{}, fmt.Errorf("create generation root: %w", err)
	}
	directory, err := os.MkdirTemp(filepath.Join(options.RunDirectory, "generations"), "candidate.")
	if err != nil {
		return Generation{}, fmt.Errorf("create candidate generation: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = os.RemoveAll(directory)
		}
	}()
	if err := os.Chmod(directory, 0o700); err != nil {
		return Generation{}, err
	}
	files := []struct {
		name  string
		value any
		mode  os.FileMode
	}{
		{"steer.uci", configBytes, 0o600}, {"bundle.json", bundle, 0o600}, {"plan.json", bundle.Plan, 0o600},
		{"sing-box.json", bundle.SingBox, 0o600}, {"environment.json", environment, 0o600}, {"firewall.nft", []byte(firewall), 0o600},
	}
	for _, file := range files {
		var content []byte
		if raw, ok := file.value.([]byte); ok {
			content = raw
		} else {
			content, err = json.MarshalIndent(file.value, "", "  ")
			if err == nil {
				content = append(content, '\n')
			}
		}
		if err != nil {
			return Generation{}, fmt.Errorf("encode %s: %w", file.name, err)
		}
		if err := os.WriteFile(filepath.Join(directory, file.name), content, file.mode); err != nil {
			return Generation{}, fmt.Errorf("write %s: %w", file.name, err)
		}
	}
	if _, err := runner.Output(ctx, options.SingBoxBinary, "check", "-c", filepath.Join(directory, "sing-box.json")); err != nil {
		return Generation{}, fmt.Errorf("sing-box native check failed: %w", err)
	}
	if _, err := runner.Output(ctx, options.NFTBinary, "-c", "-f", filepath.Join(directory, "firewall.nft")); err != nil {
		return Generation{}, fmt.Errorf("nftables native check failed: %w", err)
	}
	return Generation{Directory: directory, Bundle: bundle, Environment: environment, Firewall: firewall}, nil
}

func normalizePrepareOptions(options PrepareOptions) PrepareOptions {
	if options.ConfigPath == "" {
		options.ConfigPath = "/etc/config/steer"
	}
	if options.RunDirectory == "" {
		options.RunDirectory = "/run/steer"
	}
	if options.StateDirectory == "" {
		options.StateDirectory = "/var/lib/steer"
	}
	if options.SingBoxBinary == "" {
		options.SingBoxBinary = "/usr/bin/sing-box"
	}
	if options.NFTBinary == "" {
		options.NFTBinary = "/usr/sbin/nft"
	}
	return options
}

func DecodeBytes(config []byte) (intent model.Intent, err error) {
	return Decode(bytes.NewReader(config))
}

type ValidationError struct{ Validation model.Validation }

func (value ValidationError) Error() string {
	return fmt.Sprintf("candidate validation failed with %d error(s)", len(value.Validation.Errors))
}
