// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/capability"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/openwrt"
)

var version = "development"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if len(os.Args) < 2 || (os.Args[1] != "apply" && os.Args[1] != "rollback") {
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
	case "cleanup":
		return runCleanup(args[1:])
	case "apply":
		return runApply(args[1:])
	case "rollback":
		return runRollback(args[1:])
	case "probe":
		return runProbe(args[1:])
	case "health":
		return runHealth(args[1:])
	case "status":
		return runStatus(args[1:])
	case "geo-catalog":
		return runGeoCatalog(args[1:])
	case "validate", "compile", "compile-sing-box", "compile-firewall", "plan", "capabilities", "prepare":
		return runIntentCommand(args[0], args[1:])
	default:
		return usage()
	}
}

func runStatus(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("status accepts flags only")
	}
	writeJSON(openwrt.ReadStatus(context.Background(), openwrt.ExecRunner{}, *configPath, *runDirectory, *nftBinary))
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
	options := openwrt.ApplyOptions{
		Prepare:    openwrt.PrepareOptions{ConfigPath: *configPath, RunDirectory: *runDirectory, StateDirectory: *stateDirectory, SingBoxBinary: *singBoxPath, NFTBinary: *nftBinary},
		InitScript: *initScript,
	}
	return runLockedApply(*runDirectory, func() (openwrt.ApplyResult, error) {
		return openwrt.Apply(context.Background(), openwrt.ExecRunner{}, options)
	})
}

func runRollback(args []string) error {
	flags := flag.NewFlagSet("rollback", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	nftBinary := flags.String("nft", "/usr/sbin/nft", "nft binary")
	initScript := flags.String("init", "/etc/init.d/steer", "procd init script")
	backupPath := flags.String("backup", "/var/lib/steer/rollback.uci", "single-use rollback UCI")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("rollback accepts flags only")
	}
	options := openwrt.ApplyOptions{
		Prepare:    openwrt.PrepareOptions{ConfigPath: *configPath, RunDirectory: *runDirectory, StateDirectory: *stateDirectory, SingBoxBinary: *singBoxPath, NFTBinary: *nftBinary},
		InitScript: *initScript,
		BackupPath: *backupPath,
	}
	return runLockedApply(*runDirectory, func() (openwrt.ApplyResult, error) {
		return openwrt.Rollback(context.Background(), openwrt.ExecRunner{}, options)
	})
}

func runLockedApply(runDirectory string, operation func() (openwrt.ApplyResult, error)) error {
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
	if recordErr := writeApplyRecord(runDirectory, openwrt.ApplyRecord{Sequence: strconv.FormatInt(time.Now().UnixNano(), 10), Result: result}); recordErr != nil {
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

func runProbe(args []string) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	kind := flags.String("kind", "direct", "probe kind: direct, proxy or speedtest")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("probe accepts flags only")
	}
	report, err := openwrt.ProbeCurrent(context.Background(), *runDirectory, *kind, nil)
	if err != nil {
		return err
	}
	writeJSON(report)
	if !report.OK {
		return errors.New("one or more HTTPS probes failed")
	}
	return nil
}

func writeApplyRecord(runDirectory string, record openwrt.ApplyRecord) error {
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

func runIntentCommand(command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	currentPath := flags.String("current-plan", "", "current plan JSON for semantic diff")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	activate := flags.Bool("activate", false, "load platform resources and publish the generation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%s accepts flags only", command)
	}
	intent, decodeValidation := loadIntent(*configPath)
	if !decodeValidation.OK {
		writeJSON(decodeValidation)
		return errors.New("configuration decode failed")
	}
	bundle := compiler.CompileWithOptions(intent, compiler.Options{StateDirectory: *stateDirectory})
	if command == "validate" {
		writeJSON(bundle.Validation)
		if !bundle.Validation.OK {
			return errors.New("configuration validation failed")
		}
		return nil
	}
	if !bundle.Validation.OK {
		writeJSON(bundle.Validation)
		return errors.New("configuration validation failed")
	}
	switch command {
	case "compile":
		writeJSON(bundle)
	case "compile-sing-box":
		writeJSON(bundle.SingBox)
	case "compile-firewall":
		firewall, err := openwrt.RenderFirewall(bundle.Plan)
		if err != nil {
			return err
		}
		fmt.Print(firewall)
	case "capabilities":
		report := capability.Inspect(*singBoxPath, bundle.Plan.RequiredCapabilities)
		writeJSON(report)
		if !report.OK {
			return errors.New("sing-box capability check failed")
		}
	case "plan":
		if *currentPath == "" {
			writeJSON(bundle.Plan)
			return nil
		}
		currentFile, err := os.Open(*currentPath)
		if err != nil {
			return fmt.Errorf("open current plan: %w", err)
		}
		defer currentFile.Close()
		var current compiler.Plan
		if err := json.NewDecoder(currentFile).Decode(&current); err != nil {
			return fmt.Errorf("decode current plan: %w", err)
		}
		writeJSON(struct {
			Plan compiler.Plan     `json:"plan"`
			Diff compiler.PlanDiff `json:"diff"`
		}{bundle.Plan, compiler.Diff(current, bundle.Plan)})
	case "prepare":
		generation, err := openwrt.PrepareGeneration(context.Background(), openwrt.ExecRunner{}, openwrt.PrepareOptions{ConfigPath: *configPath, RunDirectory: *runDirectory, StateDirectory: *stateDirectory, SingBoxBinary: *singBoxPath})
		if err != nil {
			return err
		}
		if *activate {
			if err := openwrt.ActivateGeneration(context.Background(), openwrt.ExecRunner{}, generation, *runDirectory, ""); err != nil {
				return err
			}
		}
		writeJSON(generation)
	}
	return nil
}

func runCleanup(args []string) error {
	flags := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	runDirectory := flags.String("run-dir", "/run/steer", "runtime state directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var bundle compiler.Bundle
	if err := readJSON(filepath.Join(*runDirectory, "current", "bundle.json"), &bundle); err != nil {
		return err
	}
	return openwrt.CleanupPlatform(context.Background(), openwrt.ExecRunner{}, bundle.Plan, "")
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
	intent, err := openwrt.Decode(file)
	if err != nil {
		return model.Intent{}, decodeFailure(err.Error())
	}
	return intent, model.Validation{OK: true, Errors: []model.Issue{}, Warnings: []model.Issue{}}
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
	return errors.New("usage: steer version|validate|compile|compile-sing-box|compile-firewall|plan|capabilities|prepare|apply|rollback|probe|health|status|geo-catalog|cleanup [flags]")
}
