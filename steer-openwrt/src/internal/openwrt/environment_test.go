// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type fakeRunner map[string]string

func (runner fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	value, exists := runner[key]
	if !exists {
		return nil, fmt.Errorf("unexpected command: %s", key)
	}
	if strings.HasPrefix(value, "ERROR:") {
		return nil, fmt.Errorf("%s", strings.TrimPrefix(value, "ERROR:"))
	}
	return []byte(value), nil
}

func validEnvironmentRunner() fakeRunner {
	return fakeRunner{
		`ubus call uci get {"config":"firewall","type":"zone"}`: `{"values":{"cfg":{"name":"lan","network":["lan"],"device":["eth2"]}}}`,
		"ubus call network.interface.lan status":                `{"up":true,"device":"br-lan","l3_device":"br-lan"}`,
		"ip -json -4 route show default":                        `[{"dst":"default","dev":"pppoe-wan"}]`,
		"ip -json -6 route show default":                        `[{"dst":"default","dev":"pppoe-wan"}]`,
	}
}

func TestResolveStrictEnvironment(t *testing.T) {
	environment, err := ResolveEnvironment(context.Background(), validEnvironmentRunner(), []string{"lan"})
	if err != nil {
		t.Fatal(err)
	}
	if environment.WANDevice != "pppoe-wan" || strings.Join(environment.ManagedDevices, ",") != "br-lan,eth2" {
		t.Fatalf("unexpected environment: %#v", environment)
	}
}

func TestResolveRejectsMultiWAN(t *testing.T) {
	runner := validEnvironmentRunner()
	runner["ip -json -6 route show default"] = `[{"dst":"default","dev":"wan6"}]`
	if _, err := ResolveEnvironment(context.Background(), runner, []string{"lan"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveRejectsMissingOrDownZone(t *testing.T) {
	if _, err := ResolveEnvironment(context.Background(), validEnvironmentRunner(), []string{"guest"}); err == nil {
		t.Fatal("missing zone accepted")
	}
	runner := validEnvironmentRunner()
	runner["ubus call network.interface.lan status"] = `{"up":false,"device":"br-lan"}`
	if _, err := ResolveEnvironment(context.Background(), runner, []string{"lan"}); err == nil {
		t.Fatal("down managed network accepted")
	}
}
