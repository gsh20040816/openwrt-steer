// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	model "github.com/gsh20040816/steer/go/internal/intent"
	macosplatform "github.com/gsh20040816/steer/go/internal/platform/macos"
)

const (
	controlSchemaVersion  = 1
	defaultControlSocket  = "/var/run/steer/control.sock"
	maxControlDocument    = 4 << 20
	maxControlMessage     = 8 << 20
	maxControlConnections = 8
	controlDeadline       = 2 * time.Minute
)

type controlRequest struct {
	SchemaVersion int    `json:"schema_version"`
	Operation     string `json:"operation"`
	Document      string `json:"document,omitempty"`
	ID            string `json:"id,omitempty"`
	NodeID        string `json:"node_id,omitempty"`
}

type controlResponse struct {
	SchemaVersion int                   `json:"schema_version"`
	OK            bool                  `json:"ok"`
	Saved         bool                  `json:"saved"`
	Applied       bool                  `json:"applied"`
	Revision      string                `json:"revision,omitempty"`
	Status        *macosplatform.Status `json:"status,omitempty"`
	Payload       json.RawMessage       `json:"payload,omitempty"`
	Error         string                `json:"error,omitempty"`
}

type controlService struct {
	configPath string
	adminGID   int
	options    macosplatform.BackendOptions
	mu         sync.Mutex
	write      func(string, []byte, int) error
	apply      func(model.Intent, macosplatform.BackendOptions) error
	status     func(macosplatform.BackendOptions) macosplatform.Status
}

func runControlClient(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("control", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	socketPath := flags.String("socket", defaultControlSocket, "root control service socket")
	operation := flags.String("operation", "", "restricted operation: save or apply")
	inputPath := flags.String("input", "", "canonical JSON input file")
	id := flags.String("id", "", "subscription ID for a restricted subscription operation")
	nodeID := flags.String("node", "", "stale node ID for a restricted subscription operation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	allowed := *operation == "save" || *operation == "apply" || *operation == "subscription-update" || *operation == "subscription-clean"
	if flags.NArg() != 0 || !allowed {
		return errors.New("control requires a supported restricted operation")
	}
	var document []byte
	var err error
	if *operation == "save" || *operation == "apply" {
		if *inputPath == "" {
			return errors.New("control save/apply requires --input")
		}
		document, err = readLimitedFile(*inputPath, maxControlDocument)
		if err != nil {
			return fmt.Errorf("read control input: %w", err)
		}
	} else if *id == "" || (*operation == "subscription-clean" && *nodeID == "") {
		return errors.New("control subscription operation requires --id and clean also requires --node")
	}
	request := controlRequest{SchemaVersion: controlSchemaVersion, Operation: *operation, Document: string(document), ID: *id, NodeID: *nodeID}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	connection, err := net.DialTimeout("unix", *socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to Steer control service: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(controlDeadline)); err != nil {
		return err
	}
	if _, err := connection.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("send control request: %w", err)
	}
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		return errors.New("control connection is not a Unix socket")
	}
	if err := unixConnection.CloseWrite(); err != nil {
		return fmt.Errorf("finish control request: %w", err)
	}
	responseData, err := io.ReadAll(io.LimitReader(connection, maxControlMessage+1))
	if err != nil {
		return fmt.Errorf("read control response: %w", err)
	}
	if len(responseData) > maxControlMessage {
		return errors.New("control response exceeded size limit")
	}
	var response controlResponse
	if err := decodeStrictJSON(responseData, &response); err != nil {
		return fmt.Errorf("decode control response: %w", err)
	}
	if response.SchemaVersion != controlSchemaVersion {
		return fmt.Errorf("unsupported control response schema %d", response.SchemaVersion)
	}
	writeErr := writeJSON(stdout, response)
	if writeErr != nil {
		return writeErr
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "control operation failed"
		}
		return errors.New(response.Error)
	}
	return nil
}

