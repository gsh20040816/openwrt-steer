// SPDX-License-Identifier: GPL-3.0-or-later

// Package apply fixes the cross-platform Apply order without implementing a
// workflow engine. Backends remain synchronous and own their platform details.
package apply

import (
	"context"
	"fmt"

	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/generation"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

type Result struct {
	OK           bool              `json:"ok"`
	Error        string            `json:"error,omitempty"`
	Generation   string            `json:"generation,omitempty"`
	IntentDigest string            `json:"intent_digest,omitempty"`
	Validation   *model.Validation `json:"validation,omitempty"`
}

type Record struct {
	Sequence string `json:"sequence"`
	Result   Result `json:"result"`
}

type Backend interface {
	Prepare(context.Context, model.Intent, compiler.Output) (generation.Candidate, error)
	Activate(context.Context, generation.Candidate) error
	Healthy(context.Context, generation.Candidate) error
	Finalize(context.Context, generation.Candidate) error
	Disable(context.Context) error
}

type ValidationError struct{ Validation model.Validation }

func (value ValidationError) Error() string {
	return fmt.Sprintf("candidate validation failed with %d error(s)", len(value.Validation.Errors))
}

func Run(ctx context.Context, value model.Intent, options compiler.Options, backend Backend) (Result, error) {
	validation := model.Validate(value)
	if !validation.OK {
		return Result{Validation: &validation}, ValidationError{Validation: validation}
	}
	compiled := compiler.Compile(value, options)
	result := Result{IntentDigest: compiled.IntentDigest}
	if !value.Main.Enabled {
		if err := backend.Disable(ctx); err != nil {
			return result, err
		}
		result.OK = true
		return result, nil
	}
	candidate, err := backend.Prepare(ctx, value, compiled)
	if err != nil {
		return result, err
	}
	result.Generation = candidate.Directory
	if err := backend.Activate(ctx, candidate); err != nil {
		return result, err
	}
	if err := backend.Healthy(ctx, candidate); err != nil {
		return result, err
	}
	if err := backend.Finalize(ctx, candidate); err != nil {
		return result, err
	}
	result.OK = true
	return result, nil
}
