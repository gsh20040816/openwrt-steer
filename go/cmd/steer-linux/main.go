// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/geodata"
	"github.com/gsh20040816/steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

var version = "development"

const applyLockTimeout = 2 * time.Minute

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
	case "cleanup":
		return runCleanup(args[1:])
	case "geo-catalog":
		return runGeoCatalog(args[1:])
	case "subscription":
		return runSubscription(args[1:])
	case "web":
		return runWeb(args[1:])
	case "web-token":
		return runWebToken(args[1:])
	case "_run":
		return runService(args[1:])
	default:
		return usage()
	}
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/steer/config.json", "canonical JSON configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("validate accepts flags only")
	}
	value, validation, err := loadIntent(*configPath)
	if err != nil {
		writeJSON(validation)
		return err
	}
	validation = linuxplatform.Validate(value)
	writeJSON(validation)
	if !validation.OK {
		return errors.New("configuration validation failed")
	}
	return nil
}

func runApply(args []string) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/steer/config.json", "canonical JSON configuration file")
	platformPath := flags.String("platform", "/etc/steer/platform.json", "Linux platform settings file")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	systemctl := flags.String("systemctl", "/usr/bin/systemctl", "systemctl binary")
	serviceName := flags.String("service", "steer.service", "systemd service name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("apply accepts flags only")
	}
	return runLockedApply(*runDirectory, func() (coreapply.Result, error) {
		value, validation, err := loadIntent(*configPath)
		if err != nil {
			return coreapply.Result{Validation: &validation}, err
		}
		if !validation.OK {
			return coreapply.Result{Validation: &validation}, linuxplatform.ValidationError{Validation: validation}
		}
		settings, _, err := (linuxplatform.PlatformStore{Path: *platformPath}).Load()
		if err != nil {
			return coreapply.Result{}, err
		}
		backend := linuxplatform.NewBackend(linuxplatform.ExecRunner{}, value, linuxplatform.BackendOptions{
			RunDirectory: *runDirectory, StateDirectory: *stateDirectory, SingBoxBinary: *singBoxPath,
			NFTBinary: *nftBinary, SystemctlBinary: *systemctl, ServiceName: *serviceName,
			GeoSitePath: settings.GeoSitePath, GeoIPPath: settings.GeoIPPath,
		})
		return coreapply.Run(context.Background(), value, backend.CompilerOptions(), backend)
	})
}

func runService(args []string) error {
	flags := flag.NewFlagSet("_run", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/steer/config.json", "canonical JSON configuration file")
	platformPath := flags.String("platform", "/etc/steer/platform.json", "Linux platform settings file")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("_run accepts flags only")
	}
	options := linuxplatform.BackendOptions{RunDirectory: *runDirectory, StateDirectory: *stateDirectory, SingBoxBinary: *singBoxPath, NFTBinary: *nftBinary}
	if _, err := os.Stat(filepath.Join(*runDirectory, "current", "sing-box.json")); err != nil {
		lock, lockErr := acquireApplyLock(*runDirectory)
		if lockErr != nil {
			return lockErr
		}
		value, validation, loadErr := loadIntent(*configPath)
		if loadErr == nil {
			validation = linuxplatform.Validate(value)
			if !validation.OK {
				loadErr = linuxplatform.ValidationError{Validation: validation}
			}
		}
		disabled := loadErr == nil && !value.Main.Enabled
		if disabled {
			// Disabled is a valid steady state.  A service enabled in systemd
			// must not turn that state into a restart loop.
			loadErr = linuxplatform.CleanupPlatform(context.Background(), linuxplatform.ExecRunner{}, *nftBinary)
		}
		if loadErr == nil && !disabled {
			settings, _, settingsErr := (linuxplatform.PlatformStore{Path: *platformPath}).Load()
			if settingsErr != nil {
				loadErr = settingsErr
			} else {
				options.GeoSitePath, options.GeoIPPath = settings.GeoSitePath, settings.GeoIPPath
			}
		}
		if loadErr == nil && !disabled {
			backend := linuxplatform.NewBackend(linuxplatform.ExecRunner{}, value, options)
			compiled := compiler.Compile(value, backend.CompilerOptions())
			candidate, prepareErr := backend.Prepare(context.Background(), value, compiled)
			if prepareErr == nil {
				prepareErr = backend.ActivateForServiceStart(context.Background(), candidate)
			}
			if prepareErr == nil {
				prepareErr = backend.Finalize(context.Background(), candidate)
			}
			loadErr = prepareErr
		}
		lock.Close()
		if loadErr != nil {
			return loadErr
		}
		if disabled {
			return nil
		}
	}
	if err := linuxplatform.EnsureCurrentFirewall(context.Background(), linuxplatform.ExecRunner{}, *runDirectory, *nftBinary); err != nil {
		return err
	}
	return syscall.Exec(*singBoxPath, []string{filepath.Base(*singBoxPath), "run", "-c", filepath.Join(*runDirectory, "current", "sing-box.json")}, os.Environ())
}

