// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/geodata"
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
		revision: func(string) (string, error) {
			return controlRevision(document), nil
		},
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
		SchemaVersion:    controlSchemaVersion,
		Operation:        "apply",
		Document:         string(document),
		ExpectedRevision: controlRevision(document),
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

func TestControlServiceProbeForwardsOnlyStructuredSelection(t *testing.T) {
	called := false
	service := &controlService{
		configPath: "/fixed/config.json",
		probe: func(_ context.Context, configPath string, options macosplatform.BackendOptions, selection probeSelection) (probeResponse, error) {
			called = true
			if configPath != "/fixed/config.json" || options.RunDirectory != "" {
				t.Fatalf("probe received unexpected fixed paths: %q %#v", configPath, options)
			}
			if selection.Kind != "speedtest" || selection.NodeID != "node-a" || selection.RouteID != "" || !selection.Download {
				t.Fatalf("probe received unexpected selection: %#v", selection)
			}
			return probeResponse{
				TestReport: macosplatform.TestReport{Scope: "nodes", ObjectID: "node-a", Kind: "download", OK: false},
			}, nil
		},
	}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "probe",
		Kind:          "speedtest",
		NodeID:        "node-a",
		Download:      true,
	})
	if !called || !response.OK || len(response.Payload) == 0 {
		t.Fatalf("structured probe was not returned: called=%v response=%+v", called, response)
	}
	var report probeResponse
	if err := decodeStrictJSON(response.Payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.OK || report.Scope != "nodes" || report.ObjectID != "node-a" {
		t.Fatalf("HTTP probe failure was confused with control failure: %#v", report)
	}
}

func TestControlServiceOverviewProbeRequiresHealthyActiveGeneration(t *testing.T) {
	called := false
	service := &controlService{
		status: func(macosplatform.BackendOptions) macosplatform.Status {
			return macosplatform.Status{
				SchemaVersion: macosplatform.RuntimeSchemaVersion,
				GenerationID:  "generation-a",
				IntentDigest:  "digest-a",
			}
		},
		probe: func(context.Context, string, macosplatform.BackendOptions, probeSelection) (probeResponse, error) {
			called = true
			return probeResponse{}, nil
		},
	}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "probe",
		Kind:          "proxy",
	})
	if response.OK || called || !strings.Contains(response.Error, "not healthy") {
		t.Fatalf("unhealthy Active probe was not rejected before HTTP: called=%v response=%+v", called, response)
	}
}

func TestControlServiceOverviewProbeBindsHealthyActiveIdentity(t *testing.T) {
	service := &controlService{
		status: func(macosplatform.BackendOptions) macosplatform.Status {
			return macosplatform.Status{
				SchemaVersion: macosplatform.RuntimeSchemaVersion,
				Healthy:       true,
				GenerationID:  "generation-a",
				IntentDigest:  "digest-a",
			}
		},
		probe: func(context.Context, string, macosplatform.BackendOptions, probeSelection) (probeResponse, error) {
			return probeResponse{
				TestReport:       macosplatform.TestReport{Scope: "overview", Kind: "proxy", OK: true},
				ActiveGeneration: "generation-a",
				ActiveDigest:     "digest-a",
			}, nil
		},
	}
	request := controlRequest{SchemaVersion: controlSchemaVersion, Operation: "probe", Kind: "proxy"}
	if response := service.handle(request); !response.OK {
		t.Fatalf("healthy Active probe was rejected: %+v", response)
	}
	service.probe = func(context.Context, string, macosplatform.BackendOptions, probeSelection) (probeResponse, error) {
		return probeResponse{
			TestReport:       macosplatform.TestReport{Scope: "overview", Kind: "proxy", OK: true},
			ActiveGeneration: "generation-b",
			ActiveDigest:     "digest-b",
		}, nil
	}
	if response := service.handle(request); response.OK || !strings.Contains(response.Error, "changed") {
		t.Fatalf("mismatched Active identity was returned: %+v", response)
	}
}

func TestControlServiceOverviewProbeRejectsRuntimeThatBecomesUnhealthy(t *testing.T) {
	statusReads := 0
	service := &controlService{
		status: func(macosplatform.BackendOptions) macosplatform.Status {
			statusReads++
			return macosplatform.Status{
				SchemaVersion: macosplatform.RuntimeSchemaVersion,
				Healthy:       statusReads == 1,
				GenerationID:  "generation-a",
				IntentDigest:  "digest-a",
			}
		},
		probe: func(context.Context, string, macosplatform.BackendOptions, probeSelection) (probeResponse, error) {
			return probeResponse{
				TestReport:       macosplatform.TestReport{Scope: "overview", Kind: "proxy", OK: true},
				ActiveGeneration: "generation-a",
				ActiveDigest:     "digest-a",
			}, nil
		},
	}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "probe",
		Kind:          "proxy",
	})
	if response.OK || statusReads != 2 || !strings.Contains(response.Error, "became unhealthy") {
		t.Fatalf("probe survived an unhealthy Active transition: reads=%d response=%+v", statusReads, response)
	}
}

