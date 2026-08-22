// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	coreapply "github.com/gsh20040816/openwrt-steer/go/internal/apply"
	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
	"github.com/gsh20040816/openwrt-steer/go/internal/platform/openwrt"
)

var version = "development"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if len(os.Args) < 2 || os.Args[1] != "apply" {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "version":
		fmt.Println(version)
		return nil
	case "validate":
		return runValidate(args[1:])
	case "apply":
		return runApply(args[1:])
	case "health":
		return runHealth(args[1:])
	case "status":
		return runStatus(args[1:])
	case "probe":
		return runProbe(args[1:])
	case "subscription":
		return runSubscription(args[1:])
	case "geo-catalog":
		return runGeoCatalog(args[1:])
	case "cleanup":
		return runCleanup(args[1:])
	case "_start":
		return runServiceStart(args[1:])
	default:
		return usage()
	}
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("validate accepts flags only")
	}
	value, decodeValidation := loadIntent(*configPath)
	if !decodeValidation.OK {
		writeJSON(decodeValidation)
		return errors.New("configuration decode failed")
	}
	validation := model.Validate(value)
	writeJSON(validation)
	if !validation.OK {
		return errors.New("configuration validation failed")
	}
	return nil
}

func runApply(args []string) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	initScript := flags.String("init", "/etc/init.d/steer", "procd init script")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("apply accepts flags only")
	}
	return runLockedApply(*runDirectory, func() (coreapply.Result, error) {
		value, decodeValidation := loadIntent(*configPath)
		if !decodeValidation.OK {
			return coreapply.Result{Validation: &decodeValidation}, errors.New("configuration decode failed")
		}
		backend := openwrt.NewBackend(openwrt.ExecRunner{}, value, openwrt.BackendOptions{
			RunDirectory: *runDirectory, StateDirectory: *stateDirectory, SingBoxBinary: *singBoxPath,
			NFTBinary: *nftBinary, InitScript: *initScript,
		})
		return coreapply.Run(context.Background(), value, backend.CompilerOptions(), backend)
	})
}

func runLockedApply(runDirectory string, operation func() (coreapply.Result, error)) error {
	lock, err := acquireApplyLock(runDirectory)
	if err != nil {
		return err
	}
	defer lock.Close()
	result, err := operation()
	if err != nil {
		result.OK = false
		result.Error = err.Error()
	}
	if recordErr := writeApplyRecord(runDirectory, coreapply.Record{Sequence: strconv.FormatInt(time.Now().UnixNano(), 10), Result: result}); recordErr != nil {
		return recordErr
	}
	writeJSON(result)
	return err
}

func acquireApplyLock(runDirectory string) (*os.File, error) {
	if err := os.MkdirAll(runDirectory, 0o755); err != nil {
		return nil, fmt.Errorf("create runtime directory for Apply lock: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(runDirectory, "apply.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Apply lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return nil, fmt.Errorf("lock Apply transaction: %w", err)
	}
	return lock, nil
}

func runServiceStart(args []string) error {
	flags := flag.NewFlagSet("_start", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("_start accepts flags only")
	}
	lock, err := acquireApplyLock(*runDirectory)
	if err != nil {
		return err
	}
	defer lock.Close()
	value, decodeValidation := loadIntent(*configPath)
	if !decodeValidation.OK {
		return errors.New(decodeValidation.Errors[0].Message)
	}
	validation := model.Validate(value)
	if !validation.OK {
		issue := validation.Errors[0]
		return fmt.Errorf("configuration validation failed at %s %q option %q: %s", issue.ObjectType, issue.ObjectID, issue.Option, issue.Message)
	}
	if !value.Main.Enabled {
		return errors.New("cannot start a disabled configuration")
	}
	backend := openwrt.NewBackend(openwrt.ExecRunner{}, value, openwrt.BackendOptions{
		RunDirectory: *runDirectory, StateDirectory: *stateDirectory, SingBoxBinary: *singBoxPath, NFTBinary: *nftBinary,
	})
	compiled := compiler.Compile(value, backend.CompilerOptions())
	candidate, err := backend.Prepare(context.Background(), value, compiled)
	if err != nil {
		return err
	}
	if err := backend.ActivateForServiceStart(context.Background(), candidate); err != nil {
		return err
	}
	return backend.Finalize(context.Background(), candidate)
}

func runStatus(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("status accepts flags only")
	}
	writeJSON(openwrt.ReadStatus(context.Background(), openwrt.ExecRunner{}, *runDirectory, *nftBinary))
	return nil
}

func runHealth(args []string) error {
	flags := flag.NewFlagSet("health", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	timeout := flags.Duration("timeout", 10*time.Second, "local readiness deadline")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("health accepts flags only")
	}
	return openwrt.WaitCurrentHealthy(context.Background(), openwrt.ExecRunner{}, *runDirectory, *nftBinary, *timeout)
}