func runControlService(args []string) error {
	flags := flag.NewFlagSet("_control", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	socketPath := flags.String("socket", defaultControlSocket, "root control service socket")
	configPath := flags.String("config", defaultConfigPath, "canonical JSON configuration")
	options := bindBackendFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("_control accepts flags only")
	}
	if os.Geteuid() != 0 {
		return errors.New("Steer control service must run as root")
	}
	adminGID, err := lookupAdminGID()
	if err != nil {
		return err
	}
	if err := prepareControlSocketPath(*socketPath); err != nil {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: *socketPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen on Steer control socket: %w", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)
	if err := os.Chown(*socketPath, 0, adminGID); err != nil {
		return fmt.Errorf("set control socket owner: %w", err)
	}
	if err := os.Chmod(*socketPath, 0o660); err != nil {
		return fmt.Errorf("set control socket mode: %w", err)
	}
	service := &controlService{configPath: *configPath, adminGID: adminGID, options: options.value()}
	connections := make(chan struct{}, maxControlConnections)
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			return fmt.Errorf("accept control connection: %w", err)
		}
		select {
		case connections <- struct{}{}:
			go func() {
				defer func() { <-connections }()
				service.serve(connection)
			}()
		default:
			_ = connection.Close()
		}
	}
}

func (service *controlService) serve(connection *net.UnixConn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(controlDeadline))
	uid, groups, err := peerCredentials(connection)
	if err != nil || !authorizedControlPeer(uid, groups, uint32(service.adminGID)) {
		service.writeResponse(connection, controlResponse{SchemaVersion: controlSchemaVersion, Error: "control access denied"})
		return
	}
	limited := io.LimitReader(connection, maxControlMessage+1)
	data, err := io.ReadAll(limited)
	if err != nil || len(data) > maxControlMessage {
		service.writeResponse(connection, controlResponse{SchemaVersion: controlSchemaVersion, Error: "invalid control request"})
		return
	}
	var request controlRequest
	if err := decodeStrictJSON(data, &request); err != nil {
		service.writeResponse(connection, controlResponse{SchemaVersion: controlSchemaVersion, Error: "invalid control request: " + err.Error()})
		return
	}
	response := service.handle(request)
	service.writeResponse(connection, response)
}

