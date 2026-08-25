// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	model "github.com/gsh20040816/steer/go/internal/intent"
	macosplatform "github.com/gsh20040816/steer/go/internal/platform/macos"
)

func TestAuthorizedControlPeer(t *testing.T) {
	if !authorizedControlPeer(0, nil, 80) {
		t.Fatal("root must be authorized")
	}
	if !authorizedControlPeer(501, []uint32{20, 80}, 80) {
		t.Fatal("admin group member must be authorized")
	}
	if authorizedControlPeer(501, []uint32{20}, 80) {
		t.Fatal("non-admin user must not be authorized")
	}
}

func TestControlServiceApplyUsesOnlyStructuredHooks(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "..", "linux", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	written := false
	applied := false
	service := &controlService{
		configPath: "/fixed/config.json",
		adminGID:   80,
		write: func(path string, content []byte, gid int) error {
			if path != "/fixed/config.json" || gid != 80 || string(content) != string(document) {
				t.Fatalf("unexpected structured save input: path=%q gid=%d", path, gid)
			}
			written = true
			return nil
		},
		apply: func(value model.Intent, _ macosplatform.BackendOptions) error {
			if value.Main.SchemaVersion != model.SchemaVersion {
				t.Fatalf("unexpected canonical schema %d", value.Main.SchemaVersion)
			}
			applied = true
			return nil
		},
		status: func(macosplatform.BackendOptions) macosplatform.Status {
			return macosplatform.DefaultStatus()
		},
	}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "apply",
		Document:      string(document),
	})
	if !response.OK || response.Status == nil || !written || !applied {
		t.Fatalf("unexpected response or hook state: response=%+v written=%v applied=%v", response, written, applied)
	}
}

func TestControlServiceRejectsUnrestrictedOperations(t *testing.T) {
	service := &controlService{}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "shell",
		Document:      `{}`,
	})
	if response.OK || response.Error != "unsupported control operation" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestControlServiceRejectsInvalidDocumentBeforeWrite(t *testing.T) {
	service := &controlService{configPath: t.TempDir() + "/config.json"}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "save",
		Document:      `{"main":`,
	})
	if response.OK || !strings.Contains(response.Error, "decode canonical configuration") {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestDecodeStrictControlJSONRejectsUnknownFields(t *testing.T) {
	var request controlRequest
	err := decodeStrictJSON([]byte(`{"schema_version":1,"operation":"save","document":"{}","command":"id"}`), &request)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field rejection, found %v", err)
	}
}

func TestControlClientClosesRequestBeforeReadingResponse(t *testing.T) {
	placeholder, err := os.CreateTemp("/tmp", "steer-control-test-*")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(socketPath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(socketPath)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		connection, err := listener.AcceptUnix()
		if err != nil {
			serverDone <- err
			return
		}
		defer connection.Close()
		requestData, err := io.ReadAll(connection)
		if err != nil {
			serverDone <- err
			return
		}
		var request controlRequest
		if err := decodeStrictJSON(requestData, &request); err != nil {
			serverDone <- err
			return
		}
		if request.Operation != "save" {
			serverDone <- errors.New("unexpected operation")
			return
		}
		serverDone <- json.NewEncoder(connection).Encode(controlResponse{SchemaVersion: controlSchemaVersion, OK: true})
	}()
	inputPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(inputPath, []byte(`{"main":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runControlClient([]string{"--socket", socketPath, "--operation", "save", "--input", inputPath}, &output); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"ok": true`) {
		t.Fatalf("unexpected client output: %s", output.String())
	}
}