func runProbe(args []string) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
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
		var report openwrt.TestReport
		var err error
		if *nodeID != "" {
			report, err = openwrt.SpeedTestNode(context.Background(), *configPath, *stateDirectory, *singBoxPath, *nodeID, *download)
		} else {
			report, err = openwrt.SpeedTestRoute(context.Background(), *configPath, *stateDirectory, *singBoxPath, *routeID, *download)
		}
		if err != nil {
			scope, objectID := "nodes", *nodeID
			if *routeID != "" {
				scope, objectID = "routes", *routeID
			}
			testKind := "connect"
			if *download {
				testKind = "download"
			}
			writeJSON(openwrt.TestReport{Scope: scope, ObjectID: objectID, Kind: testKind, Error: err.Error(), TestedAt: time.Now().UTC(), Results: []openwrt.TestResult{}})
			return err
		}
		writeJSON(report)
		if !report.OK {
			return errors.New("HTTPS test failed")
		}
		return nil
	}
	report, err := openwrt.ProbeCurrentWithState(context.Background(), *runDirectory, *stateDirectory, *kind, nil)
	if err != nil {
		writeJSON(openwrt.TestReport{Scope: "overview", Kind: *kind, Error: err.Error(), TestedAt: time.Now().UTC(), Results: []openwrt.TestResult{}})
		return err
	}
	writeJSON(report)
	if !report.OK {
		return errors.New("one or more HTTPS probes failed")
	}
	return nil
}

func runSubscription(args []string) error {
	if len(args) == 0 || (args[0] != "update" && args[0] != "status" && args[0] != "clean") {
		return errors.New("usage: steer subscription update|status [--id ID] | clean --id ID --node NODE_ID")
	}
	command := args[0]
	flags := flag.NewFlagSet("subscription", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "subscription snapshot directory")
	id := flags.String("id", "", "only update this subscription ID")
	nodeID := flags.String("node", "", "subscription node ID for clean")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("subscription subcommands accept flags only")
	}
	if command == "status" {
		statuses, err := openwrt.ReadSubscriptionStatus(*configPath, *stateDirectory)
		if err != nil {
			return err
		}
		writeJSON(struct {
			OK            bool                         `json:"ok"`
			Subscriptions []openwrt.SubscriptionStatus `json:"subscriptions"`
		}{true, statuses})
		return nil
	}
	if command == "clean" {
		if *id == "" || *nodeID == "" {
			return errors.New("subscription clean requires --id and --node")
		}
		snapshot, err := openwrt.CleanSubscriptionNode(*configPath, *stateDirectory, *id, *nodeID)
		if err != nil {
			return err
		}
		writeJSON(struct {
			OK       bool                         `json:"ok"`
			Snapshot openwrt.SubscriptionSnapshot `json:"snapshot"`
		}{true, snapshot})
		return nil
	}
	snapshots, err := openwrt.UpdateConfiguredSubscriptions(context.Background(), &http.Client{Timeout: 30 * time.Second}, *configPath, *stateDirectory, *id)
	if err != nil {
		return err
	}
	writeJSON(struct {
		OK        bool                           `json:"ok"`
		Snapshots []openwrt.SubscriptionSnapshot `json:"snapshots"`
	}{true, snapshots})
	return nil
}

func runGeoCatalog(args []string) error {
	flags := flag.NewFlagSet("geo-catalog", flag.ContinueOnError)
	kind := flags.String("kind", "", "geosite or geoip")
	seedDirectory := flags.String("seed-dir", "/usr/share/steer/geodata-seed", "package-owned Geo seed directory")
	geoViewBinary := flags.String("geoview", "/usr/bin/geoview", "geoview binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("geo-catalog accepts flags only")
	}
	names, err := openwrt.GeoCatalog(context.Background(), openwrt.ExecRunner{}, *kind, *seedDirectory, *geoViewBinary)
	if err != nil {
		return err
	}
	writeJSON(struct {
		Kind  string   `json:"kind"`
		Names []string `json:"names"`
	}{*kind, names})
	return nil
}

func runCleanup(args []string) error {
	flags := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("cleanup accepts flags only")
	}
	var plan openwrt.Plan
	if err := readJSON(filepath.Join(*runDirectory, "current", "platform.json"), &plan); err != nil {
		return err
	}
	return openwrt.CleanupPlatform(context.Background(), openwrt.ExecRunner{}, plan, *nftBinary)
}

func writeApplyRecord(runDirectory string, record coreapply.Record) error {
	if err := os.MkdirAll(runDirectory, 0o755); err != nil {
		return fmt.Errorf("create runtime directory for Apply result: %w", err)
	}
	temporary, err := os.CreateTemp(runDirectory, ".last-apply.*")
	if err != nil {
		return fmt.Errorf("create Apply result: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
		temporary.Close()
		return fmt.Errorf("encode Apply result: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync Apply result: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Apply result: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(runDirectory, "last-apply.json")); err != nil {
		return fmt.Errorf("publish Apply result: %w", err)
	}
	return nil
}

func readJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func loadIntent(path string) (model.Intent, model.Validation) {
	file, err := os.Open(path)
	if err != nil {
		return model.Intent{}, decodeFailure(fmt.Sprintf("open configuration: %v", err))
	}
	defer file.Close()
	value, err := openwrt.Decode(file)
	if err != nil {
		return model.Intent{}, decodeFailure(err.Error())
	}
	return value, model.Validation{OK: true, Errors: []model.Issue{}, Warnings: []model.Issue{}}
}

func decodeFailure(message string) model.Validation {
	return model.Validation{Errors: []model.Issue{{Code: "DECODE_FAILED", ObjectType: "uci", Message: message}}, Warnings: []model.Issue{}}
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func usage() error {
	return errors.New("usage: steer version|validate|apply|health|status|probe|subscription|geo-catalog|cleanup [flags]")
}
