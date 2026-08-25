// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gsh20040816/steer/go/internal/geodata"
	macosplatform "github.com/gsh20040816/steer/go/internal/platform/macos"
)

func runProbe(args []string, stdoutWriter interface{ Write([]byte) (int, error) }) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	configPath := flags.String("config", defaultConfigPath, "canonical JSON configuration")
	singBoxPath := flags.String("sing-box", defaultSingBoxBinary, "sing-box executable")
	kind := flags.String("kind", "direct", "direct, proxy or speedtest")
	nodeID := flags.String("node", "", "test one node")
	routeID := flags.String("route", "", "test one route chain")
	download := flags.Bool("download", false, "download the complete response")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || (*nodeID != "" && *routeID != "") {
		return errors.New("probe accepts flags only and node/route are mutually exclusive")
	}
	var report macosplatform.TestReport
	var err error
	if *nodeID != "" {
		report, err = macosplatform.SpeedTestNode(context.Background(), *configPath, *singBoxPath, *nodeID, *download)
	} else if *routeID != "" {
		report, err = macosplatform.SpeedTestRoute(context.Background(), *configPath, *singBoxPath, *routeID, *download)
	} else {
		report, err = macosplatform.ProbeCurrent(context.Background(), *configPath, *kind, nil)
	}
	if err != nil {
		return err
	}
	if err := writeJSON(stdoutWriter, report); err != nil {
		return err
	}
	if !report.OK {
		return errors.New("HTTPS probe failed")
	}
	return nil
}

func runGeoCatalog(args []string, stdoutWriter interface{ Write([]byte) (int, error) }) error {
	flags := flag.NewFlagSet("geo-catalog", flag.ContinueOnError)
	kind := flags.String("kind", "", "geosite or geoip")
	seedDirectory := flags.String("seed-dir", defaultGeoDataDir, "package-owned Geo seed")
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
	return writeJSON(stdoutWriter, struct {
		Kind  string   `json:"kind"`
		Names []string `json:"names"`
	}{*kind, names})
}

func runSubscription(args []string, stdoutWriter interface{ Write([]byte) (int, error) }) error {
	if len(args) == 0 || (args[0] != "update" && args[0] != "status" && args[0] != "clean") {
		return errors.New("usage: steer-macos subscription update|status|clean [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet("subscription", flag.ContinueOnError)
	configPath := flags.String("config", defaultConfigPath, "canonical JSON configuration")
	stateDirectory := flags.String("state-dir", defaultStateDirectory, "subscription state directory")
	runDirectory := flags.String("run-dir", defaultRunDirectory, "operation lock directory")
	id := flags.String("id", "", "subscription ID")
	nodeID := flags.String("node", "", "stale node ID")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("subscription subcommands accept flags only")
	}
	if command == "status" {
		statuses, err := macosplatform.ReadSubscriptionStatus(*configPath, *stateDirectory)
		if err != nil {
			return err
		}
		return writeJSON(stdoutWriter, struct {
			OK            bool                               `json:"ok"`
			Subscriptions []macosplatform.SubscriptionStatus `json:"subscriptions"`
		}{true, statuses})
	}
	lock, err := acquireLock(*runDirectory)
	if err != nil {
		return err
	}
	defer lock.Close()
	if command == "clean" {
		if *id == "" || *nodeID == "" {
			return errors.New("subscription clean requires --id and --node")
		}
		snapshot, err := macosplatform.CleanSubscriptionNode(*configPath, *stateDirectory, *id, *nodeID)
		if err != nil {
			return err
		}
		if os.Geteuid() == 0 {
			adminGID, lookupErr := lookupAdminGID()
			if lookupErr != nil {
				return lookupErr
			}
			if err := setControlConfigurationPermissions(*configPath, adminGID); err != nil {
				return err
			}
			if err := setControlStatePermissions(filepath.Join(*stateDirectory, "subscriptions", snapshot.SubscriptionID+".json"), adminGID); err != nil {
				return err
			}
		}
		return writeJSON(stdoutWriter, struct {
			OK       bool                               `json:"ok"`
			Snapshot macosplatform.SubscriptionSnapshot `json:"snapshot"`
		}{true, snapshot})
	}
	snapshots, err := macosplatform.UpdateConfiguredSubscriptions(context.Background(), &http.Client{Timeout: 30 * time.Second}, *configPath, *stateDirectory, *id)
	if err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		adminGID, lookupErr := lookupAdminGID()
		if lookupErr != nil {
			return lookupErr
		}
		if err := setControlConfigurationPermissions(*configPath, adminGID); err != nil {
			return err
		}
		for _, snapshot := range snapshots {
			if err := setControlStatePermissions(filepath.Join(*stateDirectory, "subscriptions", snapshot.SubscriptionID+".json"), adminGID); err != nil {
				return err
			}
		}
	}
	return writeJSON(stdoutWriter, struct {
		OK        bool                                 `json:"ok"`
		Snapshots []macosplatform.SubscriptionSnapshot `json:"snapshots"`
	}{true, snapshots})
}
