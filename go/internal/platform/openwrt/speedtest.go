// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/go/internal/generation"
	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
	"github.com/gsh20040816/openwrt-steer/go/internal/probe"
)

type TestReport = probe.Report
type TestResult = probe.Result

func ProbeCurrent(ctx context.Context, runDirectory, kind string, client *http.Client) (TestReport, error) {
	return probeCurrent(ctx, runDirectory, "", kind, client)
}

func ProbeCurrentWithState(ctx context.Context, runDirectory, stateDirectory, kind string, client *http.Client) (TestReport, error) {
	return probeCurrent(ctx, runDirectory, stateDirectory, kind, client)
}

func probeCurrent(ctx context.Context, runDirectory, stateDirectory, kind string, client *http.Client) (TestReport, error) {
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	currentIntent, err := generation.ReadIntent(filepath.Join(runDirectory, "current"))
	if err != nil {
		return TestReport{}, err
	}
	target, download := "", false
	switch kind {
	case "direct":
		target = currentIntent.Main.ProbeDirectURL
	case "proxy":
		target = currentIntent.Main.ProbeProxyURL
	case "speedtest":
		target, download = currentIntent.Main.SpeedtestProxyURL, true
	default:
		return TestReport{}, fmt.Errorf("unsupported probe kind %q", kind)
	}
	if target == "" {
		return TestReport{}, fmt.Errorf("current intent has no %s HTTPS probe", kind)
	}
	if client == nil {
		client = probe.HTTPClient(nil, download)
	}
	report := probe.Run(ctx, client, "overview", "", kind, target, download)
	if stateDirectory != "" {
		if err := saveTestReport(stateDirectory, report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func SpeedTestNode(ctx context.Context, configPath, stateDirectory, singBoxPath, nodeID string, download bool) (TestReport, error) {
	value, err := readTestIntent(configPath, "node test")
	if err != nil {
		return TestReport{}, err
	}
	var node model.Node
	found := false
	for _, candidate := range value.Nodes {
		if candidate.ID == nodeID {
			node, found = candidate, true
			break
		}
	}
	if !found || !node.Enabled {
		return TestReport{}, fmt.Errorf("enabled node %q was not found", nodeID)
	}
	target, kind := value.Main.ProbeProxyURL, "connect"
	if download {
		target, kind = value.Main.SpeedtestProxyURL, "download"
	}
	report, err := probe.RunTemporary(ctx, singBoxPath, []any{compiler.CompileNodeOutbound(node)}, compiler.NodeOutboundTag(node.ID), "nodes", node.ID, kind, target, download)
	if err != nil {
		return TestReport{}, err
	}
	if err := saveTestReport(stateDirectory, report); err != nil {
		return report, err
	}
	return report, nil
}

func SpeedTestRoute(ctx context.Context, configPath, stateDirectory, singBoxPath, routeID string, download bool) (TestReport, error) {
	value, err := readTestIntent(configPath, "route test")
	if err != nil {
		return TestReport{}, err
	}
	var route model.Route
	found := false
	for _, candidate := range value.Routes {
		if candidate.ID == routeID {
			route, found = candidate, true
			break
		}
	}
	if !found || !route.Enabled || route.Kind != "single" {
		return TestReport{}, fmt.Errorf("enabled single-node route %q was not found", routeID)
	}
	outbounds := compiler.CompileRouteChainOutbounds(value, route.ID)
	if len(outbounds) == 0 {
		return TestReport{}, fmt.Errorf("compiled route test has no route-chain outbounds")
	}
	target, kind := value.Main.ProbeProxyURL, "connect"
	if download {
		target, kind = value.Main.SpeedtestProxyURL, "download"
	}
	report, err := probe.RunTemporary(ctx, singBoxPath, outbounds, compiler.RouteOutboundTag(route.ID), "routes", route.ID, kind, target, download)
	if err != nil {
		return TestReport{}, err
	}
	if err := saveTestReport(stateDirectory, report); err != nil {
		return report, err
	}
	return report, nil
}

func readTestIntent(configPath, operation string) (model.Intent, error) {
	config, err := os.ReadFile(configPath)
	if err != nil {
		return model.Intent{}, fmt.Errorf("read UCI for %s: %w", operation, err)
	}
	value, err := DecodeBytes(config)
	if err != nil {
		return model.Intent{}, err
	}
	validation := model.Validate(value)
	if !validation.OK {
		return model.Intent{}, ValidationError{Validation: validation}
	}
	return value, nil
}

func saveTestReport(stateDirectory string, report TestReport) error {
	if stateDirectory == "" {
		stateDirectory = "/var/lib/steer"
	}
	directory := filepath.Join(stateDirectory, "logs", "tests", report.Scope)
	if report.ObjectID != "" {
		directory = filepath.Join(directory, report.ObjectID)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create test log directory: %w", err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode test report: %w", err)
	}
	if err := atomicWrite(filepath.Join(directory, report.Kind+".json"), append(encoded, '\n')); err != nil {
		return fmt.Errorf("save test report: %w", err)
	}
	return nil
}