func (service *controlService) handle(request controlRequest) controlResponse {
	response := controlResponse{SchemaVersion: controlSchemaVersion}
	if request.SchemaVersion != controlSchemaVersion {
		response.Error = fmt.Sprintf("unsupported control request schema %d", request.SchemaVersion)
		return response
	}
	if request.Operation != "save" && request.Operation != "apply" && request.Operation != "subscription-update" && request.Operation != "subscription-clean" {
		response.Error = "unsupported control operation"
		return response
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.options.RunDirectory != "" {
		lock, err := acquireLock(service.options.RunDirectory)
		if err != nil {
			response.Error = err.Error()
			return response
		}
		defer lock.Close()
	}
	if request.Operation == "subscription-update" {
		snapshots, err := macosplatform.UpdateConfiguredSubscriptions(context.Background(), &http.Client{Timeout: 30 * time.Second}, service.configPath, service.options.StateDirectory, request.ID)
		if err != nil {
			response.Error = err.Error()
			return response
		}
		if err := setControlConfigurationPermissions(service.configPath, service.adminGID); err != nil {
			response.Error = err.Error()
			return response
		}
		for _, snapshot := range snapshots {
			if err := setControlStatePermissions(filepath.Join(service.options.StateDirectory, "subscriptions", snapshot.SubscriptionID+".json"), service.adminGID); err != nil {
				response.Error = err.Error()
				return response
			}
		}
		response.Payload, _ = json.Marshal(struct {
			Snapshots []macosplatform.SubscriptionSnapshot `json:"snapshots"`
		}{snapshots})
		response.OK = true
		return response
	}
	if request.Operation == "subscription-clean" {
		snapshot, err := macosplatform.CleanSubscriptionNode(service.configPath, service.options.StateDirectory, request.ID, request.NodeID)
		if err != nil {
			response.Error = err.Error()
			return response
		}
		if err := setControlConfigurationPermissions(service.configPath, service.adminGID); err != nil {
			response.Error = err.Error()
			return response
		}
		if err := setControlStatePermissions(filepath.Join(service.options.StateDirectory, "subscriptions", snapshot.SubscriptionID+".json"), service.adminGID); err != nil {
			response.Error = err.Error()
			return response
		}
		response.Payload, _ = json.Marshal(struct {
			Snapshot macosplatform.SubscriptionSnapshot `json:"snapshot"`
		}{snapshot})
		response.OK = true
		return response
	}
	if len(request.Document) == 0 || len(request.Document) > maxControlDocument {
		response.Error = "canonical configuration is empty or exceeds the size limit"
		return response
	}
	value, err := model.DecodeJSON(strings.NewReader(request.Document))
	if err != nil {
		response.Error = "decode canonical configuration: " + err.Error()
		return response
	}
	validation := macosplatform.Validate(value)
	if !validation.OK {
		response.Error = fmt.Sprintf("canonical configuration has %d validation error(s)", len(validation.Errors))
		return response
	}
	write := service.write
	if write == nil {
		write = writeControlConfiguration
	}
	if err := write(service.configPath, []byte(request.Document), service.adminGID); err != nil {
		response.Error = err.Error()
		return response
	}
	response.Saved = true
	response.Revision = controlRevision([]byte(request.Document))
	if request.Operation == "apply" {
		apply := service.apply
		if apply == nil {
			apply = applyControlConfiguration
		}
		if err := apply(value, service.options); err != nil {
			response.Error = err.Error()
			return response
		}
		response.Applied = true
		readStatus := service.status
		if readStatus == nil {
			readStatus = func(options macosplatform.BackendOptions) macosplatform.Status {
				return macosplatform.NewBackend(macosplatform.ExecRunner{}, model.Intent{}, options).ReadStatus(context.Background())
			}
		}
		status := readStatus(service.options)
		response.Status = &status
	}
	response.OK = true
	return response
}

func controlRevision(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256-" + hex.EncodeToString(sum[:])
}

func setControlConfigurationPermissions(path string, adminGID int) error {
	if err := os.Chown(path, 0, adminGID); err != nil {
		return fmt.Errorf("set canonical configuration owner: %w", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		return fmt.Errorf("set canonical configuration mode: %w", err)
	}
	return nil
}

func setControlStatePermissions(path string, adminGID int) error {
	if err := os.Chown(filepath.Dir(path), 0, adminGID); err != nil {
		return fmt.Errorf("set subscription state directory owner: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("set subscription state directory mode: %w", err)
	}
	if err := os.Chown(path, 0, adminGID); err != nil {
		return fmt.Errorf("set subscription state owner: %w", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		return fmt.Errorf("set subscription state mode: %w", err)
	}
	return nil
}

func applyControlConfiguration(value model.Intent, options macosplatform.BackendOptions) error {
	return runApplyOperation(options.RunDirectory, func() (coreapply.Result, error) {
		backend := macosplatform.NewBackend(macosplatform.ExecRunner{}, value, options)
		return coreapply.Run(context.Background(), value, backend.CompilerOptions(), backend)
	}, io.Discard)
}

func (service *controlService) writeResponse(writer io.Writer, response controlResponse) {
	_ = json.NewEncoder(writer).Encode(response)
}

func authorizedControlPeer(uid uint32, groups []uint32, adminGID uint32) bool {
	if uid == 0 {
		return true
	}
	for _, group := range groups {
		if group == adminGID {
			return true
		}
	}
	return false
}

func lookupAdminGID() (int, error) {
	group, err := user.LookupGroup("admin")
	if err != nil {
		return 0, fmt.Errorf("look up macOS admin group: %w", err)
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil || gid <= 0 {
		return 0, fmt.Errorf("invalid macOS admin group id %q", group.Gid)
	}
	return gid, nil
}

func prepareControlSocketPath(socketPath string) error {
	if !filepath.IsAbs(socketPath) {
		return errors.New("control socket path must be absolute")
	}
	directory := filepath.Dir(filepath.Clean(socketPath))
	if info, err := os.Lstat(directory); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("control socket directory is not a real directory")
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create control socket directory: %w", err)
	}
	if err := os.Chown(directory, 0, 0); err != nil {
		return fmt.Errorf("set control socket directory owner: %w", err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		return fmt.Errorf("set control socket directory mode: %w", err)
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("refusing to replace non-socket control path")
		}
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("remove stale control socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeControlConfiguration(path string, content []byte, adminGID int) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect configuration directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("configuration directory is not a real directory")
	}
	temporary, err := os.CreateTemp(directory, ".steer-config-*")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	closeWithError := func(err error) error {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o640); err != nil {
		return closeWithError(err)
	}
	if err := temporary.Chown(0, adminGID); err != nil {
		return closeWithError(err)
	}
	if _, err := temporary.Write(content); err != nil {
		return closeWithError(err)
	}
	if len(content) == 0 || content[len(content)-1] != '\n' {
		if _, err := temporary.Write([]byte{'\n'}); err != nil {
			return closeWithError(err)
		}
	}
	if err := temporary.Sync(); err != nil {
		return closeWithError(err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace canonical configuration: %w", err)
	}
	return nil
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds size limit")
	}
	return data, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
