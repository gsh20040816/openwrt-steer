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

type PlatformSettings struct {
	SchemaVersion int    `json:"schema_version"`
	GeoSitePath   string `json:"geosite_path,omitempty"`
	GeoIPPath     string `json:"geoip_path,omitempty"`
}

func DefaultPlatformSettings() PlatformSettings {
	return PlatformSettings{SchemaVersion: PlatformSchemaVersion}
}

func ValidatePlatformSettings(value PlatformSettings) error {
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

type PlatformStore struct {
	Path string
}

func (store PlatformStore) normalizedPath() string {
	if store.Path == "" {
		return "/etc/steer/platform.json"
	}
	return store.Path
}

func (store PlatformStore) Load() (PlatformSettings, string, error) {
	content, err := os.ReadFile(store.normalizedPath())
	if os.IsNotExist(err) {
		value := DefaultPlatformSettings()
		encoded, _ := encodePlatformSettings(value)
		return value, revision(encoded), nil
	}
	if err != nil {
		return PlatformSettings{}, "", fmt.Errorf("read platform settings: %w", err)
	}
	value, err := decodePlatformSettings(content)
	if err != nil {
		return PlatformSettings{}, "", err
	}
	return value, revision(content), nil
}

func (store PlatformStore) Save(value PlatformSettings, expectedRevision string) (string, error) {
	if err := ValidatePlatformSettings(value); err != nil {
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
	content, err := encodePlatformSettings(value)
	if err != nil {
		return "", err
	}
	if err := atomicWrite(store.normalizedPath(), content); err != nil {
		return "", fmt.Errorf("write platform settings: %w", err)
	}
	return revision(content), nil
}

func decodePlatformSettings(content []byte) (PlatformSettings, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var value PlatformSettings
	if err := decoder.Decode(&value); err != nil {
		return PlatformSettings{}, fmt.Errorf("decode platform settings: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return PlatformSettings{}, fmt.Errorf("decode platform settings: %w", err)
	}
	if err := ValidatePlatformSettings(value); err != nil {
		return PlatformSettings{}, err
	}
	return value, nil
}

func encodePlatformSettings(value PlatformSettings) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode platform settings: %w", err)
	}
	return append(encoded, '\n'), nil
}
