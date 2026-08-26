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

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/generation"
	model "github.com/gsh20040816/steer/go/internal/intent"
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

func TestStatusSeparatesActiveIdentityFromLastApply(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	runDirectory := t.TempDir()
	intent := model.Intent{Main: model.Main{Enabled: true}}
	candidate, err := generation.Create(filepath.Join(runDirectory, "generations"), intent, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(runDirectory, "current")
	if err := os.Symlink(candidate.Directory, current); err != nil {
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
	if !status.Healthy || status.Generation != filepath.Base(candidate.Directory) || status.IntentDigest != compiler.IntentDigest(intent) || status.RuntimeDigest == "" || status.LastApply == nil || status.LastApply.Sequence != "1" {
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
	if len(fields) != 5 || fields["healthy"] != true || fields["generation"] == nil || fields["intent_digest"] == nil || fields["runtime_digest"] == nil || fields["last_apply"] == nil {
		t.Fatalf("public status lifecycle facts drifted: %s", encoded)
	}
}
