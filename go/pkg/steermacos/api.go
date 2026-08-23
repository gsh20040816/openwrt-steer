// SPDX-License-Identifier: GPL-3.0-or-later

// Package steermacos is the public platform façade used by the standalone
// macOS bridge module. Platform code is kept out of pkg/steercore.
package steermacos

import internal "github.com/gsh20040816/steer/go/internal/platform/macos"

func ValidateJSON(input []byte) []byte {
	return internal.ValidateJSON(input)
}

func CompileJSON(input []byte, stateDirectory string) []byte {
	return internal.CompileJSON(input, stateDirectory)
}

func PrepareJSON(input []byte, appGroupRoot string) []byte {
	return internal.PrepareJSON(input, appGroupRoot)
}
