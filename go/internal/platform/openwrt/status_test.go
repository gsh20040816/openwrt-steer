// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreapply "github.com/gsh20040816/openwrt-steer/go/internal/apply"
	"github.com/gsh20040816/openwrt-steer/go/internal/generation"
)

type fakeRunner map[string]string

func (runner fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	value, ok := runner[key]
	if !ok {
		return nil, fmt.Errorf("unexpected command: %s", key)
	}
	return []byte(value), nil
}

func TestStatusContainsOnlyHealthAndLastApply(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	runDirectory := t.TempDir()
	current := filepath.Join(runDirectory, "current")
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := Plan{SchemaVersion: 1, Resources: Resources{TunInterface: TunInterface, DNSPort: listener.Addr().(*net.TCPAddr).Port}}
	if err := generation.WriteJSON(filepath.Join(current, "platform.json"), plan); err != nil {
		t.Fatal(err)
	}
	record := coreapply.Record{Sequence: "1", Result: coreapply.Result{OK: true}}
	if err := generation.WriteJSON(filepath.Join(runDirectory, "last-apply.json"), record); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{
		`ubus call service list {"name":"steer"}`: `{"steer":{"instances":{"sing-box":{"running":true,"pid":123}}}}`,
		"ip -json link show dev steer0":           `[]`,
		"/test/nft -j list table inet steer":      `{}`,
	}
	status := ReadStatus(context.Background(), runner, runDirectory, "/test/nft")
	if !status.Healthy || status.LastApply == nil || status.LastApply.Sequence != "1" {
		t.Fatalf("unexpected minimal status: %#v", status)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["healthy"] != true || fields["last_apply"] == nil {
		t.Fatalf("public status contract expanded: %s", encoded)
	}
}
