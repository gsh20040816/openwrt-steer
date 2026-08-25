// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

var ErrRevisionConflict = errors.New("configuration revision conflict")

type IntentStore struct {
	Path string
}

func (store IntentStore) normalizedPath() string {
	if store.Path == "" {
		return "/etc/steer/config.json"
	}
	return store.Path
}

func (store IntentStore) Load() (model.Intent, string, error) {
	content, err := os.ReadFile(store.normalizedPath())
	if err != nil {
		return model.Intent{}, "", fmt.Errorf("read canonical intent: %w", err)
	}
	value, err := model.DecodeJSON(bytes.NewReader(content))
	if err != nil {
		return model.Intent{}, "", err
	}
	return value, revision(content), nil
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
	content := encoded.Bytes()
	if err := atomicWrite(store.normalizedPath(), content); err != nil {
		return "", fmt.Errorf("write canonical intent: %w", err)
	}
	return revision(content), nil
}

func (store IntentStore) MigrateSchema8() (bool, error) {
	path := store.normalizedPath()
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read canonical intent for migration: %w", err)
	}
	var header struct {
		Main struct {
			SchemaVersion int `json:"schema_version"`
		} `json:"main"`
	}
	if err := json.Unmarshal(content, &header); err != nil {
		return false, fmt.Errorf("inspect canonical intent schema: %w", err)
	}
	switch header.Main.SchemaVersion {
	case model.SchemaVersion:
		if _, err := model.DecodeJSON(bytes.NewReader(content)); err != nil {
			return false, err
		}
		return false, nil
	case 8:
	default:
		return false, fmt.Errorf("cannot migrate canonical intent schema %d", header.Main.SchemaVersion)
	}
	value, err := model.MigrateJSON8(content)
	if err != nil {
		return false, err
	}
	validation := Validate(value)
	if !validation.OK {
		return false, ValidationError{Validation: validation}
	}
	var encoded bytes.Buffer
	if err := model.EncodeJSON(&encoded, value); err != nil {
		return false, err
	}
	if err := atomicWrite(path, encoded.Bytes()); err != nil {
		return false, fmt.Errorf("write migrated canonical intent: %w", err)
	}
	return true, nil
}

func revision(content []byte) string {
	sum := sha256.Sum256(content)
	return `"sha256-` + hex.EncodeToString(sum[:]) + `"`
}

type ValidationError struct {
	Validation model.Validation
}

func (value ValidationError) Error() string {
	return fmt.Sprintf("candidate validation failed with %d error(s)", len(value.Validation.Errors))
}
