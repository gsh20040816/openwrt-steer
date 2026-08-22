// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gsh20040816/openwrt-steer/go/internal/capability"
	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/go/internal/generation"
	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
)

type BackendOptions struct {
	RunDirectory   string
	StateDirectory string
	SingBoxBinary  string
	NFTBinary      string
	SeedDirectory  string
	GeoViewBinary  string
	InitScript     string
	HealthTimeout  time.Duration
	CheckListeners func([]int) error
}

type Backend struct {
	runner  Runner
	options BackendOptions
	plan    Plan
}

func NewBackend(runner Runner, value model.Intent, options BackendOptions) *Backend {
	if runner == nil {
		runner = ExecRunner{}
	}
	options = normalizeBackendOptions(options)
	return &Backend{runner: runner, options: options, plan: NewPlan(value)}
}

func (backend *Backend) CompilerOptions() compiler.Options {
	return compiler.Options{StateDirectory: backend.options.StateDirectory, Target: backend.plan.CompilerTarget()}
}

func (backend *Backend) Prepare(ctx context.Context, value model.Intent, compiled compiler.Output) (candidate generation.Candidate, returnErr error) {
	versionOutput, err := backend.runner.Output(ctx, backend.options.SingBoxBinary, "version")
	if err != nil {
		return generation.Candidate{}, fmt.Errorf("inspect sing-box: %w", err)
	}
	capabilityReport := capability.Parse(string(versionOutput), compiled.RequiredCapabilities)
	if !capabilityReport.OK {
		return generation.Candidate{}, fmt.Errorf("sing-box capability check failed: %v", capabilityReport.Errors)
	}
	firewall, err := RenderFirewall(backend.plan)
	if err != nil {
		return generation.Candidate{}, err
	}
	if err := EnsureGeoRules(ctx, backend.runner, compiled.GeoRuleSets, GeoOptions{
		StateDirectory: backend.options.StateDirectory, SeedDirectory: backend.options.SeedDirectory,
		GeoViewBinary: backend.options.GeoViewBinary, SingBoxBinary: backend.options.SingBoxBinary,
	}); err != nil {
		return generation.Candidate{}, err
	}
	for _, ruleSet := range compiled.GeoRuleSets {
		info, statErr := os.Stat(ruleSet.Path)
		if statErr != nil || !info.Mode().IsRegular() {
			return generation.Candidate{}, fmt.Errorf("required Geo rule-set is unavailable: %s", ruleSet.Path)
		}
	}
	candidate, err = generation.Create(filepath.Join(backend.options.RunDirectory, "generations"), value, compiled.SingBox)
	if err != nil {
		return generation.Candidate{}, err
	}
	defer func() {
		if returnErr != nil {
			_ = os.RemoveAll(candidate.Directory)
		}
	}()
	if err := generation.WriteJSON(filepath.Join(candidate.Directory, "platform.json"), backend.plan); err != nil {
		return generation.Candidate{}, err
	}
	if err := os.WriteFile(filepath.Join(candidate.Directory, "firewall.nft"), []byte(firewall), 0o600); err != nil {
		return generation.Candidate{}, fmt.Errorf("write firewall.nft: %w", err)
	}
	if _, err := backend.runner.Output(ctx, backend.options.SingBoxBinary, "check", "-c", filepath.Join(candidate.Directory, "sing-box.json")); err != nil {
		return generation.Candidate{}, fmt.Errorf("sing-box native check failed: %w", err)
	}
	if _, err := backend.runner.Output(ctx, backend.options.NFTBinary, "-c", "-f", filepath.Join(candidate.Directory, "firewall.nft")); err != nil {
		return generation.Candidate{}, fmt.Errorf("nftables native check failed: %w", err)
	}
	return candidate, nil
}

func normalizeBackendOptions(options BackendOptions) BackendOptions {
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
	if options.InitScript == "" {
		options.InitScript = "/etc/init.d/steer"
	}
	if options.HealthTimeout == 0 {
		options.HealthTimeout = 10 * time.Second
	}
	if options.CheckListeners == nil {
		options.CheckListeners = checkListenerPorts
	}
	return options
}

type ValidationError struct{ Validation model.Validation }

func (value ValidationError) Error() string {
	return fmt.Sprintf("candidate validation failed with %d error(s)", len(value.Validation.Errors))
}
