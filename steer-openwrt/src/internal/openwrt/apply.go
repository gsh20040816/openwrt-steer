// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
)

type ApplyOptions struct {
	Prepare        PrepareOptions
	InitScript     string
	HTTPClient     *http.Client
	HealthTimeout  time.Duration
	CheckListeners func([]int) error
}

type ApplyResult struct {
	OK           bool          `json:"ok"`
	Generation   string        `json:"generation,omitempty"`
	IntentDigest string        `json:"intent_digest,omitempty"`
	Probes       []ProbeResult `json:"probes"`
	RolledBack   bool          `json:"rolled_back"`
}

type ProbeResult struct {
	URL      string `json:"url"`
	OK       bool   `json:"ok"`
	Attempts int    `json:"attempts"`
	Status   int    `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
}

func Apply(ctx context.Context, runner Runner, options ApplyOptions) (ApplyResult, error) {
	options.Prepare = normalizePrepareOptions(options.Prepare)
	if options.InitScript == "" {
		options.InitScript = "/etc/init.d/steer"
	}
	if options.HealthTimeout == 0 {
		options.HealthTimeout = 10 * time.Second
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	}
	if options.CheckListeners == nil {
		options.CheckListeners = checkListenerPorts
	}
	candidateConfig, err := os.ReadFile(options.Prepare.ConfigPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("read candidate UCI: %w", err)
	}
	intent, err := DecodeBytes(candidateConfig)
	if err != nil {
		return ApplyResult{}, err
	}
	bundle := compiler.CompileWithOptions(intent, compiler.Options{StateDirectory: options.Prepare.StateDirectory})
	if !bundle.Validation.OK {
		return ApplyResult{}, ValidationError{Validation: bundle.Validation}
	}
	if !intent.Main.Enabled {
		return applyDisabled(ctx, runner, options, bundle.IntentDigest)
	}
	candidate, err := PrepareGeneration(ctx, runner, options.Prepare)
	if err != nil {
		return ApplyResult{}, err
	}
	runDirectory := options.Prepare.RunDirectory
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	oldConfig, hasOld := readCurrentConfig(runDirectory)
	if _, err := runner.Output(ctx, options.InitScript, "stop"); err != nil {
		return ApplyResult{}, fmt.Errorf("stop current generation: %w", err)
	}
	if err := ActivateGeneration(ctx, runner, candidate, runDirectory, options.Prepare.NFTBinary); err != nil {
		return rollbackApply(ctx, runner, options, oldConfig, hasOld, nil, err)
	}
	if _, err := runner.Output(ctx, "/usr/bin/env", "STEER_USE_CURRENT=1", options.InitScript, "start"); err != nil {
		return rollbackApply(ctx, runner, options, oldConfig, hasOld, nil, fmt.Errorf("start candidate generation: %w", err))
	}
	ports := []int{candidate.Bundle.Plan.Resources.DNSPort}
	for _, binding := range candidate.Bundle.Plan.Resources.MACBindings {
		ports = append(ports, binding.TProxyPort, binding.DNSPort)
	}
	if err := waitHealthy(ctx, runner, candidate.Bundle.Plan, options.HealthTimeout, options.CheckListeners, ports, options.Prepare.NFTBinary); err != nil {
		return rollbackApply(ctx, runner, options, oldConfig, hasOld, nil, err)
	}
	probes := runProbes(ctx, options.HTTPClient, candidate.Bundle.Plan.Probes)
	for _, probe := range probes {
		if !probe.OK {
			return rollbackApply(ctx, runner, options, oldConfig, hasOld, probes, fmt.Errorf("candidate HTTPS probes failed"))
		}
	}
	if err := pruneGenerations(runDirectory, candidate.Directory); err != nil {
		return ApplyResult{}, fmt.Errorf("prune obsolete runtime generations: %w", err)
	}
	return ApplyResult{OK: true, Generation: candidate.Directory, IntentDigest: candidate.Bundle.IntentDigest, Probes: probes}, nil
}

func applyDisabled(ctx context.Context, runner Runner, options ApplyOptions, digest string) (ApplyResult, error) {
	runDirectory := options.Prepare.RunDirectory
	oldConfig, hasOld := readCurrentConfig(runDirectory)
	if _, err := runner.Output(ctx, options.InitScript, "stop"); err != nil {
		if hasOld {
			if restoreErr := atomicWrite(options.Prepare.ConfigPath, oldConfig); restoreErr != nil {
				return ApplyResult{}, fmt.Errorf("stop Steer while disabling: %w; restore previous UCI: %v", err, restoreErr)
			}
		}
		return ApplyResult{}, fmt.Errorf("stop Steer while disabling: %w", err)
	}
	if err := os.Remove(filepath.Join(runDirectory, "current")); err != nil && !os.IsNotExist(err) {
		return rollbackApply(ctx, runner, options, oldConfig, hasOld, nil, fmt.Errorf("remove disabled current generation: %w", err))
	}
	if err := os.RemoveAll(filepath.Join(runDirectory, "generations")); err != nil {
		return rollbackApply(ctx, runner, options, oldConfig, hasOld, nil, fmt.Errorf("remove disabled runtime generations: %w", err))
	}
	return ApplyResult{OK: true, IntentDigest: digest, Probes: []ProbeResult{}}, nil
}

func rollbackApply(ctx context.Context, runner Runner, options ApplyOptions, oldConfig []byte, hasOld bool, probes []ProbeResult, cause error) (ApplyResult, error) {
	_, stopErr := runner.Output(ctx, options.InitScript, "stop")
	if !hasOld {
		if stopErr != nil {
			return ApplyResult{Probes: probes}, fmt.Errorf("%w; additionally failed to stop rejected candidate: %v", cause, stopErr)
		}
		return ApplyResult{Probes: probes}, cause
	}
	if err := atomicWrite(options.Prepare.ConfigPath, oldConfig); err != nil {
		return ApplyResult{Probes: probes}, fmt.Errorf("%w; restore previous UCI: %v", cause, err)
	}
	if _, err := runner.Output(ctx, options.InitScript, "start"); err != nil {
		return ApplyResult{Probes: probes}, fmt.Errorf("%w; restart previous generation: %v", cause, err)
	}
	return ApplyResult{Probes: probes, RolledBack: true}, cause
}

func readCurrentConfig(runDirectory string) ([]byte, bool) {
	content, err := os.ReadFile(filepath.Join(runDirectory, "current", "steer.uci"))
	return content, err == nil
}

func waitHealthy(ctx context.Context, runner Runner, plan compiler.Plan, timeout time.Duration, listenerCheck func([]int) error, ports []int, nftBinary string) error {
	if nftBinary == "" {
		nftBinary = "/usr/sbin/nft"
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := checkHealthOnce(ctx, runner, plan, listenerCheck, ports, nftBinary); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("candidate did not become locally healthy: %w", lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func WaitCurrentHealthy(ctx context.Context, runner Runner, runDirectory, nftBinary string, timeout time.Duration) error {
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	var plan compiler.Plan
	file, err := os.Open(filepath.Join(runDirectory, "current", "plan.json"))
	if err != nil {
		return fmt.Errorf("open current execution plan: %w", err)
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(&plan); err != nil {
		return fmt.Errorf("decode current execution plan: %w", err)
	}
	ports := []int{plan.Resources.DNSPort}
	for _, binding := range plan.Resources.MACBindings {
		ports = append(ports, binding.TProxyPort, binding.DNSPort)
	}
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return waitHealthy(ctx, runner, plan, timeout, checkListenerPorts, ports, nftBinary)
}

func checkHealthOnce(ctx context.Context, runner Runner, plan compiler.Plan, listenerCheck func([]int) error, ports []int, nftBinary string) error {
	serviceOutput, err := runner.Output(ctx, "ubus", "call", "service", "list", `{"name":"steer"}`)
	if err != nil {
		return err
	}
	var services map[string]struct {
		Instances map[string]struct {
			Running bool `json:"running"`
			PID     int  `json:"pid"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(serviceOutput, &services); err != nil {
		return fmt.Errorf("decode procd status: %w", err)
	}
	service, exists := services["steer"]
	if !exists || !service.Instances["sing-box"].Running || service.Instances["sing-box"].PID <= 0 {
		return fmt.Errorf("procd sing-box instance is not running")
	}
	if _, err := runner.Output(ctx, "ip", "-json", "link", "show", "dev", plan.Resources.TunInterface); err != nil {
		return fmt.Errorf("TUN interface is not ready: %w", err)
	}
	if _, err := runner.Output(ctx, nftBinary, "-j", "list", "table", "inet", "steer"); err != nil {
		return fmt.Errorf("Steer nftables shim is not ready: %w", err)
	}
	if err := listenerCheck(ports); err != nil {
		return err
	}
	return nil
}

