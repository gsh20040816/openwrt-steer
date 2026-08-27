// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"

	"github.com/gsh20040816/steer/go/internal/geodata"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

func runProbe(args []string) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	configPath := flags.String("config", "/etc/steer/config.json", "canonical JSON configuration file")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	kind := flags.String("kind", "direct", "probe kind: direct, proxy or speedtest")
	nodeID := flags.String("node", "", "run a temporary test through this node")
	routeID := flags.String("route", "", "run a temporary test through this route and its detour chain")
	download := flags.Bool("download", false, "download the complete speed-test response")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("probe accepts flags only")
	}
	if *nodeID != "" && *routeID != "" {
		return errors.New("--node and --route are mutually exclusive")
	}
	if *nodeID != "" || *routeID != "" {
		if *kind != "speedtest" {
			return errors.New("--node and --route require --kind speedtest")
		}
		var report linuxplatform.TestReport
		var err error
		if *nodeID != "" {
			report, err = linuxplatform.SpeedTestNode(context.Background(), *configPath, *stateDirectory, *singBoxPath, *nodeID, *download)
		} else {
			report, err = linuxplatform.SpeedTestRoute(context.Background(), *configPath, *stateDirectory, *singBoxPath, *routeID, *download)
		}
		if err != nil {
			return err
		}
		writeJSON(report)
		if !report.OK {
			return errors.New("HTTPS test failed")
		}
		return nil
	}
	report, err := linuxplatform.ProbeOverviewWithState(context.Background(), *configPath, *runDirectory, *stateDirectory, *kind, nil)
	if err != nil {
		return err
	}
	writeJSON(report)
	if !report.OK {
		return errors.New("one or more HTTPS probes failed")
	}
	return nil
}

func runGeoCatalog(args []string) error {
	flags := flag.NewFlagSet("geo-catalog", flag.ContinueOnError)
	kind := flags.String("kind", "", "geosite or geoip")
	seedDirectory := flags.String("seed-dir", "/usr/share/steer/geodata-seed", "package-owned Geo seed directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("geo-catalog accepts flags only")
	}
	names, err := geodata.Catalog(*seedDirectory, *kind)
	if err != nil {
		return err
	}
	writeJSON(struct {
		Kind  string   `json:"kind"`
		Names []string `json:"names"`
	}{*kind, names})
	return nil
}
