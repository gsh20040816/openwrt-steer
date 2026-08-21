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
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

type ApplyOptions struct {
	Prepare        PrepareOptions
	InitScript     string
	BackupPath     string
	HealthTimeout  time.Duration
	CheckListeners func([]int) error
}

type ApplyResult struct {
	OK           bool              `json:"ok"`
	Error        string            `json:"error,omitempty"`
	Generation   string            `json:"generation,omitempty"`
	IntentDigest string            `json:"intent_digest,omitempty"`
	Validation   *model.Validation `json:"validation,omitempty"`
}

type ProbeResult struct {
	URL      string `json:"url"`
	OK       bool   `json:"ok"`
	Attempts int    `json:"attempts"`
	Status   int    `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
}

func Apply(ctx context.Context, runner Runner, options ApplyOptions) (ApplyResult, error) {
	return apply(ctx, runner, options, true)
}

func apply(ctx context.Context, runner Runner, options ApplyOptions, saveBackup bool) (ApplyResult, error) {
	options.Prepare = normalizePrepareOptions(options.Prepare)
	if options.InitScript == "" {
		options.InitScript = "/etc/init.d/steer"
	}
	if options.BackupPath == "" {
		options.BackupPath = "/var/lib/steer/rollback.uci"
	}
	if options.HealthTimeout == 0 {
		options.HealthTimeout = 10 * time.Second
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
		return ApplyResult{IntentDigest: bundle.IntentDigest, Validation: &bundle.Validation}, ValidationError{Validation: bundle.Validation}
	}
	result := ApplyResult{IntentDigest: bundle.IntentDigest}
	if !intent.Main.Enabled {
		return applyDisabled(ctx, runner, options, result, saveBackup)
	}
	candidate, err := PrepareGeneration(ctx, runner, options.Prepare)
	if err != nil {
		return result, err
	}
	result.Generation = candidate.Directory
	runDirectory := options.Prepare.RunDirectory
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	if saveBackup {
		if err := backupHealthyCurrent(ctx, runner, options, runDirectory); err != nil {
			return result, err
		}
	}
	if _, err := runner.Output(ctx, options.InitScript, "stop"); err != nil {
		return result, fmt.Errorf("stop current generation: %w", err)
	}
	if err := ActivateGeneration(ctx, runner, candidate, runDirectory, options.Prepare.NFTBinary); err != nil {
		return result, err
	}
	if _, err := runner.Output(ctx, "/usr/bin/env", "STEER_USE_CURRENT=1", options.InitScript, "start"); err != nil {
		return result, fmt.Errorf("start candidate generation: %w", err)
	}
	ports := []int{candidate.Bundle.Plan.Resources.DNSPort}
	for _, binding := range candidate.Bundle.Plan.Resources.MACBindings {
		ports = append(ports, binding.TProxyPort, binding.DNSPort)
	}
	if err := waitHealthy(ctx, runner, candidate.Bundle.Plan, options.HealthTimeout, options.CheckListeners, ports, options.Prepare.NFTBinary); err != nil {
		return result, err
	}
	if err := pruneGenerations(runDirectory, candidate.Directory); err != nil {
		return result, fmt.Errorf("prune obsolete runtime generations: %w", err)
	}
	result.OK = true
	return result, nil
}

func applyDisabled(ctx context.Context, runner Runner, options ApplyOptions, result ApplyResult, saveBackup bool) (ApplyResult, error) {
	runDirectory := options.Prepare.RunDirectory
	if saveBackup {
		if err := backupHealthyCurrent(ctx, runner, options, runDirectory); err != nil {
			return result, err
		}
	}
	if _, err := runner.Output(ctx, options.InitScript, "stop"); err != nil {
		return result, fmt.Errorf("stop Steer while disabling: %w", err)
	}
	if err := os.Remove(filepath.Join(runDirectory, "current")); err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("remove disabled current generation: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(runDirectory, "generations")); err != nil {
		return result, fmt.Errorf("remove disabled runtime generations: %w", err)
	}
	result.OK = true
	return result, nil
}

func backupHealthyCurrent(ctx context.Context, runner Runner, options ApplyOptions, runDirectory string) error {
	config, ok := readCurrentConfig(runDirectory)
	if !ok {
		return nil
	}
	plan, err := readCurrentPlan(runDirectory)
	if err != nil {
		return nil
	}
	ports := planListenerPorts(plan)
	if err := checkHealthOnce(ctx, runner, plan, options.CheckListeners, ports, options.Prepare.NFTBinary); err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(options.BackupPath), 0o700); err != nil {
		return fmt.Errorf("create rollback backup directory: %w", err)
	}
	if err := atomicWrite(options.BackupPath, config); err != nil {
		return fmt.Errorf("save rollback UCI: %w", err)
	}
	return nil
}

func Rollback(ctx context.Context, runner Runner, options ApplyOptions) (ApplyResult, error) {
	options.Prepare = normalizePrepareOptions(options.Prepare)
	if options.BackupPath == "" {
		options.BackupPath = "/var/lib/steer/rollback.uci"
	}
	config, err := os.ReadFile(options.BackupPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("read rollback UCI: %w", err)
	}
	if err := atomicWrite(options.Prepare.ConfigPath, config); err != nil {
		return ApplyResult{}, fmt.Errorf("restore rollback UCI: %w", err)
	}
	result, err := apply(ctx, runner, options, false)
	if err != nil {
		return result, err
	}
	if err := os.Remove(options.BackupPath); err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("consume rollback UCI: %w", err)
	}
	return result, nil
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
	plan, err := readCurrentPlan(runDirectory)
	if err != nil {
		return err
	}
	ports := planListenerPorts(plan)
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return waitHealthy(ctx, runner, plan, timeout, checkListenerPorts, ports, nftBinary)
}

func readCurrentPlan(runDirectory string) (compiler.Plan, error) {
	var plan compiler.Plan
	file, err := os.Open(filepath.Join(runDirectory, "current", "plan.json"))
	if err != nil {
		return plan, fmt.Errorf("open current execution plan: %w", err)
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(&plan); err != nil {
		return plan, fmt.Errorf("decode current execution plan: %w", err)
	}
	return plan, nil
}

func planListenerPorts(plan compiler.Plan) []int {
	ports := []int{plan.Resources.DNSPort}
	for _, binding := range plan.Resources.MACBindings {
		ports = append(ports, binding.TProxyPort, binding.DNSPort)
	}
	return ports
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

type ProbeReport struct {
	Kind    string        `json:"kind"`
	OK      bool          `json:"ok"`
	Results []ProbeResult `json:"results"`
}

func ProbeCurrent(ctx context.Context, runDirectory, kind string, client *http.Client) (ProbeReport, error) {
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	plan, err := readCurrentPlan(runDirectory)
	if err != nil {
		return ProbeReport{}, err
	}
	var urls []string
	switch kind {
	case "direct":
		urls = plan.ProbeDirect
	case "proxy":
		urls = plan.ProbeProxy
	case "speedtest":
		urls = plan.SpeedtestProxy
	default:
		return ProbeReport{}, fmt.Errorf("unsupported probe kind %q", kind)
	}
	if len(urls) == 0 {
		return ProbeReport{Kind: kind, OK: false}, fmt.Errorf("current execution plan has no %s HTTPS probes", kind)
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	}
	results := runProbes(ctx, client, urls)
	report := ProbeReport{Kind: kind, OK: true, Results: results}
	for _, result := range results {
		if !result.OK {
			report.OK = false
			break
		}
	}
	return report, nil
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
