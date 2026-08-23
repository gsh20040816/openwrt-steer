// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
)

type Status struct {
	Healthy   bool              `json:"healthy"`
	LastApply *coreapply.Record `json:"last_apply,omitempty"`
}

func WaitCurrentHealthy(ctx context.Context, runner Runner, options BackendOptions, timeout time.Duration) error {
	options = normalizeBackendOptions(options)
	plan, err := readCurrentPlan(options.RunDirectory)
	if err != nil {
		return err
	}
	if timeout > 0 {
		options.HealthTimeout = timeout
	}
	return waitHealthy(ctx, runner, plan, options, filepath.Join(options.RunDirectory, "current"))
}

func waitHealthy(ctx context.Context, runner Runner, plan Plan, options BackendOptions, expectedDirectory string) error {
	deadline := time.Now().Add(options.HealthTimeout)
	var lastErr error
	for {
		if err := checkHealthOnce(ctx, runner, plan, options, expectedDirectory); err == nil {
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

func checkHealthOnce(ctx context.Context, runner Runner, plan Plan, options BackendOptions, expectedDirectory string) error {
	output, err := runner.Output(ctx, options.SystemctlBinary, "show", options.ServiceName, "--property=ActiveState", "--property=MainPID")
	if err != nil {
		return err
	}
	properties := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			properties[key] = value
		}
	}
	pid, _ := strconv.Atoi(properties["MainPID"])
	if properties["ActiveState"] != "active" || pid <= 0 {
		return fmt.Errorf("systemd sing-box service is not active")
	}
	if _, err := runner.Output(ctx, "ip", "-json", "link", "show", "dev", plan.Resources.TunInterface); err != nil {
		return fmt.Errorf("TUN interface is not ready: %w", err)
	}
	if _, err := runner.Output(ctx, options.NFTBinary, "-j", "list", "table", "inet", "steer"); err != nil {
		return fmt.Errorf("Steer nftables DNS shim is not ready: %w", err)
	}
	if err := options.CheckListeners([]int{plan.Resources.DNSPort, plan.Resources.DNSPort6}); err != nil {
		return err
	}
	if expectedDirectory != "" {
		currentInfo, err := os.Stat(filepath.Join(options.RunDirectory, "current"))
		if err != nil {
			return fmt.Errorf("stat current generation: %w", err)
		}
		expectedInfo, err := os.Stat(expectedDirectory)
		if err != nil {
			return fmt.Errorf("stat expected generation: %w", err)
		}
		if !os.SameFile(currentInfo, expectedInfo) {
			return fmt.Errorf("active generation does not match the candidate")
		}
	}
	return nil
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

func ReadStatus(ctx context.Context, runner Runner, options BackendOptions) Status {
	options = normalizeBackendOptions(options)
	status := Status{}
	if file, err := os.Open(filepath.Join(options.RunDirectory, "last-apply.json")); err == nil {
		var record coreapply.Record
		if json.NewDecoder(file).Decode(&record) == nil && record.Sequence != "" {
			status.LastApply = &record
		}
		file.Close()
	}
	plan, err := readCurrentPlan(options.RunDirectory)
	if err == nil && checkHealthOnce(ctx, runner, plan, options, filepath.Join(options.RunDirectory, "current")) == nil {
		status.Healthy = true
	}
	return status
}