func TestControlServiceProbeRejectsConfigurationAndPathFields(t *testing.T) {
	service := &controlService{
		probe: func(context.Context, string, macosplatform.BackendOptions, probeSelection) (probeResponse, error) {
			t.Fatal("invalid probe reached backend")
			return probeResponse{}, nil
		},
	}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "probe",
		Kind:          "proxy",
		Document:      `{"url":"https://attacker.example/"}`,
	})
	if response.OK || !strings.Contains(response.Error, "only kind") {
		t.Fatalf("unexpected invalid probe response: %+v", response)
	}
	var request controlRequest
	if err := decodeStrictJSON([]byte(`{"schema_version":1,"operation":"probe","kind":"proxy","url":"https://attacker.example/"}`), &request); err == nil {
		t.Fatal("control probe accepted an arbitrary URL field")
	}
}

func TestControlServiceRejectsInvalidDocumentBeforeWrite(t *testing.T) {
	service := &controlService{
		configPath: t.TempDir() + "/config.json",
		revision:   func(string) (string, error) { return "current", nil },
	}
	response := service.handle(controlRequest{
		SchemaVersion:    controlSchemaVersion,
		Operation:        "save",
		Document:         `{"main":`,
		ExpectedRevision: "current",
	})
	if response.OK || !strings.Contains(response.Error, "decode canonical configuration") {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestControlServiceReturnsStructuredGeoValidationBeforeWrite(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "..", "linux", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := model.DecodeJSON(bytes.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	value.Rules = append([]model.Rule{{
		ID: "geo", Enabled: true, DNSProfile: value.Rules[0].DNSProfile, Route: value.Rules[0].Route,
		DomainMatch: []string{"geosite:cn"},
	}}, value.Rules...)
	var candidate bytes.Buffer
	if err := model.EncodeJSON(&candidate, value); err != nil {
		t.Fatal(err)
	}
	written := false
	service := &controlService{
		configPath: filepath.Join(t.TempDir(), "config.json"),
		revision:   func(string) (string, error) { return "current", nil },
		options: macosplatform.BackendOptions{
			GeoDataDirectory: filepath.Join(t.TempDir(), "missing"),
		},
		write: func(string, []byte, int) error { written = true; return nil },
	}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion, Operation: "save", Document: candidate.String(), ExpectedRevision: "current",
	})
	if response.OK || response.Saved || written || response.Validation == nil {
		t.Fatalf("invalid candidate reached persistence: response=%+v written=%v", response, written)
	}
	if response.ErrorCode == controlRevisionRequired || response.ErrorCode == controlRevisionConflict {
		t.Fatalf("Geo validation was masked by revision handling: %+v", response)
	}
	for _, issue := range response.Validation.Errors {
		if issue.Code == geodata.ErrorManifestInvalid && issue.ObjectType == "rule" && issue.ObjectID == "geo" && issue.Option == "domain_match" {
			return
		}
	}
	t.Fatalf("structured manifest issue is missing: %#v", response.Validation.Errors)
}

func TestControlServiceSavedOnlyConfigurationCannotReachApplyHook(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "..", "linux", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	applyCalls := 0
	writeCalls := 0
	service := &controlService{
		configPath: "/fixed/config.json",
		revision: func(string) (string, error) {
			return controlRevision(document), nil
		},
		write: func(_ string, _ []byte, _ int) error {
			writeCalls++
			return nil
		},
		apply: func(_ model.Intent, _ macosplatform.BackendOptions) error {
			applyCalls++
			return nil
		},
	}
	response := service.handle(controlRequest{
		SchemaVersion:    controlSchemaVersion,
		Operation:        "save",
		Document:         string(document),
		ExpectedRevision: controlRevision(document),
	})
	if !response.OK || !response.Saved || response.Applied {
		t.Fatalf("unexpected save response: %+v", response)
	}
	if writeCalls != 1 || applyCalls != 0 {
		t.Fatalf("saved-only configuration wrote %d time(s) and applied %d time(s)", writeCalls, applyCalls)
	}
}

func TestControlServiceRequiresExpectedRevision(t *testing.T) {
	service := &controlService{}
	response := service.handle(controlRequest{
		SchemaVersion: controlSchemaVersion,
		Operation:     "save",
		Document:      `{}`,
	})
	if response.OK || response.Saved || response.Applied || response.ErrorCode != controlRevisionRequired {
		t.Fatalf("unexpected missing-revision response: %+v", response)
	}
}

func TestControlServiceRejectsStaleGUIRevisionAfterSubscriptionUpdate(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "linux", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	updatedIntent, err := model.DecodeJSON(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	updatedIntent.Subscriptions = []model.Subscription{{ID: "public", Enabled: true, URL: "https://subscription.example/nodes"}}
	updatedIntent.Nodes = []model.Node{{
		ID: "public-node", Enabled: true, Type: "socks", Server: "127.0.0.1", ServerPort: 1080,
		NodeSource: model.NodeSource{SourceSubscription: "public"},
	}}
	var updatedBuffer bytes.Buffer
	if err := model.EncodeJSON(&updatedBuffer, updatedIntent); err != nil {
		t.Fatal(err)
	}
	updated := updatedBuffer.Bytes()
	for _, operation := range []string{"save", "apply"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			configPath := filepath.Join(root, "config.json")
			if err := os.WriteFile(configPath, updated, 0o600); err != nil {
				t.Fatal(err)
			}
			writeCalls, applyCalls := 0, 0
			service := &controlService{
				configPath: configPath,
				options:    macosplatform.BackendOptions{RunDirectory: filepath.Join(root, "run")},
				write: func(string, []byte, int) error {
					writeCalls++
					return nil
				},
				apply: func(model.Intent, macosplatform.BackendOptions) error {
					applyCalls++
					return nil
				},
			}
			response := service.handle(controlRequest{
				SchemaVersion:    controlSchemaVersion,
				Operation:        operation,
				Document:         string(original),
				ExpectedRevision: controlRevision(original),
			})
			if response.OK || response.Saved || response.Applied || response.ErrorCode != controlRevisionConflict ||
				response.Revision != controlRevision(updated) {
				t.Fatalf("stale %s returned unexpected response: %+v", operation, response)
			}
			if writeCalls != 0 || applyCalls != 0 {
				t.Fatalf("stale %s wrote %d time(s) and applied %d time(s)", operation, writeCalls, applyCalls)
			}
			after, err := os.ReadFile(configPath)
			if err != nil || string(after) != string(updated) {
				t.Fatalf("stale %s changed subscription-updated Saved content: %v\n%s", operation, err, after)
			}
		})
	}
}

