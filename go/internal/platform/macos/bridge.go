// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"bytes"
	"fmt"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/pkg/steercore"
)

// ValidateJSON is the macOS-specific JSON entry point. It keeps the shared
// ABI envelope but applies platform validation before returning success.
func ValidateJSON(input []byte) []byte {
	value, err := decodeJSON(input)
	if err != nil {
		return steercore.EncodeEnvelope(false, nil, &steercore.Error{Code: "INVALID_JSON", Message: err.Error()})
	}
	validation := Validate(value)
	if !validation.OK {
		return steercore.EncodeEnvelope(false, validation, &steercore.Error{Code: "VALIDATION_FAILED", Message: "macOS intent validation failed"})
	}
	return steercore.EncodeEnvelope(true, validation, nil)
}

// CompileJSON builds the macOS target without exposing platform structures to
// the shared steercore package. It is suitable for a future cgo/XPC wrapper.
func CompileJSON(input []byte, stateDirectory string) []byte {
	value, err := decodeJSON(input)
	if err != nil {
		return steercore.EncodeEnvelope(false, nil, &steercore.Error{Code: "INVALID_JSON", Message: err.Error()})
	}
	validation := Validate(value)
	if !validation.OK {
		return steercore.EncodeEnvelope(false, validation, &steercore.Error{Code: "VALIDATION_FAILED", Message: "macOS intent validation failed"})
	}
	bundle := compiler.Compile(value, NewPlan(value).CompilerOptions(stateDirectory))
	return steercore.EncodeEnvelope(true, bundle, nil)
}

// PrepareJSON writes a candidate into the caller-resolved App Group root and
// returns its metadata without activating or publishing it.
func PrepareJSON(input []byte, appGroupRoot string) []byte {
	value, err := decodeJSON(input)
	if err != nil {
		return steercore.EncodeEnvelope(false, nil, &steercore.Error{Code: "INVALID_JSON", Message: err.Error()})
	}
	paths, err := NewPaths(appGroupRoot)
	if err != nil {
		return steercore.EncodeEnvelope(false, nil, &steercore.Error{Code: "INVALID_APP_GROUP", Message: err.Error()})
	}
	prepared, err := Prepare(value, paths)
	if err != nil {
		return steercore.EncodeEnvelope(false, nil, &steercore.Error{Code: "PREPARE_FAILED", Message: err.Error()})
	}
	return steercore.EncodeEnvelope(true, prepared, nil)
}

func decodeJSON(input []byte) (model.Intent, error) {
	value, err := model.DecodeJSON(bytes.NewReader(input))
	if err != nil {
		return model.Intent{}, fmt.Errorf("decode canonical intent: %w", err)
	}
	return value, nil
}
