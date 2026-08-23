// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	webCredentialsSchemaVersion = 1
	defaultWebCredentialsPath   = "/etc/steer/web.json"
)

type webCredentials struct {
	SchemaVersion int    `json:"schema_version"`
	Token         string `json:"token"`
}

func configuredWebToken(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var value webCredentials
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode Web credentials: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode Web credentials: %w", err)
	}
	if value.SchemaVersion != webCredentialsSchemaVersion {
		return nil, fmt.Errorf("Web credentials require schema %d, found %d", webCredentialsSchemaVersion, value.SchemaVersion)
	}
	if len(value.Token) < 32 {
		return nil, errors.New("Web token must contain at least 32 characters")
	}
	if len(value.Token) > 256 {
		return nil, errors.New("Web token must contain at most 256 characters")
	}
	for _, char := range []byte(value.Token) {
		if char < 0x21 || char > 0x7e {
			return nil, errors.New("Web token must use visible ASCII characters without spaces")
		}
	}
	return []byte(value.Token), nil
}