func runHealth(args []string) error {
	flags := flag.NewFlagSet("health", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	systemctl := flags.String("systemctl", "/usr/bin/systemctl", "systemctl binary")
	serviceName := flags.String("service", "steer.service", "systemd service name")
	timeout := flags.Duration("timeout", 10*time.Second, "local readiness deadline")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("health accepts flags only")
	}
	return linuxplatform.WaitCurrentHealthy(context.Background(), linuxplatform.ExecRunner{}, linuxplatform.BackendOptions{RunDirectory: *runDirectory, NFTBinary: *nftBinary, SystemctlBinary: *systemctl, ServiceName: *serviceName}, *timeout)
}

func runStatus(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	systemctl := flags.String("systemctl", "/usr/bin/systemctl", "systemctl binary")
	serviceName := flags.String("service", "steer.service", "systemd service name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("status accepts flags only")
	}
	writeJSON(linuxplatform.ReadStatus(context.Background(), linuxplatform.ExecRunner{}, linuxplatform.BackendOptions{RunDirectory: *runDirectory, NFTBinary: *nftBinary, SystemctlBinary: *systemctl, ServiceName: *serviceName}))
	return nil
}

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
	report, err := linuxplatform.ProbeCurrentWithState(context.Background(), *runDirectory, *stateDirectory, *kind, nil)
	if err != nil {
		return err
	}
	writeJSON(report)
	if !report.OK {
		return errors.New("one or more HTTPS probes failed")
	}
	return nil
}

func runCleanup(args []string) error {
	flags := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("cleanup accepts flags only")
	}
	if err := linuxplatform.CleanupPlatform(context.Background(), linuxplatform.ExecRunner{}, *nftBinary); err != nil {
		return err
	}
	// Service stop/restart must preserve the current generation. Disable is
	// the only operation that removes it through the Apply backend.
	return nil
}

func runGeoCatalog(args []string) error {
	flags := flag.NewFlagSet("geo-catalog", flag.ContinueOnError)
	kind := flags.String("kind", "", "geosite or geoip")
	platformPath := flags.String("platform", "/etc/steer/platform.json", "Linux platform settings file")
	geoViewBinary := flags.String("geoview", "/usr/bin/geoview", "geoview binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("geo-catalog accepts flags only")
	}
	settings, _, err := (linuxplatform.PlatformStore{Path: *platformPath}).Load()
	if err != nil {
		return err
	}
	path := settings.GeoIPPath
	if *kind == "geosite" {
		path = settings.GeoSitePath
	}
	names, err := geodata.Catalog(context.Background(), linuxplatform.ExecRunner{}, *kind, path, *geoViewBinary)
	if err != nil {
		return err
	}
	writeJSON(struct {
		Kind  string   `json:"kind"`
		Names []string `json:"names"`
	}{*kind, names})
	return nil
}

