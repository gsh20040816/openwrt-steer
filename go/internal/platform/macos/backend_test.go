// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/compiler"
)

type launchdFakeRunner struct {
	loaded bool
}

func (runner *launchdFakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	command := name + " " + strings.Join(args, " ")
	switch {
	case name == "/test/sing-box" && len(args) == 1 && args[0] == "version":
		return []byte("sing-box version 1.14.0-rc.1\nTags: with_quic,with_utls\n"), nil
	case name == "/test/sing-box" && len(args) == 3 && args[0] == "check":
		return nil, nil
	case name == "/test/launchctl" && len(args) >= 2 && args[0] == "print":
		if !runner.loaded {
			return nil, fmt.Errorf("label is unloaded")
		}
		return []byte("state = running\npid = 42\n"), nil
	case name == "/test/launchctl" && len(args) >= 2 && args[0] == "bootout":
		runner.loaded = false
		return nil, nil
	case name == "/test/launchctl" && len(args) >= 3 && args[0] == "bootstrap":
		runner.loaded = true
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", command)
	}
}

func TestBackendUsesLaunchdGenerationLifecycle(t *testing.T) {
	root := t.TempDir()
	value := validIntent()
	runner := &launchdFakeRunner{}
	backend := NewBackend(runner, value, BackendOptions{
		RunDirectory: root + "/run", StateDirectory: root + "/state", SingBoxBinary: "/test/sing-box",
		LaunchctlBinary: "/test/launchctl", LaunchDaemonLabel: DefaultLaunchDaemonLabel,
		LaunchDaemonPlist: root + "/steer.plist", CheckTUN: func([]string) error { return nil },
	})
	compiled := compiler.Compile(value, backend.CompilerOptions())
	candidate, err := backend.Prepare(context.Background(), value, compiled)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Activate(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if err := backend.Healthy(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if err := backend.Finalize(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	current, err := backend.paths().LoadCurrent()
	if err != nil || current.GenerationID == "" || current.Directory == "" {
		t.Fatalf("unexpected current generation: %#v %v", current, err)
	}
}
