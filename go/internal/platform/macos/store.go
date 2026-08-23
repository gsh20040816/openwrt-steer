// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

const AppGroupSchemaVersion = 1

var ErrRevisionConflict = errors.New("configuration revision conflict")

// Paths is the only filesystem layout known by the macOS adapter. The caller
// must pass the resolved App Group container; this package never guesses a
// user, home, or Unix system directory.
type Paths struct {
	Root                 string `json:"root"`
	ConfigDirectory      string `json:"config_directory"`
	ConfigPath           string `json:"config_path"`
	GenerationsDirectory string `json:"generations_directory"`
	StateDirectory       string `json:"state_directory"`
	StatusDirectory      string `json:"status_directory"`
	StatusPath           string `json:"status_path"`
	LogsDirectory        string `json:"logs_directory"`
}

func NewPaths(appGroupRoot string) (Paths, error) {
	if appGroupRoot == "" || !filepath.IsAbs(appGroupRoot) {
		return Paths{}, errors.New("macOS App Group root must be an absolute path")
	}
	root := filepath.Clean(appGroupRoot)
	configDirectory := filepath.Join(root, "config")
	return Paths{
		Root: root, ConfigDirectory: configDirectory,
		ConfigPath:           filepath.Join(configDirectory, "config.json"),
		GenerationsDirectory: filepath.Join(root, "generations"),
		StateDirectory:       filepath.Join(root, "state"),
		StatusDirectory:      filepath.Join(root, "status"),
		StatusPath:           filepath.Join(root, "status", "current.json"),
		LogsDirectory:        filepath.Join(root, "logs"),
	}, nil
}

func (paths Paths) Ensure() error {
	for _, directory := range []string{
		paths.ConfigDirectory, paths.GenerationsDirectory, paths.StateDirectory,
		paths.StatusDirectory, paths.LogsDirectory,
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create macOS App Group directory %s: %w", directory, err)
		}
	}
	return nil
}

type IntentStore struct {
	Paths Paths
}

func (store IntentStore) Load() (model.Intent, string, error) {
	content, err := os.ReadFile(store.Paths.ConfigPath)
	if err != nil {
		return model.Intent{}, "", fmt.Errorf("read macOS canonical intent: %w", err)
	}
	value, err := model.DecodeJSON(bytes.NewReader(content))
	if err != nil {
		return model.Intent{}, "", err
	}
	return value, contentRevision(content), nil
}

func (store IntentStore) Save(value model.Intent, expectedRevision string) (string, error) {
	validation := Validate(value)
	if !validation.OK {
		return "", ValidationError{Validation: validation}
	}
	if expectedRevision != "" {
		_, current, err := store.Load()
		if err != nil {
			return "", err
		}
		if current != expectedRevision {
			return "", ErrRevisionConflict
		}
	}
	var encoded bytes.Buffer
	if err := model.EncodeJSON(&encoded, value); err != nil {
		return "", err
	}
	if err := atomicWrite(store.Paths.ConfigPath, encoded.Bytes()); err != nil {
		return "", fmt.Errorf("write macOS canonical intent: %w", err)
	}
	return contentRevision(encoded.Bytes()), nil
}

type Status struct {
	SchemaVersion int               `json:"schema_version"`
	Healthy       bool              `json:"healthy"`
	GenerationID  string            `json:"generation_id,omitempty"`
	IntentDigest  string            `json:"intent_digest,omitempty"`
	LastApply     *coreapply.Record `json:"last_apply,omitempty"`
	Error         string            `json:"error,omitempty"`
}

func DefaultStatus() Status {
	return Status{SchemaVersion: AppGroupSchemaVersion}
}

func (paths Paths) LoadStatus() (Status, error) {
	content, err := os.ReadFile(paths.StatusPath)
	if err != nil {
		return Status{}, fmt.Errorf("read macOS status: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var status Status
	if err := decoder.Decode(&status); err != nil {
		return Status{}, fmt.Errorf("decode macOS status: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return Status{}, fmt.Errorf("decode macOS status: %w", err)
	}
	if status.SchemaVersion != AppGroupSchemaVersion {
		return Status{}, fmt.Errorf("macOS status requires schema %d, found %d", AppGroupSchemaVersion, status.SchemaVersion)
	}
	return status, nil
}

func (paths Paths) SaveStatus(status Status) error {
	if status.SchemaVersion == 0 {
		status.SchemaVersion = AppGroupSchemaVersion
	}
	if status.SchemaVersion != AppGroupSchemaVersion {
		return fmt.Errorf("macOS status requires schema %d, found %d", AppGroupSchemaVersion, status.SchemaVersion)
	}
	encoded, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("encode macOS status: %w", err)
	}
	return atomicWrite(paths.StatusPath, append(encoded, '\n'))
}

func contentRevision(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256-" + hex.EncodeToString(sum[:])
}

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".steer-write-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func readJSON(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	return content, nil
}

func unmarshalStrict(content []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
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

func jsonMarshal(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode JSON: %w", err)
	}
	return encoded, nil
}

type ValidationError struct{ Validation model.Validation }

func (value ValidationError) Error() string {
	return fmt.Sprintf("candidate validation failed with %d error(s)", len(value.Validation.Errors))
}
