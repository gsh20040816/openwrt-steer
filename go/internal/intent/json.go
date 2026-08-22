// SPDX-License-Identifier: GPL-3.0-or-later

package intent

import (
	"encoding/json"
	"fmt"
	"io"
)

// DecodeJSON decodes the canonical cross-platform file format. Unknown fields
// and trailing JSON values are errors so file-backed platforms cannot silently
// accept intent that OpenWrt would reject.
func DecodeJSON(reader io.Reader) (Intent, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var value Intent
	if err := decoder.Decode(&value); err != nil {
		return Intent{}, fmt.Errorf("decode canonical intent JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Intent{}, fmt.Errorf("decode canonical intent JSON: trailing value")
		}
		return Intent{}, fmt.Errorf("decode canonical intent JSON: %w", err)
	}
	return value, nil
}

// EncodeJSON writes the deterministic representation stored in every runtime
// generation and consumed by future file-backed platform adapters.
func EncodeJSON(writer io.Writer, value Intent) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode canonical intent JSON: %w", err)
	}
	return nil
}
