// SPDX-License-Identifier: GPL-3.0-or-later

// steer-macos is the platform-neutral host-side helper used while the native
// macOS app and NetworkExtension targets are being assembled. It deliberately
// performs only canonical JSON validation and deterministic compilation; it
// does not attempt to start a tunnel or access Darwin APIs.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/internal/platform/macos"
)

var version = "development"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	case "validate":
		return runValidate(args[1:], stdout)
	case "compile":
		return runCompile(args[1:], stdout)
	default:
		return usage()
	}
}

func runValidate(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "canonical JSON configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" {
		return errors.New("validate requires --config and accepts flags only")
	}
	value, err := loadIntent(*configPath)
	if err != nil {
		return err
	}
	validation := macos.Validate(value)
	if err := writeJSON(stdout, validation); err != nil {
		return err
	}
	if !validation.OK {
		return errors.New("configuration validation failed")
	}
	return nil
}

func runCompile(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("compile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "canonical JSON configuration")
	stateDirectory := flags.String("state-dir", "", "App Group state directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" || *stateDirectory == "" {
		return errors.New("compile requires --config and --state-dir and accepts flags only")
	}
	value, err := loadIntent(*configPath)
	if err != nil {
		return err
	}
	validation := macos.Validate(value)
	if !validation.OK {
		return errors.New("configuration validation failed")
	}
	bundle, err := compiler.Compile(value, macos.NewPlan(value).CompilerOptions(*stateDirectory))
	if err != nil {
		return err
	}
	return writeJSON(stdout, bundle)
}

func loadIntent(path string) (model.Intent, error) {
	file, err := os.Open(path)
	if err != nil {
		return model.Intent{}, fmt.Errorf("open canonical config: %w", err)
	}
	defer file.Close()
	value, err := model.DecodeJSON(file)
	if err != nil {
		return model.Intent{}, err
	}
	return value, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usage() error {
	return errors.New("usage: steer-macos {version|validate --config PATH|compile --config PATH --state-dir PATH}")
}
