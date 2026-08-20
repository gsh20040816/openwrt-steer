// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/capability"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/openwrt"
)

var version = "development"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
	case "validate", "compile", "compile-sing-box", "plan", "capabilities":
		return runIntentCommand(args[0], args[1:])
	default:
		return usage()
	}
}

func runIntentCommand(command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := flags.String("config", "/etc/config/steer", "UCI configuration file")
	currentPath := flags.String("current-plan", "", "current plan JSON for semantic diff")
	singBoxPath := flags.String("sing-box", "/usr/bin/sing-box", "sing-box binary")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "generated state directory")
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
	return errors.New("usage: steer-openwrt version|validate|compile|compile-sing-box|plan|capabilities [flags]")
}
