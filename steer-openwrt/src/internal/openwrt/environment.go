// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type Runner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

type Environment struct {
	ManagedDevices []string `json:"managed_devices"`
	WANDevice      string   `json:"wan_device"`
}

func ResolveEnvironment(ctx context.Context, runner Runner, zones []string) (Environment, error) {
	devices, err := resolveManagedDevices(ctx, runner, zones)
	if err != nil {
		return Environment{}, err
	}
	wan, err := resolveWANDevice(ctx, runner)
	if err != nil {
		return Environment{}, err
	}
	for _, device := range devices {
		if device == wan {
			return Environment{}, fmt.Errorf("managed ingress device %q is also the unique default WAN", device)
		}
	}
	return Environment{ManagedDevices: devices, WANDevice: wan}, nil
}

func resolveManagedDevices(ctx context.Context, runner Runner, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, fmt.Errorf("no managed firewall zone requested")
	}
	output, err := runner.Output(ctx, "ubus", "call", "uci", "get", `{"config":"firewall","type":"zone"}`)
	if err != nil {
		return nil, fmt.Errorf("read firewall zones: %w", err)
	}
	var response struct {
		Values map[string]map[string]any `json:"values"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("decode firewall zones: %w", err)
	}
	zones := map[string]map[string]any{}
	for _, value := range response.Values {
		if name, ok := value["name"].(string); ok && name != "" {
			zones[name] = value
		}
	}
	seen := map[string]bool{}
	var devices []string
	for _, name := range requested {
		zone, exists := zones[name]
		if !exists {
			return nil, fmt.Errorf("managed firewall zone %q does not exist", name)
		}
		for _, device := range stringValues(zone["device"]) {
			if device != "" && !seen[device] {
				seen[device] = true
				devices = append(devices, device)
			}
		}
		for _, network := range stringValues(zone["network"]) {
			statusOutput, statusErr := runner.Output(ctx, "ubus", "call", "network.interface."+network, "status")
			if statusErr != nil {
				return nil, fmt.Errorf("resolve network %q in zone %q: %w", network, name, statusErr)
			}
			var status struct {
				Up       bool   `json:"up"`
				Device   string `json:"device"`
				L3Device string `json:"l3_device"`
			}
			if err := json.Unmarshal(statusOutput, &status); err != nil {
				return nil, fmt.Errorf("decode network %q status: %w", network, err)
			}
			device := status.L3Device
			if device == "" {
				device = status.Device
			}
			if !status.Up || device == "" {
				return nil, fmt.Errorf("network %q in managed zone %q is not up with a concrete device", network, name)
			}
			if !seen[device] {
				seen[device] = true
				devices = append(devices, device)
			}
		}
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("managed firewall zones resolve to no devices")
	}
	sort.Strings(devices)
	for _, device := range devices {
		if !validDevice.MatchString(device) {
			return nil, fmt.Errorf("firewall resolved unsafe device %q", device)
		}
	}
	return devices, nil
}

func resolveWANDevice(ctx context.Context, runner Runner) (string, error) {
	devices := map[string]bool{}
	for _, family := range []string{"-4", "-6"} {
		output, err := runner.Output(ctx, "ip", "-json", family, "route", "show", "default")
		if err != nil {
			return "", fmt.Errorf("read %s default routes: %w", family, err)
		}
		var routes []struct {
			Device string `json:"dev"`
			Type   string `json:"type"`
		}
		if err := json.Unmarshal(output, &routes); err != nil {
			return "", fmt.Errorf("decode %s default routes: %w", family, err)
		}
		for _, route := range routes {
			if route.Device != "" && (route.Type == "" || route.Type == "unicast") {
				devices[route.Device] = true
			}
		}
	}
	if len(devices) != 1 {
		return "", fmt.Errorf("default WAN is ambiguous: resolved devices %v", sortedSet(devices))
	}
	for device := range devices {
		if !validDevice.MatchString(device) {
			return "", fmt.Errorf("default WAN resolved unsafe device %q", device)
		}
		return device, nil
	}
	panic("unreachable")
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		return strings.Fields(typed)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	case []string:
		return append([]string(nil), typed...)
	default:
		return nil
	}
}
func sortedSet(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