func runProbes(ctx context.Context, client *http.Client, urls []string) []ProbeResult {
	results := make([]ProbeResult, len(urls))
	var group sync.WaitGroup
	for index, target := range urls {
		group.Add(1)
		go func() { defer group.Done(); results[index] = probe(ctx, client, target) }()
	}
	group.Wait()
	return results
}

func probe(ctx context.Context, client *http.Client, target string) ProbeResult {
	result := ProbeResult{URL: target}
	for attempt := 1; attempt <= 2; attempt++ {
		result.Attempts = attempt
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			result.Error = err.Error()
			continue
		}
		response, err := client.Do(request)
		if err != nil {
			result.Error = err.Error()
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		closeErr := response.Body.Close()
		result.Status = response.StatusCode
		if closeErr == nil && response.StatusCode >= 200 && response.StatusCode < 400 {
			result.OK = true
			result.Error = ""
			return result
		}
		if closeErr != nil {
			result.Error = closeErr.Error()
		} else {
			result.Error = fmt.Sprintf("unexpected HTTP status %d", response.StatusCode)
		}
	}
	return result
}

func checkListenerPorts(ports []int) error {
	found := map[int]bool{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6", "/proc/net/udp", "/proc/net/udp6"} {
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read listeners from %s: %w", path, err)
		}
		for _, line := range strings.Split(string(content), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			decoded, decodeErr := hex.DecodeString(parts[1])
			if decodeErr != nil || len(decoded) != 2 {
				continue
			}
			port := int(decoded[0])<<8 | int(decoded[1])
			if strings.Contains(path, "tcp") && fields[3] != "0A" {
				continue
			}
			found[port] = true
		}
	}
	for _, port := range ports {
		if !found[port] {
			return fmt.Errorf("expected listener port %d is not ready", port)
		}
	}
	return nil
}

func atomicWrite(path string, content []byte) error {
	if path == "" {
		return fmt.Errorf("rollback configuration path is empty")
	}
	directory := filepath.Dir(path)
	info, err := os.Stat(path)
	mode := os.FileMode(0o600)
	if err == nil {
		mode = info.Mode().Perm()
	}
	temporary, err := os.CreateTemp(directory, ".steer.rollback.")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func pruneGenerations(runDirectory, keep string) error {
	entries, err := os.ReadDir(filepath.Join(runDirectory, "generations"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(runDirectory, "generations", entry.Name())
		if path != keep {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	return nil
}