func runSubscription(args []string) error {
	if len(args) == 0 || (args[0] != "update" && args[0] != "status" && args[0] != "clean") {
		return errors.New("usage: steer-linux subscription update|status|clean [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet("subscription", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/steer/config.json", "canonical JSON configuration file")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "subscription snapshot directory")
	runDirectory := flags.String("run-dir", "/run/steer", "shared operation lock directory")
	id := flags.String("id", "", "only update this subscription ID")
	nodeID := flags.String("node", "", "subscription node ID for clean")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("subscription subcommands accept flags only")
	}
	if command == "status" {
		statuses, err := linuxplatform.ReadSubscriptionStatus(*configPath, *stateDirectory)
		if err != nil {
			return err
		}
		writeJSON(struct {
			OK            bool                               `json:"ok"`
			Subscriptions []linuxplatform.SubscriptionStatus `json:"subscriptions"`
		}{true, statuses})
		return nil
	}
	if command == "clean" {
		if *id == "" || *nodeID == "" {
			return errors.New("subscription clean requires --id and --node")
		}
		var snapshot linuxplatform.SubscriptionSnapshot
		err := withOperationLock(*runDirectory, func() error {
			var err error
			snapshot, err = linuxplatform.CleanSubscriptionNode(*configPath, *stateDirectory, *id, *nodeID)
			return err
		})
		if err != nil {
			return err
		}
		writeJSON(struct {
			OK       bool                               `json:"ok"`
			Snapshot linuxplatform.SubscriptionSnapshot `json:"snapshot"`
		}{true, snapshot})
		return nil
	}
	var snapshots []linuxplatform.SubscriptionSnapshot
	err := withOperationLock(*runDirectory, func() error {
		var err error
		snapshots, err = linuxplatform.UpdateConfiguredSubscriptions(context.Background(), &http.Client{Timeout: 30 * time.Second}, *configPath, *stateDirectory, *id)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(struct {
		OK        bool                                 `json:"ok"`
		Snapshots []linuxplatform.SubscriptionSnapshot `json:"snapshots"`
	}{true, snapshots})
	return nil
}

func runWeb(args []string) error {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/steer/config.json", "canonical JSON configuration file")
	platformPath := flags.String("platform", "/etc/steer/platform.json", "Linux platform settings file")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	listen := flags.String("listen", "127.0.0.1:9080", "loopback listen address")
	tokenPath := flags.String("token", "/var/lib/steer/web.token", "Web bearer token file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("web accepts flags only")
	}
	if !strings.HasPrefix(*listen, "127.0.0.1:") && !strings.HasPrefix(*listen, "[::1]:") {
		return errors.New("web listen address must be loopback")
	}
	return serveWeb(*listen, *tokenPath, *configPath, *platformPath, *runDirectory, *stateDirectory)
}

func runWebToken(args []string) error {
	flags := flag.NewFlagSet("web-token", flag.ContinueOnError)
	path := flags.String("path", "/var/lib/steer/web.token", "Web token file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("web-token accepts flags only")
	}
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return fmt.Errorf("generate Web token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(*path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(*path, []byte(hex.EncodeToString(value)+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(*path, 0o600); err != nil {
		return err
	}
	fmt.Println(hex.EncodeToString(value))
	return nil
}

func loadIntent(path string) (intent.Intent, intent.Validation, error) {
	value, _, err := (linuxplatform.JSONStore{Path: path}).Load()
	if err != nil {
		return intent.Intent{}, intent.Validation{Errors: []intent.Issue{{Code: "DECODE_FAILED", ObjectType: "json", Message: err.Error()}}}, err
	}
	return value, linuxplatform.Validate(value), nil
}

func runLockedApply(runDirectory string, operation func() (coreapply.Result, error)) error {
	lock, err := acquireApplyLock(runDirectory)
	if err != nil {
		return err
	}
	defer lock.Close()
	result, err := runApplyOperation(runDirectory, operation)
	writeJSON(result)
	return err
}

func runLockedApplyResult(runDirectory string, operation func() (coreapply.Result, error)) (coreapply.Result, error) {
	lock, err := acquireApplyLock(runDirectory)
	if err != nil {
		return coreapply.Result{}, err
	}
	defer lock.Close()
	return runApplyOperation(runDirectory, operation)
}

func withOperationLock(runDirectory string, operation func() error) error {
	lock, err := acquireApplyLock(runDirectory)
	if err != nil {
		return err
	}
	defer lock.Close()
	return operation()
}

func runApplyOperation(runDirectory string, operation func() (coreapply.Result, error)) (coreapply.Result, error) {
	result, err := operation()
	return recordApplyResult(runDirectory, result, err)
}

func recordApplyResult(runDirectory string, result coreapply.Result, err error) (coreapply.Result, error) {
	if err != nil {
		result.OK = false
		result.Error = err.Error()
	}
	if recordErr := writeApplyRecord(runDirectory, coreapply.Record{Sequence: strconv.FormatInt(time.Now().UnixNano(), 10), Result: result}); recordErr != nil {
		if err != nil {
			err = errors.Join(err, recordErr)
		} else {
			err = recordErr
		}
		result.OK = false
		result.Error = err.Error()
		return result, err
	}
	return result, err
}

func acquireApplyLock(runDirectory string) (*os.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), applyLockTimeout)
	defer cancel()
	return acquireApplyLockContext(ctx, runDirectory)
}

func acquireApplyLockContext(ctx context.Context, runDirectory string) (*os.File, error) {
	if err := os.MkdirAll(runDirectory, 0o750); err != nil {
		return nil, fmt.Errorf("create runtime directory for Apply lock: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(runDirectory, "operation.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Apply lock: %w", err)
	}
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			lock.Close()
			return nil, fmt.Errorf("lock Apply transaction: %w", err)
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			lock.Close()
			return nil, fmt.Errorf("lock Apply transaction: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func writeApplyRecord(runDirectory string, record coreapply.Record) error {
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return linuxAtomicWrite(filepath.Join(runDirectory, "last-apply.json"), append(encoded, '\n'))
}

func linuxAtomicWrite(path string, content []byte) error {
	// Keep the command package free of a second filesystem implementation.
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".steer.result.*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
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

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func usage() error {
	return errors.New("usage: steer-linux version|validate|apply|health|status|probe|cleanup|geo-catalog|subscription|web|web-token|_run [flags]")
}
