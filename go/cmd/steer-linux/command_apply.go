// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"syscall"
	"time"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

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
		settings, _, err := (linuxplatform.SettingsStore{Path: *platformPath}).Load()
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
			loadErr = linuxplatform.CleanupPlatform(context.Background(), linuxplatform.ExecRunner{}, *nftBinary)
		}
		if loadErr == nil && !disabled {
			settings, _, settingsErr := (linuxplatform.SettingsStore{Path: *platformPath}).Load()
			if settingsErr != nil {
				loadErr = settingsErr
			} else {
				options.GeoSitePath, options.GeoIPPath = settings.GeoSitePath, settings.GeoIPPath
			}
		}
		if loadErr == nil && !disabled {
			backend := linuxplatform.NewBackend(linuxplatform.ExecRunner{}, value, options)
			compiled, compileErr := compiler.Compile(value, backend.CompilerOptions())
			if compileErr != nil {
				loadErr = compileErr
			} else {
				candidate, prepareErr := backend.Prepare(context.Background(), value, compiled)
				if prepareErr == nil {
					prepareErr = backend.ActivateForServiceStart(context.Background(), candidate)
				}
				if prepareErr == nil {
					prepareErr = backend.Finalize(context.Background(), candidate)
				}
				loadErr = prepareErr
			}
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
	return nil
}

func loadIntent(path string) (intent.Intent, intent.Validation, error) {
	value, _, err := (linuxplatform.IntentStore{Path: path}).Load()
	if err != nil {
		return intent.Intent{}, intent.Validation{Errors: []intent.Issue{{Code: "DECODE_FAILED", ObjectType: "json", Message: err.Error()}}}, err
	}
	return value, linuxplatform.Validate(value), nil
}
