// SPDX-License-Identifier: GPL-3.0-or-later

// Package steercore exposes the narrow JSON contract intended for future
// Swift/XPC or C ABI adapters. It does not import any operating-system
// package; platform adapters provide compiler.Target explicitly.
package steercore

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

const ABIVersion = 1

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Envelope struct {
	ABIVersion int             `json:"abi_version"`
	OK         bool            `json:"ok"`
	Value      json.RawMessage `json:"value,omitempty"`
	Error      *Error          `json:"error,omitempty"`
}

type MACBinding struct {
	Address       string `json:"address"`
	TProxyTag     string `json:"tproxy_tag"`
	DNSInboundTag string `json:"dns_inbound_tag"`
}

// Target is the JSON-façade representation of platform-owned sing-box
// fragments. It intentionally contains no Linux, OpenWrt, Darwin or Swift
// type. Platform packages translate their native plan into this small shape.
type Target struct {
	Inbounds             []any        `json:"inbounds"`
	DNSCaptureMode       string       `json:"dns_capture_mode"`
	DNSInboundTags       []string     `json:"dns_inbound_tags"`
	SniffInboundTags     []string     `json:"sniff_inbound_tags"`
	MACBindings          []MACBinding `json:"mac_bindings"`
	RequiredCapabilities []string     `json:"required_capabilities"`
}

// ValidateJSON always returns an ABI envelope. Malformed input and invalid
// canonical intent are represented as explicit errors rather than defaults.
func ValidateJSON(input []byte) []byte {
	value, err := decode(input)
	if err != nil {
		return failure("INVALID_JSON", err.Error())
	}
	validation := model.Validate(value)
	if !validation.OK {
		return envelope(false, validation, &Error{Code: "VALIDATION_FAILED", Message: "canonical intent validation failed"})
	}
	return success(validation)
}

// CompileJSON compiles a validated intent with the caller-provided target.
// The compiler target is intentionally an argument so this package remains
// platform-neutral; macOS-specific target construction belongs in the macOS
// adapter package.
func CompileJSON(input []byte, stateDirectory string, target Target) []byte {
	value, err := decode(input)
	if err != nil {
		return failure("INVALID_JSON", err.Error())
	}
	validation := model.Validate(value)
	if !validation.OK {
		return envelope(false, validation, &Error{Code: "VALIDATION_FAILED", Message: "canonical intent validation failed"})
	}
	bundle := compiler.Compile(value, compiler.Options{StateDirectory: stateDirectory, Target: target.compilerTarget()})
	return success(bundle)
}

func (target Target) compilerTarget() compiler.Target {
	bindings := make([]compiler.MACBinding, 0, len(target.MACBindings))
	for _, binding := range target.MACBindings {
		bindings = append(bindings, compiler.MACBinding{
			Address: binding.Address, TProxyTag: binding.TProxyTag, DNSInboundTag: binding.DNSInboundTag,
		})
	}
	return compiler.Target{
		Inbounds: target.Inbounds,
		DNSCapture: compiler.DNSCapture{
			Mode:        compiler.DNSCaptureMode(target.DNSCaptureMode),
			InboundTags: target.DNSInboundTags,
		},
		SniffInboundTags:     target.SniffInboundTags,
		MACBindings:          bindings,
		RequiredCapabilities: target.RequiredCapabilities,
	}
}

func decode(input []byte) (model.Intent, error) {
	value, err := model.DecodeJSON(bytes.NewReader(input))
	if err != nil {
		return model.Intent{}, fmt.Errorf("decode canonical intent: %w", err)
	}
	return value, nil
}

func success(value any) []byte {
	return envelope(true, value, nil)
}

func failure(code, message string) []byte {
	return EncodeEnvelope(false, nil, &Error{Code: code, Message: message})
}

func envelope(ok bool, value any, failure *Error) []byte {
	return EncodeEnvelope(ok, value, failure)
}

// EncodeEnvelope lets a platform adapter preserve the same ABI while adding
// its own validation before calling the shared compiler.
func EncodeEnvelope(ok bool, value any, failure *Error) []byte {
	var raw json.RawMessage
	if value != nil {
		encoded, err := json.Marshal(value)
		if err != nil {
			failure = &Error{Code: "ENCODE_FAILED", Message: err.Error()}
			ok = false
		} else {
			raw = encoded
		}
	}
	encoded, err := json.Marshal(Envelope{ABIVersion: ABIVersion, OK: ok, Value: raw, Error: failure})
	if err != nil {
		panic(fmt.Sprintf("encode ABI envelope: %v", err))
	}
	return encoded
}
