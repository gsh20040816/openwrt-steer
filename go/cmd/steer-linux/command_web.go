// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
