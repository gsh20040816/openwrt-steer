// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const PlatformSchemaVersion = 1

type Settings struct {
	SchemaVersion int    `json:"schema_version"`
	GeoSitePath   string `json:"geosite_path,omitempty"`
	GeoIPPath     string `json:"geoip_path,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{SchemaVersion: PlatformSchemaVersion}
}

func ValidateSettings(value Settings) error {
	if value.SchemaVersion != PlatformSchemaVersion {
		return fmt.Errorf("platform settings require schema %d, found %d", PlatformSchemaVersion, value.SchemaVersion)
	}
	for name, path := range map[string]string{"geosite_path": value.GeoSitePath, "geoip_path": value.GeoIPPath} {
		if path != "" && !filepath.IsAbs(path) {
			return fmt.Errorf("%s must be an absolute path", name)
		}
	}
	return nil
}

type SettingsStore struct {
	Path string
}

func (store SettingsStore) normalizedPath() string {
	if store.Path == "" {
		return "/etc/steer/platform.json"
	}
	return store.Path
}

func (store SettingsStore) Load() (Settings, string, error) {
	content, err := os.ReadFile(store.normalizedPath())
	if os.IsNotExist(err) {
		value := DefaultSettings()
		encoded, _ := encodeSettings(value)
		return value, revision(encoded), nil
	}
	if err != nil {
		return Settings{}, "", fmt.Errorf("read platform settings: %w", err)
	}
	value, err := decodeSettings(content)
	if err != nil {
		return Settings{}, "", err
	}
	return value, revision(content), nil
}

func (store SettingsStore) Save(value Settings, expectedRevision string) (string, error) {
	if err := ValidateSettings(value); err != nil {
		return "", err
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
	content, err := encodeSettings(value)
	if err != nil {
		return "", err
	}
	if err := atomicWrite(store.normalizedPath(), content); err != nil {
		return "", fmt.Errorf("write platform settings: %w", err)
	}
	return revision(content), nil
}

func decodeSettings(content []byte) (Settings, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var value Settings
	if err := decoder.Decode(&value); err != nil {
		return Settings{}, fmt.Errorf("decode platform settings: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return Settings{}, fmt.Errorf("decode platform settings: %w", err)
	}
	if err := ValidateSettings(value); err != nil {
		return Settings{}, err
	}
	return value, nil
}

func encodeSettings(value Settings) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode platform settings: %w", err)
	}
	return append(encoded, '\n'), nil
}