func TestControlServiceReturnsRevisionOfNormalizedSavedBytes(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "..", "linux", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	document = bytes.TrimSuffix(document, []byte{'\n'})
	var written []byte
	service := &controlService{
		configPath: "/fixed/config.json",
		revision:   func(string) (string, error) { return "loaded", nil },
		write: func(_ string, content []byte, _ int) error {
			written = append([]byte{}, content...)
			return nil
		},
	}
	response := service.handle(controlRequest{
		SchemaVersion:    controlSchemaVersion,
		Operation:        "save",
		Document:         string(document),
		ExpectedRevision: "loaded",
	})
	if !response.OK || !response.Saved || len(written) == 0 || written[len(written)-1] != '\n' ||
		response.Revision != controlRevision(written) {
		t.Fatalf("response revision does not describe normalized Saved bytes: %+v %q", response, written)
	}
}

func TestControlRevisionUsesStableSHA256Format(t *testing.T) {
	const want = "sha256-ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := controlRevision([]byte("abc")); got != want {
		t.Fatalf("control revision = %q, want %q", got, want)
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
		if request.Operation != "save" || request.ExpectedRevision != "sha256-loaded" {
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
	if err := runControlClient([]string{
		"--socket", socketPath, "--operation", "save", "--input", inputPath,
		"--expected-revision", "sha256-loaded",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"ok": true`) {
		t.Fatalf("unexpected client output: %s", output.String())
	}
}

func TestRunProbeReturnsFlatActiveDTO(t *testing.T) {
	placeholder, err := os.CreateTemp("/tmp", "steer-probe-control-test-*")
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
		if request.Operation != "probe" || request.Kind != "proxy" || request.Document != "" || request.ID != "" || request.NodeID != "" || request.RouteID != "" {
			serverDone <- errors.New("unexpected structured probe request")
			return
		}
		payload := json.RawMessage(`{"scope":"overview","kind":"proxy","ok":true,"tested_at":"2026-08-26T01:02:03Z","results":[],"active_generation":"generation-a","active_digest":"digest-a"}`)
		serverDone <- json.NewEncoder(connection).Encode(controlResponse{
			SchemaVersion: controlSchemaVersion,
			OK:            true,
			Payload:       payload,
		})
	}()
	var output bytes.Buffer
	if err := runProbe([]string{"--socket", socketPath, "--kind", "proxy"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"active_generation": "generation-a"`, `"active_digest": "digest-a"`, `"tested_at": "2026-08-26T01:02:03Z"`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("flat probe output is missing %s:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), `"payload"`) {
		t.Fatalf("probe leaked the control envelope: %s", output.String())
	}
}
