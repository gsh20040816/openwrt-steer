// SPDX-License-Identifier: GPL-3.0-or-later

// Package generation owns the platform-neutral files in one prepared runtime
// generation. Platform adapters may add files but must not change these names.
package generation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	model "github.com/gsh20040816/steer/go/internal/intent"
)

type Candidate struct {
	Directory string `json:"directory"`
}

func Create(root string, value model.Intent, singBox map[string]any) (candidate Candidate, returnErr error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Candidate{}, fmt.Errorf("create generation root: %w", err)
	}
	directory, err := os.MkdirTemp(root, "candidate.")
	if err != nil {
		return Candidate{}, fmt.Errorf("create candidate generation: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = os.RemoveAll(directory)
		}
	}()
	if err := os.Chmod(directory, 0o700); err != nil {
		return Candidate{}, fmt.Errorf("protect candidate generation: %w", err)
	}
	intentFile, err := os.OpenFile(filepath.Join(directory, "intent.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Candidate{}, fmt.Errorf("create intent.json: %w", err)
	}
	if err := model.EncodeJSON(intentFile, value); err != nil {
		intentFile.Close()
		return Candidate{}, err
	}
	if err := intentFile.Close(); err != nil {
		return Candidate{}, fmt.Errorf("close intent.json: %w", err)
	}
	if err := WriteJSON(filepath.Join(directory, "sing-box.json"), singBox); err != nil {
		return Candidate{}, err
	}
	return Candidate{Directory: directory}, nil
}

func WriteJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		file.Close()
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	return nil
}

func ReadIntent(directory string) (model.Intent, error) {
	file, err := os.Open(filepath.Join(directory, "intent.json"))
	if err != nil {
		return model.Intent{}, fmt.Errorf("open current intent: %w", err)
	}
	defer file.Close()
	return model.DecodeJSON(file)
}
