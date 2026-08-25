// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/generation"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

type GenerationMetadata struct {
	SchemaVersion int    `json:"schema_version"`
	GenerationID  string `json:"generation_id"`
	IntentDigest  string `json:"intent_digest"`
}

type PreparedGeneration struct {
	generation.Candidate
	Metadata GenerationMetadata
}

// Prepare compiles a validated intent and writes an immutable candidate. The
// launchd backend publishes it only after sing-box has passed its native config
// check and the new utun generation becomes healthy.
func Prepare(value model.Intent, paths Paths) (PreparedGeneration, error) {
	validation := Validate(value)
	if !validation.OK {
		return PreparedGeneration{}, ValidationError{Validation: validation}
	}
	if err := paths.Ensure(); err != nil {
		return PreparedGeneration{}, err
	}
	plan := NewPlan(value)
	compiled := compiler.Compile(value, plan.CompilerOptions(paths.StateDirectory))
	candidate, err := generation.Create(paths.GenerationsDirectory, value, compiled.SingBox)
	if err != nil {
		return PreparedGeneration{}, err
	}
	keepCandidate := false
	defer func() {
		if !keepCandidate {
			_ = os.RemoveAll(candidate.Directory)
		}
	}()
	metadata := GenerationMetadata{
		SchemaVersion: RuntimeSchemaVersion,
		GenerationID:  compiled.IntentDigest,
		IntentDigest:  compiled.IntentDigest,
	}
	if err := generation.WriteJSON(filepath.Join(candidate.Directory, "macos.json"), plan); err != nil {
		return PreparedGeneration{}, err
	}
	if err := generation.WriteJSON(filepath.Join(candidate.Directory, "generation.json"), metadata); err != nil {
		return PreparedGeneration{}, err
	}
	keepCandidate = true
	return PreparedGeneration{Candidate: candidate, Metadata: metadata}, nil
}

type CurrentGeneration struct {
	SchemaVersion int    `json:"schema_version"`
	GenerationID  string `json:"generation_id"`
	Directory     string `json:"directory"`
	IntentDigest  string `json:"intent_digest"`
}

// Publish records the candidate after launchd has stopped the old sing-box
// process and before the new LaunchDaemon is bootstrapped.
func (paths Paths) Publish(prepared PreparedGeneration) error {
	if prepared.Directory == "" || prepared.Metadata.GenerationID == "" {
		return fmt.Errorf("cannot publish an incomplete macOS generation")
	}
	root, err := filepath.Abs(paths.GenerationsDirectory)
	if err != nil {
		return fmt.Errorf("resolve macOS generations directory: %w", err)
	}
	candidateDirectory, err := filepath.Abs(prepared.Directory)
	if err != nil {
		return fmt.Errorf("resolve macOS candidate directory: %w", err)
	}
	relative, err := filepath.Rel(root, candidateDirectory)
	if err != nil || relative == "." || relative == ".." || len(relative) < 3 || relative[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("macOS candidate is outside the generations directory")
	}
	info, err := os.Stat(candidateDirectory)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("macOS candidate directory is unavailable: %w", err)
	}
	metadataContent, err := readJSON(filepath.Join(candidateDirectory, "generation.json"))
	if err != nil {
		return err
	}
	var metadata GenerationMetadata
	if err := unmarshalStrict(metadataContent, &metadata); err != nil {
		return fmt.Errorf("decode macOS candidate metadata: %w", err)
	}
	if metadata.SchemaVersion != RuntimeSchemaVersion || metadata.GenerationID != prepared.Metadata.GenerationID || metadata.IntentDigest != prepared.Metadata.IntentDigest {
		return fmt.Errorf("macOS candidate metadata does not match the prepared generation")
	}
	current := CurrentGeneration{
		SchemaVersion: RuntimeSchemaVersion,
		GenerationID:  prepared.Metadata.GenerationID,
		Directory:     filepath.Base(candidateDirectory),
		IntentDigest:  prepared.Metadata.IntentDigest,
	}
	encoded, err := marshalJSON(current)
	if err != nil {
		return err
	}
	// current.json contains only generation identifiers and a directory name.
	// Keep it world-readable so the unprivileged GUI can report status without
	// requesting administrator authorization; generated sing-box configs remain
	// private under the root-owned runtime directory.
	return atomicWriteMode(filepath.Join(paths.Root, "current.json"), encoded, 0o644)
}

func (paths Paths) LoadCurrent() (CurrentGeneration, error) {
	content, err := readJSON(filepath.Join(paths.Root, "current.json"))
	if err != nil {
		return CurrentGeneration{}, err
	}
	var current CurrentGeneration
	if err := unmarshalStrict(content, &current); err != nil {
		return CurrentGeneration{}, fmt.Errorf("decode macOS current generation: %w", err)
	}
	if current.SchemaVersion != RuntimeSchemaVersion || current.GenerationID == "" || current.Directory == "" {
		return CurrentGeneration{}, fmt.Errorf("invalid macOS current generation contract")
	}
	return current, nil
}

func marshalJSON(value any) ([]byte, error) {
	encoded, err := jsonMarshal(value)
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
